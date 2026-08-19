package exchange

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSafeClientHealthPostsAllMids(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(t, r)
		got = body
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"BTC": "99900",
			"ETH": "2000",
		})
	}))
	defer srv.Close()

	client := NewSafeClient(srv.URL, "wss://example.invalid/ws", "", testingLogger())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	health, err := client.Health(ctx)
	if err != nil {
		t.Fatalf("health failed: %v", err)
	}
	if health.Connected != true {
		t.Fatalf("expected connected status, got %#v", health)
	}
	if got["type"] != "allMids" {
		t.Fatalf("expected request type allMids, got %#v", got)
	}
}

func TestSafeClientMarketsRequestsMetaAndAssetCtxsAndParses(t *testing.T) {
	t.Parallel()
	var got map[string]any
	response := map[string]any{
		"universe": []any{
			map[string]any{
				"name":          "BTC",
				"markPx":        "100000",
				"indexPx":       "99900",
				"maxLeverage":   20,
				"isDelisted":    false,
				"baseDecimals":  8,
				"quoteDecimals": 6,
				"pxDecimals":    2,
				"szDecimals":    4,
				"funding":       "0.0001",
				"index":         "BTC-PERP",
				"timeInForce":   "gtc",
			},
		},
		"assetCtxs": []any{
			map[string]any{
				"name":        "BTC",
				"markPx":      "99990",
				"funding":     "0.0001",
				"maxLeverage": 25,
				"isCross":     true,
				"type":        "cross",
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = decodeBody(t, r)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	client := NewSafeClient(srv.URL, "wss://example.invalid/ws", "", testingLogger())
	markets, err := client.Markets(context.Background())
	if err != nil {
		t.Fatalf("markets failed: %v", err)
	}
	if len(markets) != 1 {
		t.Fatalf("expected 1 market, got %d", len(markets))
	}
	if got["type"] != "metaAndAssetCtxs" {
		t.Fatalf("expected payload type metaAndAssetCtxs, got %#v", got)
	}
	if markets[0].Symbol != "BTC" {
		t.Fatalf("expected BTC market, got %q", markets[0].Symbol)
	}
	if markets[0].Funding == "" || markets[0].LeverageInfo.MaxLeverage == 0 {
		t.Fatalf("expected merged context fields, got %#v", markets[0])
	}
}

func TestSafeClientMarketsParsesCanonicalMetaAndAssetCtxsTuple(t *testing.T) {
	t.Parallel()
	response := []any{
		map[string]any{
			"universe": []any{
				map[string]any{
					"name":        "BTC",
					"szDecimals":  5,
					"maxLeverage": 50,
				},
			},
		},
		[]any{
			map[string]any{
				"markPx":   "115000.0",
				"oraclePx": "114990.0",
				"funding":  "0.0000125",
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = decodeBody(t, r)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	client := NewSafeClient(srv.URL, "wss://example.invalid/ws", "", testingLogger())
	markets, err := client.Markets(context.Background())
	if err != nil {
		t.Fatalf("markets failed: %v", err)
	}
	if len(markets) != 1 {
		t.Fatalf("expected 1 market, got %d", len(markets))
	}
	if markets[0].Symbol != "BTC" || markets[0].Base != "BTC" || markets[0].Quote != "USD" || markets[0].IndexName != "BTC-PERP" || markets[0].MarkPx != "115000.0" || markets[0].Funding != "0.0000125" {
		t.Fatalf("canonical tuple was not merged: %#v", markets[0])
	}
}

func TestSafeClientBookPostsL2BookPayloadAndParsesLevelTotals(t *testing.T) {
	t.Parallel()
	var got map[string]any
	response := map[string]any{
		"coin": "BTC",
		"levels": []any{
			[]any{
				[]any{"101.00", "2"},
				[]any{"102.00", "3"},
			},
			[]any{
				[]any{"99.00", "1"},
				[]any{"98.50", "2"},
			},
		},
		"time": 1700000123,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = decodeBody(t, r)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	client := NewSafeClient(srv.URL, "wss://example.invalid/ws", "", testingLogger())
	book, err := client.Book(context.Background(), "btc")
	if err != nil {
		t.Fatalf("book failed: %v", err)
	}
	if got["type"] != "l2Book" || got["coin"] != "BTC" || got["nSigFigs"] != float64(4) {
		t.Fatalf("unexpected request: %#v", got)
	}
	if len(book.Bids) != 2 || len(book.Asks) != 2 {
		t.Fatalf("unexpected level counts: bids=%d asks=%d", len(book.Bids), len(book.Asks))
	}
	if book.Bids[1].Total == "" || book.Asks[1].Total == "" {
		t.Fatalf("expected cumulative totals: %#v", book)
	}
}

func TestSafeClientTradesPostsRecentTradesAndParses(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = decodeBody(t, r)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode([]any{
			map[string]any{
				"coin": "BTC",
				"side": "sell",
				"sz":   "0.20",
				"px":   "25100.5",
				"ts":   1700000200,
				"seq":  7,
			},
			map[string]any{
				"coin":  "BTC",
				"side":  "buy",
				"size":  "0.10",
				"price": "25090.0",
				"time":  1700000210,
				"seq":   8,
			},
		})
	}))
	defer srv.Close()

	client := NewSafeClient(srv.URL, "wss://example.invalid/ws", "", testingLogger())
	trades, err := client.Trades(context.Background(), "BTC", 2)
	if err != nil {
		t.Fatalf("trades failed: %v", err)
	}
	if got["type"] != "recentTrades" || got["coin"] != "BTC" || got["n"] != float64(2) {
		t.Fatalf("unexpected request: %#v", got)
	}
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}
	if trades[0].Px != "25100.5" || trades[0].Ts != 1700000200 {
		t.Fatalf("unexpected trade: %#v", trades[0])
	}
	wire, err := json.Marshal(trades[0])
	if err != nil {
		t.Fatalf("marshal trade: %v", err)
	}
	var wireTrade map[string]any
	if err := json.Unmarshal(wire, &wireTrade); err != nil {
		t.Fatalf("decode trade wire shape: %v", err)
	}
	if wireTrade["ts"] == nil || wireTrade["timestamp"] != nil {
		t.Fatalf("frontend trade timestamp must use ts: %s", wire)
	}
}

func TestSafeClientCandlesPostsCandleSnapshotPayloadAndParses(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = decodeBody(t, r)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candles": []any{
				[]any{1700000100000.0, "101", "110", "90", "105", "100"},
				[]any{1700000200000.0, "105", "112", "95", "108", "150"},
			},
		})
	}))
	defer srv.Close()

	client := NewSafeClient(srv.URL, "wss://example.invalid/ws", "", testingLogger())
	candles, err := client.Candles(context.Background(), "BTC", "15m", 2)
	if err != nil {
		t.Fatalf("candles failed: %v", err)
	}
	req, ok := got["req"].(map[string]any)
	if !ok || got["type"] != "candleSnapshot" || req["coin"] != "BTC" || req["interval"] != "15m" {
		t.Fatalf("unexpected request: %#v", got)
	}
	if len(candles) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(candles))
	}
	if candles[0].Timestamp != 1700000100000 || candles[0].Close != "105" {
		t.Fatalf("unexpected candle: %#v", candles[0])
	}
}

func TestSafeClientOrderEndpointsUseExpectedPayloadsAndNormalize(t *testing.T) {
	t.Parallel()
	requests := make(map[string]map[string]any)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(t, r)
		method := body["type"].(string)
		requests[method] = body
		switch method {
		case "frontendOpenOrders":
			_ = json.NewEncoder(w).Encode([]any{
				map[string]any{
					"coin":      "BTC",
					"oid":       "oid-1",
					"side":      "buy",
					"orderType": "stopLimit",
					"sz":        "0.5",
					"px":        "10100",
					"triggerPx": "9800",
					"status":    "open",
					"time":      171,
				},
			})
		case "historicalOrders":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"orders": []any{
					map[string]any{
						"coin":      "BTC",
						"oid":       "oid-2",
						"side":      "sell",
						"orderType": "limit",
						"sz":        "0.2",
						"px":        "10200",
						"status":    "closed",
						"time":      172,
					},
				},
			})
		case "userFills":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"fills": []any{
					map[string]any{
						"coin":   "BTC",
						"side":   "buy",
						"sz":     "0.2",
						"px":     "10120",
						"fee":    "0.001",
						"fillId": "fill-1",
						"oid":    "oid-1",
						"time":   173,
					},
				},
			})
		case "userFunding":
			_ = json.NewEncoder(w).Encode([]any{
				map[string]any{
					"coin":        "BTC",
					"fundingRate": "0.0001",
					"payment":     "0.01",
					"time":        174,
				},
			})
		case "clearinghouseState":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"marginSummary": map[string]any{
					"crossBalance":     "50",
					"availableBalance": "40",
					"totalMarginUsed":  "10",
					"marginMode":       "cross",
				},
				"assetPositions": []any{
					map[string]any{
						"position": map[string]any{
							"coin":                 "BTC",
							"szi":                  "0.20",
							"leverage":             20,
							"entryPx":              "100",
							"markPx":               "120",
							"unrealizedPnl":        "4",
							"realizedPnl":          "1",
							"liquidationPx":        "70",
							"unrealizedPnlPercent": "0.34",
						},
					},
				},
				"assets": []any{
					map[string]any{
						"coin":            "USDC",
						"wallet":          "1000",
						"crossMarginUsed": "60",
						"available":       "940",
					},
				},
			})
		default:
			t.Fatalf("unexpected request payload %v", method)
		}
	}))
	defer srv.Close()

	client := NewSafeClient(srv.URL, "wss://example.invalid/ws", "0xabc", testingLogger())
	ctx := context.Background()

	ordersOpen, err := client.Orders(ctx, "0xabc", "open")
	if err != nil {
		t.Fatalf("orders open failed: %v", err)
	}
	if len(ordersOpen) != 0 {
		t.Fatalf("expected open list to exclude stop orders")
	}

	ordersTrigger, err := client.Orders(ctx, "0xabc", "trigger")
	if err != nil {
		t.Fatalf("orders trigger failed: %v", err)
	}
	if len(ordersTrigger) != 1 {
		t.Fatalf("expected one trigger order, got %d", len(ordersTrigger))
	}

	ordersHistory, err := client.Orders(ctx, "0xabc", "history")
	if err != nil {
		t.Fatalf("orders history failed: %v", err)
	}
	if len(ordersHistory) != 1 {
		t.Fatalf("expected one historical order, got %d", len(ordersHistory))
	}

	fills, err := client.Fills(ctx, "0xabc")
	if err != nil || len(fills) != 1 {
		t.Fatalf("fills failed: %v len=%d", err, len(fills))
	}
	if fills[0].FillID != "fill-1" {
		t.Fatalf("unexpected fill: %#v", fills[0])
	}

	funding, err := client.Funding(ctx, "0xabc")
	if err != nil || len(funding) != 1 {
		t.Fatalf("funding failed: %v len=%d", err, len(funding))
	}
	if funding[0].Rate != "0.0001" {
		t.Fatalf("unexpected funding: %#v", funding[0])
	}

	account, err := client.AccountSnapshot(ctx, "0xabc")
	if err != nil {
		t.Fatalf("account snapshot failed: %v", err)
	}
	if account.Margin.CrossBalance != "50" || account.Margin.AvailableBalance != "40" {
		t.Fatalf("unexpected margin: %#v", account.Margin)
	}
	if account.Positions[0].Symbol != "BTC" || account.Positions[0].Side != "buy" {
		t.Fatalf("unexpected position: %#v", account.Positions[0])
	}

	if requests["frontendOpenOrders"]["type"] != "frontendOpenOrders" {
		t.Fatalf("missing/open-orders request %v", requests["frontendOpenOrders"])
	}
	if requests["historicalOrders"]["type"] != "historicalOrders" {
		t.Fatalf("missing/history request %v", requests["historicalOrders"])
	}
	if requests["userFills"]["type"] != "userFills" {
		t.Fatalf("missing/fills request %v", requests["userFills"])
	}
	if requests["userFunding"]["type"] != "userFunding" {
		t.Fatalf("missing/funding request %v", requests["userFunding"])
	}
}

func TestIntegrationPublicReadMethods_GatedByEnv(t *testing.T) {
	if !strings.EqualFold(os.Getenv("FAKEMEX_INTEGRATION"), "1") {
		t.Skip("set FAKEMEX_INTEGRATION=1 to run live test")
	}
	apiURL := os.Getenv("HL_API_URL")
	if strings.TrimSpace(apiURL) == "" {
		apiURL = "https://api.hyperliquid-testnet.xyz"
	}
	wsURL := os.Getenv("HL_WS_URL")
	if strings.TrimSpace(wsURL) == "" {
		wsURL = "wss://api.hyperliquid-testnet.xyz/ws"
	}
	client := NewSafeClient(apiURL, wsURL, "", testingLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	markets, err := client.Markets(ctx)
	if err != nil {
		t.Fatalf("markets failed: %v", err)
	}
	if len(markets) == 0 {
		t.Fatal("markets should not be empty")
	}
	symbol := markets[0].Symbol

	if _, err := client.Health(ctx); err != nil {
		t.Fatalf("health failed: %v", err)
	}
	if _, err := client.AssetContexts(ctx); err != nil {
		t.Fatalf("asset contexts failed: %v", err)
	}
	if _, err := client.Book(ctx, symbol); err != nil {
		t.Fatalf("book failed: %v", err)
	}
	if _, err := client.Trades(ctx, symbol, 5); err != nil {
		t.Fatalf("trades failed: %v", err)
	}
	if _, err := client.Candles(ctx, symbol, "15m", 5); err != nil {
		t.Fatalf("candles failed: %v", err)
	}
}

func TestSafeClientSubscribeSendsExpectedFramesAndReconnects(t *testing.T) {
	t.Parallel()

	type frameSet struct {
		id     int
		frames []map[string]any
	}

	var (
		connID   int32
		captured = make(chan frameSet, 4)
	)

	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		id := int(atomic.AddInt32(&connID, 1))

		var frames []map[string]any
		for i := 0; i < 4; i++ {
			_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, payload, err := ws.ReadMessage()
			if err != nil {
				t.Errorf("read frame failed (conn=%d): %v", id, err)
				return
			}
			var frame map[string]any
			if err := json.Unmarshal(payload, &frame); err != nil {
				t.Errorf("decode frame failed (conn=%d): %v", id, err)
				return
			}
			frames = append(frames, frame)
		}
		captured <- frameSet{id: id, frames: frames}

		if id == 1 {
			_ = ws.WriteJSON(map[string]any{
				"channel": "subscriptionResponse",
				"data":    map[string]any{"status": "ok"},
			})
			_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "ok"))
			return
		}

		_ = ws.WriteJSON(map[string]any{
			"channel": "activeAssetCtx",
			"data": map[string]any{
				"coin": "BTC",
				"ctx": map[string]any{
					"markPx":  "25000",
					"funding": "0.0001",
				},
			},
		})
		_ = ws.WriteJSON(map[string]any{
			"channel": "trades",
			"data": []any{
				map[string]any{
					"coin": "BTC",
					"side": "buy",
					"sz":   "1",
					"px":   "25000",
					"ts":   1,
				},
			},
		})
		time.Sleep(1 * time.Second)
	}))
	defer srv.Close()

	client := NewSafeClient("http://example.invalid", strings.Replace(srv.URL, "http://", "ws://", 1), "", testingLogger())
	events, err := client.Subscribe(context.Background(), []string{"BTC"}, "15m")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	received := make(map[int]frameSet)
	for len(received) < 2 {
		select {
		case fs := <-captured:
			received[fs.id] = fs
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout waiting for websocket connections")
		}
	}

	first, ok1 := received[1]
	second, ok2 := received[2]
	if !ok1 || !ok2 {
		t.Fatalf("expected two websocket connections, got %#v", received)
	}
	assertWSSubscribeFrames(t, first.frames, second.frames)

	assertStreamChannelRemainsOpen(t, events)
}

func assertWSSubscribeFrames(t *testing.T, firstConnection, secondConnection []map[string]any) {
	t.Helper()
	if len(firstConnection) != 4 || len(secondConnection) != 4 {
		t.Fatalf("expected 4 frames for each connection, got %d and %d", len(firstConnection), len(secondConnection))
	}
	expected := map[string]int{
		"l2book:BTC":         0,
		"activeassetctx:BTC": 0,
		"trades:BTC":         0,
		"candle:BTC":         0,
	}

	for _, frame := range append(firstConnection, secondConnection...) {
		if frame["method"] != "subscribe" {
			t.Fatalf("expected subscribe method, got %v", frame["method"])
		}
		subscription, ok := frame["subscription"].(map[string]any)
		if !ok {
			t.Fatalf("missing subscription: %#v", frame)
		}
		typ := asString(subscription["type"])
		symbol := asString(subscription["coin"])
		key := strings.ToLower(typ) + ":" + strings.ToUpper(symbol)
		if _, ok := expected[key]; !ok {
			t.Fatalf("unexpected subscription frame: %#v", frame)
		}
		if typ == "l2Book" && asInt(subscription["nSigFigs"]) != 4 {
			t.Fatalf("expected nSigFigs 4 for l2Book, got %v", subscription["nSigFigs"])
		}
		if typ == "candle" && asString(subscription["interval"]) != "15m" {
			t.Fatalf("unexpected candle interval: %v", subscription["interval"])
		}
		expected[key]++
	}
	if len(expected) != 4 {
		t.Fatalf("unexpected subscribe frame count map: %#v", expected)
	}
	for key, got := range expected {
		if got != 2 {
			t.Fatalf("expected exactly 1 subscription frame per connection for %s, got %d", key, got)
		}
	}
}

func TestSafeClientSubscribeDispatchesActiveAssetContext(t *testing.T) {
	t.Parallel()

	captured := make(chan []map[string]any, 1)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		var frames []map[string]any
		for i := 0; i < 4; i++ {
			_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, payload, err := ws.ReadMessage()
			if err != nil {
				t.Errorf("read frame failed: %v", err)
				return
			}
			var frame map[string]any
			if err := json.Unmarshal(payload, &frame); err != nil {
				t.Errorf("decode frame failed: %v", err)
				return
			}
			frames = append(frames, frame)
		}
		captured <- frames

		_ = ws.WriteJSON(map[string]any{
			"channel": "subscriptionResponse",
			"data":    map[string]any{"status": "ok"},
		})
		_ = ws.WriteJSON(map[string]any{
			"channel": "activeAssetCtx",
			"data": map[string]any{
				"coin": "BTC",
				"ctx": map[string]any{
					"markPx":  "25010",
					"funding": "0.0002",
				},
			},
		})
		_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "ok"))
	}))
	defer srv.Close()

	client := NewSafeClient("http://example.invalid", strings.Replace(srv.URL, "http://", "ws://", 1), "", testingLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	events, err := client.Subscribe(ctx, []string{"BTC"}, "15m")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	select {
	case frames := <-captured:
		assertActiveAssetCtxFrame(t, frames)
	case <-ctx.Done():
		t.Fatal("timeout waiting for websocket subscription capture")
	}

	select {
	case ev := <-events:
		if ev.Type != EventAssetContext {
			t.Fatalf("expected %s event, got %s", EventAssetContext, ev.Type)
		}
		payload, ok := ev.Data.(map[string]any)
		if !ok {
			t.Fatalf("expected map payload, got %#v", ev.Data)
		}
		if asString(payload["symbol"]) != "BTC" || asString(payload["markPx"]) != "25010" || asString(payload["funding"]) != "0.0002" {
			t.Fatalf("unexpected asset context payload: %#v", payload)
		}
		if _, present := payload["maxLeverage"]; present {
			t.Fatalf("expected sparse active asset payload without maxLeverage, got %#v", payload)
		}
		if _, present := payload["baseDecimals"]; present {
			t.Fatalf("expected sparse active asset payload without baseDecimals, got %#v", payload)
		}
		if _, present := payload["quoteDecimals"]; present {
			t.Fatalf("expected sparse active asset payload without quoteDecimals, got %#v", payload)
		}
		if _, present := payload["leverage"]; present {
			t.Fatalf("expected sparse active asset payload without leverage, got %#v", payload)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for activeAssetCtx event")
	}
}

func TestSafeClientSubscribeIncludesAccountUserFramesWhenConfigured(t *testing.T) {
	t.Parallel()

	type accountFeedFrameSet struct {
		frames []map[string]any
	}
	captured := make(chan accountFeedFrameSet, 2)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		var frames []map[string]any
		for i := 0; i < 8; i++ {
			_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, payload, err := ws.ReadMessage()
			if err != nil {
				t.Errorf("read frame failed: %v", err)
				return
			}
			var frame map[string]any
			if err := json.Unmarshal(payload, &frame); err != nil {
				t.Errorf("decode frame failed: %v", err)
				return
			}
			frames = append(frames, frame)
		}
		captured <- accountFeedFrameSet{frames: frames}
		_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "ok"))
	}))
	defer srv.Close()

	client := NewSafeClient("http://example.invalid", strings.Replace(srv.URL, "http://", "ws://", 1), "0xabcdef", testingLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := client.Subscribe(ctx, []string{"BTC"}, "15m")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	_ = events

	select {
	case capturedFrames := <-captured:
		seen := map[string]struct{}{}
		for _, frame := range capturedFrames.frames {
			subscription, ok := frame["subscription"].(map[string]any)
			if !ok {
				t.Fatalf("missing subscription: %#v", frame)
			}
			typ := asString(subscription["type"])
			seen[typ] = struct{}{}
			if typ == "clearinghouseState" && asString(subscription["user"]) != "0xabcdef" {
				t.Fatalf("unexpected clearinghouseState user: %#v", subscription)
			}
			if typ == "userFills" && asString(subscription["user"]) != "0xabcdef" {
				t.Fatalf("unexpected userFills user: %#v", subscription)
			}
			if typ == "userFundings" && asString(subscription["user"]) != "0xabcdef" {
				t.Fatalf("unexpected userFundings user: %#v", subscription)
			}
			if typ == "openOrders" && asString(subscription["user"]) != "0xabcdef" {
				t.Fatalf("unexpected openOrders user: %#v", subscription)
			}
		}
		required := map[string]struct{}{
			"l2Book":             {},
			"activeAssetCtx":     {},
			"trades":             {},
			"candle":             {},
			"clearinghouseState": {},
			"userFills":          {},
			"userFundings":       {},
			"openOrders":         {},
		}
		for typ := range required {
			if _, ok := seen[typ]; !ok {
				t.Fatalf("missing subscription type %q", typ)
			}
		}
		cancel()
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for captured account subscription frames")
	}
}

func TestSafeClientSubscribeDispatchesFullAccountFromClearinghouseState(t *testing.T) {
	t.Parallel()

	captured := make(chan []map[string]any, 1)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		var frames []map[string]any
		for i := 0; i < 8; i++ {
			_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, payload, err := ws.ReadMessage()
			if err != nil {
				t.Errorf("read frame failed: %v", err)
				return
			}
			var frame map[string]any
			if err := json.Unmarshal(payload, &frame); err != nil {
				t.Errorf("decode frame failed: %v", err)
				return
			}
			frames = append(frames, frame)
		}
		captured <- frames

		_ = ws.WriteJSON(map[string]any{
			"channel": "subscriptionResponse",
			"data":    map[string]any{"status": "ok"},
		})
		_ = ws.WriteJSON(map[string]any{
			"channel": "clearinghouseState",
			"data": map[string]any{
				"user": "0xabc",
				"marginSummary": map[string]any{
					"crossBalance":     "500",
					"availableBalance": "420",
					"totalMarginUsed":  "80",
					"marginMode":       "cross",
				},
				"assetPositions": []any{
					map[string]any{
						"coin":             "BTC",
						"szi":              "0.75",
						"size":             "0.75",
						"side":             "buy",
						"leverage":         12,
						"entryPx":          "25000",
						"markPx":           "26000",
						"unrealizedPnl":    "150",
						"realizedPnl":      "10",
						"liquidationPx":    "18000",
						"liquidationPrice": "18000",
					},
				},
				"assets": []any{
					map[string]any{
						"coin":            "USDC",
						"wallet":          "1000",
						"crossMarginUsed": "350",
						"available":       "420",
					},
				},
			},
		})
		_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "ok"))
	}))
	defer srv.Close()

	client := NewSafeClient("http://example.invalid", strings.Replace(srv.URL, "http://", "ws://", 1), "0xabc", testingLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := client.Subscribe(ctx, []string{"BTC"}, "15m")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	select {
	case frames := <-captured:
		if len(frames) != 8 {
			t.Fatalf("expected 8 subscription frames, got %d", len(frames))
		}
		found := false
		foundOpenOrders := false
		for _, frame := range frames {
			subscription, ok := frame["subscription"].(map[string]any)
			if !ok {
				continue
			}
			if asString(subscription["type"]) == "clearinghouseState" {
				found = true
				if asString(subscription["user"]) != "0xabc" {
					t.Fatalf("expected clearinghouseState user 0xabc, got %#v", subscription)
				}
			}
			if asString(subscription["type"]) == "openOrders" {
				foundOpenOrders = true
				if asString(subscription["user"]) != "0xabc" {
					t.Fatalf("expected openOrders user 0xabc, got %#v", subscription)
				}
			}
		}
		if !found {
			t.Fatal("did not capture clearinghouseState subscription frame")
		}
		if !foundOpenOrders {
			t.Fatal("did not capture openOrders subscription frame")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for subscription capture")
	}

	select {
	case ev := <-events:
		if ev.Type != EventAccount {
			t.Fatalf("expected %s event, got %s", EventAccount, ev.Type)
		}
		account, ok := ev.Data.(Account)
		if !ok {
			t.Fatalf("expected Account payload, got %#v", ev.Data)
		}
		if account.Margin.CrossBalance != "500" || account.Margin.AvailableBalance != "420" || account.Margin.TotalMarginUsed != "80" {
			t.Fatalf("unexpected account margin: %#v", account.Margin)
		}
		if account.LeverageMode != "cross" {
			t.Fatalf("unexpected leverage mode: %#v", account.LeverageMode)
		}
		if len(account.Positions) != 1 {
			t.Fatalf("expected one position, got %d", len(account.Positions))
		}
		if account.Positions[0].Symbol != "BTC" || account.Positions[0].Side != "buy" || account.Positions[0].Size != "0.75" {
			t.Fatalf("unexpected position: %#v", account.Positions[0])
		}
		if len(account.Assets) != 1 || account.Assets[0].Coin != "USDC" || account.Assets[0].Wallet != "1000" {
			t.Fatalf("unexpected assets: %#v", account.Assets)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for clearinghouseState account event")
	}
}

func TestSafeClientSubscribeDispatchesOpenOrdersSnapshot(t *testing.T) {
	t.Parallel()

	captured := make(chan []map[string]any, 1)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer ws.Close()

		var frames []map[string]any
		for i := 0; i < 8; i++ {
			_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, payload, err := ws.ReadMessage()
			if err != nil {
				t.Errorf("read frame failed: %v", err)
				return
			}
			var frame map[string]any
			if err := json.Unmarshal(payload, &frame); err != nil {
				t.Errorf("decode frame failed: %v", err)
				return
			}
			frames = append(frames, frame)
		}
		captured <- frames

		_ = ws.WriteJSON(map[string]any{
			"channel": "subscriptionResponse",
			"data":    map[string]any{"status": "ok"},
		})
		_ = ws.WriteJSON(map[string]any{
			"channel": "openOrders",
			"data": map[string]any{
				"dex":  "0x1",
				"user": "0xabc",
				"orders": []any{
					map[string]any{
						"coin":      "BTC",
						"oid":       "oid-1",
						"side":      "buy",
						"orderType": "limit",
						"sz":        "0.5",
						"px":        "10100",
						"time":      171,
					},
					map[string]any{
						"coin":      "BTC",
						"oid":       "oid-2",
						"side":      "sell",
						"orderType": "stopMarket",
						"sz":        "1.0",
						"triggerPx": "9800",
						"time":      172,
					},
				},
			},
		})
		_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "ok"))
	}))
	defer srv.Close()

	client := NewSafeClient("http://example.invalid", strings.Replace(srv.URL, "http://", "ws://", 1), "0xabc", testingLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := client.Subscribe(ctx, []string{"BTC"}, "15m")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	var openOrdersFrame map[string]any
	select {
	case frames := <-captured:
		for _, frame := range frames {
			subscription, ok := frame["subscription"].(map[string]any)
			if !ok {
				continue
			}
			if asString(subscription["type"]) == "openOrders" {
				openOrdersFrame = frame
				break
			}
		}
		if openOrdersFrame == nil {
			t.Fatal("did not capture openOrders subscription frame")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for websocket subscription capture")
	}

	subscription, ok := openOrdersFrame["subscription"].(map[string]any)
	if !ok {
		t.Fatalf("openOrders frame missing subscription: %#v", openOrdersFrame)
	}
	if openOrdersFrame["method"] != "subscribe" {
		t.Fatalf("expected openOrders frame method subscribe, got %v", openOrdersFrame["method"])
	}
	if asString(subscription["type"]) != "openOrders" {
		t.Fatalf("expected openOrders type, got %#v", subscription)
	}
	if asString(subscription["user"]) != "0xabc" {
		t.Fatalf("unexpected openOrders user: %#v", subscription)
	}

	select {
	case ev := <-events:
		if ev.Type != EventOrders {
			t.Fatalf("expected %s event, got %s", EventOrders, ev.Type)
		}
		orders, ok := ev.Data.(OrdersSnapshot)
		if !ok {
			t.Fatalf("expected OrdersSnapshot payload, got %#v", ev.Data)
		}
		if len(orders.Open) != 1 || len(orders.Trigger) != 1 {
			t.Fatalf("unexpected orders snapshot: %#v", orders)
		}
		if orders.Open[0].ID != "oid-1" || orders.Trigger[0].ID != "oid-2" {
			t.Fatalf("unexpected parsed order ids: %#v", orders)
		}
		if orders.Open[0].Status != "open" || orders.Trigger[0].Status != "open" {
			t.Fatalf("expected order status to default to open: %#v", orders)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for openOrders event")
	}
}

func TestParseOpenOrdersAcceptsWrappedPayloads(t *testing.T) {
	t.Parallel()

	orderEntry := map[string]any{
		"coin":      "BTC",
		"oid":       "oid-1",
		"side":      "buy",
		"orderType": "limit",
		"sz":        "0.5",
		"px":        "10100",
		"time":      171,
	}
	orderEntryTrigger := map[string]any{
		"coin":      "BTC",
		"oid":       "oid-2",
		"side":      "sell",
		"orderType": "stopMarket",
		"sz":        "1.0",
		"triggerPx": "9800",
		"time":      172,
	}

	t.Run("orders-array", func(t *testing.T) {
		snapshot, ok := parseOpenOrders([]any{orderEntry, orderEntryTrigger})
		if !ok {
			t.Fatal("expected parseOpenOrders success")
		}
		if len(snapshot.Open) != 1 || len(snapshot.Trigger) != 1 {
			t.Fatalf("unexpected partition: %#v", snapshot)
		}
		if snapshot.Open[0].Status != "open" || snapshot.Trigger[0].Status != "open" {
			t.Fatalf("expected order status to default to open: %#v", snapshot)
		}
	})

	t.Run("data-envelope", func(t *testing.T) {
		raw := map[string]any{
			"data": map[string]any{
				"orders": []any{orderEntry, orderEntryTrigger},
			},
		}
		b, err := json.Marshal(raw)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var rawMsg json.RawMessage
		if err := json.Unmarshal(b, &rawMsg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		snapshot, ok := parseOpenOrders(rawMsg)
		if !ok {
			t.Fatal("expected parseOpenOrders success for data envelope")
		}
		if len(snapshot.Open) != 1 || len(snapshot.Trigger) != 1 {
			t.Fatalf("unexpected partition: %#v", snapshot)
		}
		if snapshot.Open[0].Status != "open" || snapshot.Trigger[0].Status != "open" {
			t.Fatalf("expected order status to default to open: %#v", snapshot)
		}
	})

	t.Run("payload-envelope", func(t *testing.T) {
		raw := map[string]any{
			"payload": map[string]any{
				"openOrders": []any{orderEntry, orderEntryTrigger},
			},
		}
		b, err := json.Marshal(raw)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var rawMsg json.RawMessage
		if err := json.Unmarshal(b, &rawMsg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		snapshot, ok := parseOpenOrders(rawMsg)
		if !ok {
			t.Fatal("expected parseOpenOrders success for payload envelope")
		}
		if len(snapshot.Open) != 1 || len(snapshot.Trigger) != 1 {
			t.Fatalf("unexpected partition: %#v", snapshot)
		}
		if snapshot.Open[0].Status != "open" || snapshot.Trigger[0].Status != "open" {
			t.Fatalf("expected order status to default to open: %#v", snapshot)
		}
	})
}

func assertActiveAssetCtxFrame(t *testing.T, frames []map[string]any) {
	t.Helper()
	if len(frames) != 4 {
		t.Fatalf("expected 4 subscribe frames, got %d", len(frames))
	}
	for _, frame := range frames {
		subscription, ok := frame["subscription"].(map[string]any)
		if !ok {
			t.Fatalf("missing subscription in frame: %#v", frame)
		}
		if asString(subscription["type"]) == "activeAssetCtx" {
			if asString(subscription["coin"]) != "BTC" {
				t.Fatalf("expected activeAssetCtx BTC, got %#v", subscription)
			}
			return
		}
	}
	t.Fatal("missing activeAssetCtx subscription")
}

func assertStreamChannelRemainsOpen(t *testing.T, events <-chan StreamEvent) {
	t.Helper()
	for i := 0; i < 10; i++ {
		select {
		case _, ok := <-events:
			if !ok {
				t.Fatal("stream channel closed unexpectedly after reconnect")
			}
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func TestIntegrationPublicWebsocketStream_GatedByEnv(t *testing.T) {
	if !strings.EqualFold(os.Getenv("FAKEMEX_INTEGRATION"), "1") {
		t.Skip("set FAKEMEX_INTEGRATION=1 to run live test")
	}
	apiURL := os.Getenv("HL_API_URL")
	if strings.TrimSpace(apiURL) == "" {
		apiURL = "https://api.hyperliquid-testnet.xyz"
	}
	wsURL := os.Getenv("HL_WS_URL")
	if strings.TrimSpace(wsURL) == "" {
		wsURL = "wss://api.hyperliquid-testnet.xyz/ws"
	}
	client := NewSafeClient(apiURL, wsURL, "", testingLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	markets, err := client.Markets(ctx)
	if err != nil {
		t.Fatalf("markets failed: %v", err)
	}
	if len(markets) == 0 {
		t.Fatal("markets should not be empty")
	}
	symbol := markets[0].Symbol
	stream, err := client.Subscribe(ctx, []string{symbol}, "15m")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	select {
	case event, ok := <-stream:
		if !ok {
			t.Fatal("stream closed before receiving event")
		}
		if event.Type != EventBook && event.Type != EventTrades && event.Type != EventCandle {
			t.Fatalf("unexpected stream event type %s", event.Type)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for websocket stream event")
	}
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) == 0 {
		return map[string]any{}
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return got
}

func testingLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}))
}
