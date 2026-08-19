package exchange

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type safeClient struct {
	apiURL         string
	wsURL          string
	accountAddress string
	httpClient     *http.Client
	logger         *slog.Logger
	signingKey     *ecdsa.PrivateKey
	vaultAddress   string
	isMainnet      bool
	nonceMu        sync.Mutex
	lastNonce      int64
}

func NewSafeClient(apiURL, wsURL, accountAddress string, logger *slog.Logger) ExchangeClient {
	return newSafeClient(apiURL, wsURL, accountAddress, logger)
}

func newSafeClient(apiURL, wsURL, accountAddress string, logger *slog.Logger) *safeClient {
	return &safeClient{
		apiURL:         strings.TrimRight(apiURL, "/"),
		wsURL:          strings.TrimRight(wsURL, "/"),
		accountAddress: accountAddress,
		httpClient:     &http.Client{Timeout: 8 * time.Second},
		logger:         logger.With("component", "exchange"),
	}
}

func (c *safeClient) Health(ctx context.Context) (Health, error) {
	start := time.Now()
	var out map[string]any
	if err := c.postJSON(ctx, "/info", map[string]any{"type": "allMids"}, &out); err != nil {
		return Health{
			Backend:      "up",
			Upstream:     "error",
			Connected:    false,
			AccountReady: c.accountAddress != "",
		}, err
	}
	if len(out) == 0 {
		return Health{
			Backend:      "up",
			Upstream:     "error",
			Connected:    false,
			AccountReady: c.accountAddress != "",
		}, errors.New("allMids returned empty payload")
	}
	return Health{
		Backend:            "up",
		Upstream:           "connected",
		Connected:          true,
		UpstreamLatencyMs:  time.Since(start).Milliseconds(),
		LastSuccessfulPing: time.Now().UnixMilli(),
		AccountReady:       c.accountAddress != "",
	}, nil
}

func (c *safeClient) Markets(ctx context.Context) ([]Market, error) {
	var raw any
	if err := c.postJSON(ctx, "/info", map[string]any{"type": "metaAndAssetCtxs"}, &raw); err != nil {
		return nil, err
	}
	markets, contexts, err := parseMetaAndAssetCtxs(raw)
	if err != nil {
		return nil, err
	}
	if len(markets) == 0 {
		return []Market{}, nil
	}
	mergeMarketContext(markets, contexts)
	return markets, nil
}

func (c *safeClient) AssetContexts(ctx context.Context) ([]AssetContext, error) {
	var raw any
	if err := c.postJSON(ctx, "/info", map[string]any{"type": "metaAndAssetCtxs"}, &raw); err != nil {
		return nil, err
	}
	_, contexts, err := parseMetaAndAssetCtxs(raw)
	if err != nil {
		return nil, err
	}
	if contexts == nil {
		return []AssetContext{}, nil
	}
	return contexts, nil
}

func (c *safeClient) Book(ctx context.Context, symbol string) (OrderBook, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	var raw any
	payload := map[string]any{
		"type":     "l2Book",
		"coin":     symbol,
		"nSigFigs": 4,
	}
	if err := c.postJSON(ctx, "/info", payload, &raw); err != nil {
		return OrderBook{}, err
	}
	book, err := parseL2Book(raw)
	if err != nil {
		return OrderBook{}, err
	}
	if book.Symbol == "" {
		book.Symbol = symbol
	}
	return book, nil
}

func (c *safeClient) Trades(ctx context.Context, symbol string, limit int) ([]Trade, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if limit <= 0 {
		limit = 50
	}
	var raw any
	payload := map[string]any{
		"type": "recentTrades",
		"coin": symbol,
		"n":    limit,
	}
	if err := c.postJSON(ctx, "/info", payload, &raw); err != nil {
		return []Trade{}, err
	}
	trades := parseTrades(raw)
	for i := range trades {
		if trades[i].Symbol == "" {
			trades[i].Symbol = symbol
		}
	}
	return nonNil(trades), nil
}

func (c *safeClient) Candles(ctx context.Context, symbol, interval string, limit int) ([]Candle, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	interval = strings.TrimSpace(interval)
	if interval == "" {
		interval = "15m"
	}
	if limit <= 0 {
		limit = 200
	}
	span := intervalToMs(interval)
	if span <= 0 {
		span = 15 * 60 * 1000
	}
	endTime := time.Now().UnixMilli()
	startTime := endTime - int64(limit)*span
	if startTime < 0 {
		startTime = 0
	}
	var raw any
	payload := map[string]any{
		"type": "candleSnapshot",
		"req": map[string]any{
			"coin":      symbol,
			"interval":  interval,
			"startTime": startTime,
			"endTime":   endTime,
		},
	}
	if err := c.postJSON(ctx, "/info", payload, &raw); err != nil {
		return []Candle{}, err
	}
	candles := parseCandles(raw)
	for i := range candles {
		if candles[i].Symbol == "" {
			candles[i].Symbol = symbol
		}
	}
	return nonNil(candles), nil
}

func (c *safeClient) AccountSnapshot(ctx context.Context, address string) (Account, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return Account{}, errors.New("address required")
	}
	var clearinghouseRaw any
	payload := map[string]any{
		"type": "clearinghouseState",
		"user": address,
	}
	if err := c.postJSON(ctx, "/info", payload, &clearinghouseRaw); err != nil {
		return Account{}, err
	}
	account, err := parseClearinghouseState(clearinghouseRaw, address)
	if err != nil {
		return Account{}, err
	}

	// Hyperliquid exposes token balances separately from perpetual margin state.
	// Prefer spot balances when present (including unified-account balances), but
	// retain compatibility with upstreams that include assets in clearinghouseState.
	var spotRaw any
	if err := c.postJSON(ctx, "/info", map[string]any{
		"type": "spotClearinghouseState",
		"user": address,
	}, &spotRaw); err != nil {
		c.logger.Warn("spot account state unavailable", "error", err)
	} else if assets := parseAssets(spotRaw); len(assets) > 0 {
		account.Balances = assets
		account.Assets = assets
	}

	if len(account.Assets) == 0 {
		account.Assets = assetsFromMargin(account.Margin)
		account.Balances = account.Assets
	}
	return account, nil
}

func (c *safeClient) Orders(ctx context.Context, address, view string) ([]Order, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, errors.New("address required")
	}
	var raw any
	switch strings.ToLower(view) {
	case "", "open", "trigger":
		typ := "frontendOpenOrders"
		if err := c.postJSON(ctx, "/info", map[string]any{
			"type": typ,
			"user": address,
		}, &raw); err != nil {
			return []Order{}, err
		}
		return parseFrontendOrders(raw, view), nil
	case "history":
		if err := c.postJSON(ctx, "/info", map[string]any{
			"type": "historicalOrders",
			"user": address,
		}, &raw); err != nil {
			return []Order{}, err
		}
		return parseHistoricalOrders(raw), nil
	default:
		return nil, fmt.Errorf("unknown view: %s", view)
	}
}

func (c *safeClient) Fills(ctx context.Context, address string) ([]Fill, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, errors.New("address required")
	}
	var raw any
	if err := c.postJSON(ctx, "/info", map[string]any{
		"type": "userFills",
		"user": address,
	}, &raw); err != nil {
		return []Fill{}, err
	}
	return parseUserFills(raw), nil
}

func (c *safeClient) Funding(ctx context.Context, address string) ([]FundingEvent, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, errors.New("address required")
	}
	var raw any
	if err := c.postJSON(ctx, "/info", map[string]any{
		"type": "userFunding",
		"user": address,
	}, &raw); err != nil {
		return []FundingEvent{}, err
	}
	return parseUserFunding(raw), nil
}

func (c *safeClient) Subscribe(ctx context.Context, symbols []string, interval string) (<-chan StreamEvent, error) {
	if len(symbols) == 0 {
		return nil, errors.New("symbols required")
	}
	if interval == "" {
		interval = "15m"
	}

	out := make(chan StreamEvent, 128)
	go func() {
		defer close(out)
		for {
			if ctx.Err() != nil {
				return
			}

			conn, err := c.connectWS(ctx)
			if err != nil {
				c.logger.Warn("websocket connect failed", "error", err)
				if !waitOrDone(ctx, 2*time.Second) {
					return
				}
				continue
			}
			c.configureWSConnection(conn)
			stopHeartbeat := make(chan struct{})
			go c.pingWS(ctx, conn, stopHeartbeat)

			if !c.subscribeWS(ctx, conn, symbols, interval) {
				close(stopHeartbeat)
				_ = conn.Close()
				if !waitOrDone(ctx, 2*time.Second) {
					return
				}
				continue
			}

			if c.readWSEvents(ctx, conn, out) {
				close(stopHeartbeat)
				_ = conn.Close()
				if !waitOrDone(ctx, 2*time.Second) {
					return
				}
				continue
			}
			close(stopHeartbeat)
			_ = conn.Close()
			return
		}
	}()
	return out, nil
}

func (c *safeClient) Close() {}

func (c *safeClient) connectWS(ctx context.Context) (*websocket.Conn, error) {
	u, err := url.Parse(c.wsURL)
	if err != nil {
		return nil, err
	}
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (c *safeClient) configureWSConnection(conn *websocket.Conn) {
	conn.SetReadLimit(8 * 1024 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	conn.SetPongHandler(func(_ string) error {
		_ = conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		return nil
	})
}

func (c *safeClient) pingWS(ctx context.Context, conn *websocket.Conn, stop <-chan struct{}) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
				c.logger.Warn("websocket ping failed", "error", err)
				return
			}
			_ = conn.SetWriteDeadline(time.Time{})
		}
	}
}

func (c *safeClient) subscribeWS(ctx context.Context, conn *websocket.Conn, symbols []string, interval string) bool {
	accountAddress := strings.TrimSpace(c.accountAddress)
	for _, symbol := range symbols {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" {
			continue
		}
		subscriptions := []map[string]any{
			{
				"method":       "subscribe",
				"subscription": map[string]any{"type": "l2Book", "coin": symbol, "nSigFigs": 4},
			},
			{
				"method":       "subscribe",
				"subscription": map[string]any{"type": "activeAssetCtx", "coin": symbol},
			},
			{
				"method":       "subscribe",
				"subscription": map[string]any{"type": "trades", "coin": symbol},
			},
			{
				"method":       "subscribe",
				"subscription": map[string]any{"type": "candle", "coin": symbol, "interval": interval},
			},
		}
		if accountAddress != "" {
			subscriptions = append(subscriptions,
				map[string]any{"method": "subscribe", "subscription": map[string]any{"type": "clearinghouseState", "user": accountAddress}},
				map[string]any{"method": "subscribe", "subscription": map[string]any{"type": "userFills", "user": accountAddress}},
				map[string]any{"method": "subscribe", "subscription": map[string]any{"type": "userFundings", "user": accountAddress}},
				map[string]any{"method": "subscribe", "subscription": map[string]any{"type": "openOrders", "user": accountAddress}},
			)
		}
		for _, sub := range subscriptions {
			if !writeWS(ctx, conn, sub) {
				return false
			}
		}
	}
	return true
}

func (c *safeClient) readWSEvents(ctx context.Context, conn *websocket.Conn, out chan<- StreamEvent) bool {
	for {
		if ctx.Err() != nil {
			return false
		}
		_ = conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if isWebSocketClosed(err) {
				c.logger.Debug("websocket closed", "error", err)
				return true
			}
			c.logger.Warn("websocket read failed", "error", err)
			return true
		}
		dispatchWSMessage(raw, out)
	}
}

func isWebSocketClosed(err error) bool {
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "close") || strings.Contains(text, "connection closed")
}

func writeWS(ctx context.Context, conn *websocket.Conn, payload any) bool {
	if ctx.Err() != nil {
		return false
	}
	return conn.WriteJSON(payload) == nil
}

func dispatchWSMessage(raw []byte, out chan<- StreamEvent) {
	var envelope struct {
		Channel string          `json:"channel"`
		Type    string          `json:"type"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return
	}
	channel := strings.ToLower(firstNonEmptyString(strings.TrimSpace(envelope.Channel), strings.TrimSpace(envelope.Type)))
	switch channel {
	case "l2book":
		book, err := parseL2Book(envelope.Data)
		if err != nil {
			return
		}
		levels := flattenBookLevels(book)
		sendEvent(out, StreamEvent{Type: EventBook, Symbol: book.Symbol, ServerTime: time.Now().UnixMilli(), Data: levels})
	case "activeassetctx":
		market, ok := parseActiveAssetCtx(envelope.Data, symbolFromEnvelope(envelope.Data))
		if !ok {
			return
		}
		sendEvent(out, StreamEvent{Type: EventAssetContext, Symbol: asString(market["symbol"]), ServerTime: time.Now().UnixMilli(), Data: market})
	case "trades":
		trades := parseTrades(envelope.Data)
		if len(trades) == 0 {
			return
		}
		sendEvent(out, StreamEvent{Type: EventTrades, ServerTime: time.Now().UnixMilli(), Data: trades})
	case "candle":
		candle, ok := parseSingleCandle(envelope.Data)
		if !ok {
			return
		}
		sendEvent(out, StreamEvent{Type: EventCandle, ServerTime: time.Now().UnixMilli(), Data: candle})
	case "clearinghousestate":
		account, ok := parseClearinghouseStateMessage(envelope.Data)
		if !ok {
			return
		}
		sendEvent(out, StreamEvent{Type: EventAccount, ServerTime: time.Now().UnixMilli(), Data: account})
	case "userfills":
		fillEnvelope, err := asMap(envelope.Data)
		if err != nil {
			return
		}
		if asBool(fillEnvelope["isSnapshot"]) {
			return
		}
		if fills := parseUserFillEvents(fillEnvelope); len(fills) > 0 {
			sendEvent(out, StreamEvent{Type: EventFills, ServerTime: time.Now().UnixMilli(), Data: fills})
		}
	case "userfundings":
		fundingEnvelope, err := asMap(envelope.Data)
		if err != nil {
			return
		}
		if asBool(fundingEnvelope["isSnapshot"]) {
			return
		}
		if fundings, ok := parseUserFundingEvents(fundingEnvelope); ok && len(fundings) > 0 {
			sendEvent(out, StreamEvent{Type: EventFunding, ServerTime: time.Now().UnixMilli(), Data: fundings})
		}
	case "openorders":
		orders, ok := parseOpenOrders(envelope.Data)
		if !ok {
			return
		}
		sendEvent(out, StreamEvent{Type: EventOrders, ServerTime: time.Now().UnixMilli(), Data: orders})
	case "activespotassetctx":
		market, ok := parseActiveAssetCtx(envelope.Data, symbolFromEnvelope(envelope.Data))
		if !ok {
			return
		}
		sendEvent(out, StreamEvent{Type: EventAssetContext, Symbol: asString(market["symbol"]), ServerTime: time.Now().UnixMilli(), Data: market})
	}
}

func (c *safeClient) postJSON(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("content-type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxExchangeResponseBytes+1))
	if err != nil {
		return err
	}
	if len(responseBody) > maxExchangeResponseBytes {
		return errors.New("upstream response exceeded size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("upstream error %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(responseBody, out)
}

func parseMetaAndAssetCtxs(raw any) ([]Market, []AssetContext, error) {
	rawMap, ok := raw.(map[string]any)
	if ok {
		markets, err := parseMarketsFromAny(rawMap["universe"])
		if err != nil {
			return nil, nil, err
		}
		contexts, err := parseAssetContextsFromAny(rawMap["assetCtxs"])
		if err != nil {
			return markets, nil, err
		}
		return markets, contexts, nil
	}
	if arr, ok := raw.([]any); ok {
		if len(arr) >= 2 {
			marketSource := arr[0]
			if meta, ok := arr[0].(map[string]any); ok {
				marketSource = meta["universe"]
			}
			markets, err := parseMarketsFromAny(marketSource)
			if err != nil {
				return nil, nil, err
			}
			contexts, err := parseAssetContextsFromAny(arr[1])
			if err != nil {
				return markets, nil, err
			}
			// Hyperliquid's canonical response is [meta, assetCtxs]. Context
			// entries do not repeat the coin name; they align with universe by
			// index. Preserve that association before merging the two arrays.
			for i := range contexts {
				if contexts[i].Symbol == "" && i < len(markets) {
					contexts[i].Symbol = markets[i].Symbol
				}
			}
			return markets, contexts, nil
		}
		markets, err := parseMarketsFromAny(arr)
		return markets, nil, err
	}
	return nil, nil, errors.New("unexpected metaAndAssetCtxs payload")
}

func parseMarketsFromAny(raw any) ([]Market, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, errors.New("expected market array")
	}
	out := make([]Market, 0, len(arr))
	for assetIndex, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		symbol := firstNonEmptyString(
			asString(obj["name"]),
			asString(obj["symbol"]),
			asString(obj["coin"]),
		)
		if symbol == "" {
			continue
		}
		market := Market{
			AssetIndex:      assetIndex,
			Symbol:          symbol,
			Base:            firstNonEmptyString(asString(obj["base"], obj["baseCurrency"]), symbol),
			Quote:           firstNonEmptyString(asString(obj["quote"], obj["quoteCurrency"]), "USD"),
			Active:          !asBool(obj["isDelisted"]),
			MaxLeverage:     firstPositiveInt(asInt(obj["maxLeverage"]), asInt(obj["maxLeverageValue"]), asInt(obj["maxLeverage"]), asInt(obj["maxLeverage"])),
			IndexPrice:      asString(obj["indexPx"], obj["indexPrice"]),
			MarkPx:          firstNonEmptyString(asString(obj["markPx"]), asString(obj["markPrice"])),
			MarkPrice:       firstNonEmptyString(asString(obj["markPx"]), asString(obj["markPrice"]), asString(obj["indexPx"])),
			Contract:        firstNonEmptyString(asString(obj["contract"]), symbol+"-PERP"),
			TimeInForce:     asString(obj["timeInForce"], "gtc"),
			MaxOrderSize:    asString(obj["maxOrderSize"]),
			PricePrecision:  firstPositiveInt(asInt(obj["pxDecimals"]), asInt(obj["priceDecimals"])),
			SizePrecision:   firstPositiveInt(asInt(obj["szDecimals"]), asInt(obj["sizeDecimals"])),
			LastUpdateTime:  asInt64(obj["update"], obj["lastUpdate"], obj["t"]),
			LastFundingRate: asString(obj["funding"], obj["funding8h"]),
			IndexName:       firstNonEmptyString(asString(obj["index"], obj["indexName"]), symbol+"-PERP"),
			Funding:         asString(obj["funding"], obj["funding8h"]),
			BaseDecimals:    firstPositiveInt(asInt(obj["baseDecimals"]), asInt(obj["szDecimals"])),
			QuoteDecimals:   firstPositiveInt(asInt(obj["quoteDecimals"]), asInt(obj["weiDecimals"])),
		}
		market.LeverageInfo = MarketLeverage{
			MaxLeverage:     maxInt(1, firstPositiveInt(market.MaxLeverage, 1)),
			CurrentMode:     "cross",
			CurrentLeverage: maxInt(1, firstPositiveInt(market.MaxLeverage, 1)),
		}
		if market.MarkPx == "" {
			market.MarkPx = market.IndexPrice
			market.MarkPrice = market.IndexPrice
		}
		out = append(out, market)
	}
	return out, nil
}

func parseActiveAssetCtx(raw any, fallbackSymbol string) (map[string]any, bool) {
	body, err := asMap(raw)
	if err != nil {
		return nil, false
	}

	coin := firstNonEmptyString(
		asString(body["coin"]),
		asString(body["symbol"]),
		asString(body["name"]),
		fallbackSymbol,
	)
	if coin == "" {
		return nil, false
	}

	payload := body
	if nested, ok := body["ctx"].(map[string]any); ok {
		payload = nested
	} else if nested, ok := body["ctx"].(map[string]interface{}); ok {
		payload = map[string]any(nested)
	}

	markPx := firstNonEmptyString(
		asString(payload["markPx"]),
		asString(payload["markPrice"]),
		asString(body["markPx"]),
		asString(body["markPrice"]),
	)
	markPrice := firstNonEmptyString(
		asString(payload["markPrice"]),
		asString(body["markPrice"]),
		markPx,
	)
	indexPrice := firstNonEmptyString(
		asString(payload["indexPx"]),
		asString(body["indexPrice"]),
		asString(body["indexPx"]),
		asString(payload["indexPrice"]),
	)
	funding := firstNonEmptyString(
		asString(payload["funding"]),
		asString(body["funding"]),
		asString(payload["funding8h"]),
		asString(body["funding8h"]),
	)
	lastFundingRate := firstNonEmptyString(
		asString(payload["lastFundingRate"]),
		asString(body["lastFundingRate"]),
		funding,
	)

	result := map[string]any{
		"symbol": coin,
	}
	if markPx != "" {
		result["markPx"] = markPx
	}
	if markPrice != "" {
		result["markPrice"] = markPrice
	}
	if indexPrice != "" {
		result["indexPrice"] = indexPrice
	}
	if funding != "" {
		result["funding"] = funding
	}
	if lastFundingRate != "" {
		result["lastFundingRate"] = lastFundingRate
	}

	lastUpdateTime := asInt64(payload["time"], body["time"])
	if lastUpdateTime > 0 {
		result["lastUpdateTime"] = lastUpdateTime
	}

	hasLeverageFields := func(candidate ...string) bool {
		for _, key := range candidate {
			if _, ok := payload[key]; ok {
				return true
			}
			if _, ok := body[key]; ok {
				return true
			}
		}
		return false
	}("leverage", "currentLeverage", "maxLeverage", "crossMaxLeverage", "isolatedMaxLeverage", "maxLeverageValue", "type", "mode", "marginMode", "isCross")

	if hasLeverageFields {
		leverage := map[string]any{}
		if maxLeverage := firstPositiveInt(
			asInt(payload["maxLeverage"]),
			asInt(body["maxLeverage"]),
			asInt(payload["crossMaxLeverage"]),
			asInt(body["crossMaxLeverage"]),
			asInt(payload["isolatedMaxLeverage"]),
			asInt(body["isolatedMaxLeverage"]),
			asInt(payload["maxLeverageValue"]),
			asInt(body["maxLeverageValue"]),
		); maxLeverage > 0 {
			leverage["maxLeverage"] = maxLeverage
		}

		modeCandidate := firstNonEmptyString(
			asString(payload["type"]),
			asString(body["type"]),
			asString(payload["marginMode"]),
			asString(body["marginMode"]),
			asString(payload["mode"]),
			asString(body["mode"]),
		)
		currentMode := ""
		if modeCandidate != "" {
			currentMode = chooseLeverageMode(modeCandidate)
		}
		if currentMode == "" && (payload["isCross"] != nil || body["isCross"] != nil) {
			if asBool(payload["isCross"]) || asBool(body["isCross"]) {
				currentMode = "cross"
			} else {
				currentMode = "isolated"
			}
		}
		if currentMode != "" {
			leverage["currentMode"] = currentMode
		}

		if currentLeverage := firstPositiveInt(
			asInt(payload["leverage"]),
			asInt(payload["currentLeverage"]),
			asInt(body["leverage"]),
			asInt(body["currentLeverage"]),
		); currentLeverage > 0 {
			leverage["currentLeverage"] = currentLeverage
		}

		if len(leverage) > 0 {
			result["leverage"] = leverage
		}
	}

	if baseDecimals := firstPositiveInt(asInt(payload["baseDecimals"]), asInt(body["baseDecimals"])); baseDecimals > 0 {
		result["baseDecimals"] = baseDecimals
	}
	if quoteDecimals := firstPositiveInt(asInt(payload["quoteDecimals"]), asInt(body["quoteDecimals"])); quoteDecimals > 0 {
		result["quoteDecimals"] = quoteDecimals
	}

	return result, true
}

func parseOpenOrders(raw any) (OrdersSnapshot, bool) {
	payload := raw
	for i := 0; i < 2; i++ {
		if objPayload, err := asMap(payload); err == nil {
			if nested := rawValues(objPayload, "data", "payload"); nested != nil {
				payload = nested
				continue
			}
		}
		break
	}

	var ordersContainer []any
	if isSlice(payload) {
		ordersContainer = asArray(payload)
	} else {
		objPayload, err := asMap(payload)
		if err != nil {
			return OrdersSnapshot{}, false
		}
		container := rawValues(objPayload, "orders", "openOrders", "history")
		if container == nil {
			return OrdersSnapshot{}, false
		}
		ordersContainer = asArray(container)
		if ordersContainer == nil {
			// Accept a valid wrapper shape that does not yet include orders.
			ordersContainer = nil
		}
	}

	orders := parseGenericOrders(ordersContainer)
	snapshot := OrdersSnapshot{
		Open:    make([]Order, 0, len(orders)),
		Trigger: make([]Order, 0, len(orders)),
	}
	for _, order := range orders {
		if order.Status == "" {
			order.Status = "open"
		}
		if isTriggerOrder(order.Kind) {
			snapshot.Trigger = append(snapshot.Trigger, order)
			continue
		}
		snapshot.Open = append(snapshot.Open, order)
	}
	return snapshot, true
}

func symbolFromEnvelope(raw any) string {
	body, err := asMap(raw)
	if err != nil {
		return ""
	}
	return asString(body["coin"], body["symbol"])
}

func parseClearinghouseStateMessage(raw any) (Account, bool) {
	data, err := asMap(raw)
	if err != nil {
		return Account{}, false
	}
	state := firstMap(data["clearinghouseState"])
	if len(state) == 0 {
		state = data
	}
	account, err := parseClearinghouseState(state, asString(data["user"]))
	if err != nil {
		return Account{}, false
	}
	return account, true
}

func parseUserFillEvents(raw map[string]any) []Fill {
	candidates := asArray(raw["fills"])
	if len(candidates) == 0 {
		candidates = asArray(raw["data"])
	}
	fills := parseUserFills(candidates)
	return fills
}

func parseUserFundingEvents(raw map[string]any) ([]FundingEvent, bool) {
	candidates := asArray(raw["fundings"])
	if len(candidates) == 0 {
		candidates = asArray(raw["funding"])
	}
	if len(candidates) == 0 {
		candidates = asArray(raw["events"])
	}
	if len(candidates) == 0 {
		candidates = asArray(raw["data"])
	}
	if len(candidates) == 0 {
		return nil, false
	}
	fundings := parseFundingEvents(candidates)
	return fundings, len(fundings) > 0
}

func parseFundingEvents(raw []any) []FundingEvent {
	out := make([]FundingEvent, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		delta, err := asMap(entry["delta"])
		if err == nil {
			out = append(out, FundingEvent{
				Symbol:  asString(delta["coin"]),
				Rate:    asString(delta["fundingRate"]),
				Payment: firstNonEmptyString(asString(delta["usdc"]), asString(delta["payment"])),
				Ts:      asInt64(entry["time"], delta["time"]),
			})
			continue
		}
		legacy := FundingEvent{
			Symbol:  asString(entry["coin"], entry["symbol"]),
			Rate:    asString(entry["fundingRate"], entry["rate"]),
			Payment: firstNonEmptyString(asString(entry["payment"], entry["funding"], entry["amount"]), asString(entry["delta"], entry["value"])),
			Ts:      asInt64(entry["time"], entry["ts"], entry["timestamp"]),
		}
		out = append(out, legacy)
	}
	return nonNil(out)
}

func parseAssetContextsFromAny(raw any) ([]AssetContext, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, errors.New("expected asset context array")
	}
	out := make([]AssetContext, 0, len(arr))
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		symbol := firstNonEmptyString(
			asString(obj["name"]),
			asString(obj["symbol"]),
			asString(obj["coin"]),
		)
		out = append(out, AssetContext{
			Symbol:      symbol,
			Leverage:    asInt(obj["leverage"]),
			Type:        asString(obj["type"]),
			MaxLeverage: firstPositiveInt(asInt(obj["maxLeverage"]), asInt(obj["crossMaxLeverage"]), asInt(obj["isolatedMaxLeverage"])),
			IsCross:     asBool(obj["isCross"]),
			MarkPx:      asString(obj["markPx"]),
			LastFunding: asString(obj["funding"], obj["funding8h"]),
			Mark:        asString(obj["indexPx"], obj["indexPrice"]),
		})
	}
	return out, nil
}

func mergeMarketContext(markets []Market, contexts []AssetContext) {
	ctxBySymbol := make(map[string]AssetContext, len(contexts))
	for _, c := range contexts {
		ctxBySymbol[strings.ToUpper(c.Symbol)] = c
	}
	for i := range markets {
		ctx, ok := ctxBySymbol[strings.ToUpper(markets[i].Symbol)]
		if !ok {
			continue
		}
		markets[i].MarkPx = firstNonEmptyString(markets[i].MarkPx, ctx.MarkPx, markets[i].MarkPx)
		markets[i].MarkPrice = firstNonEmptyString(markets[i].MarkPrice, markets[i].MarkPx)
		markets[i].Funding = firstNonEmptyString(markets[i].Funding, ctx.LastFunding)
		markets[i].LastFundingRate = firstNonEmptyString(markets[i].LastFundingRate, ctx.LastFunding)
		markets[i].LeverageInfo.MaxLeverage = maxInt(markets[i].LeverageInfo.MaxLeverage, ctx.MaxLeverage)
		markets[i].LeverageInfo.CurrentMode = firstNonEmptyString(ctx.Type, "cross")
	}
}

func parseL2Book(raw any) (OrderBook, error) {
	body, err := asMap(raw)
	if err != nil {
		return OrderBook{}, err
	}
	book := OrderBook{
		Symbol:    asString(body["coin"], body["symbol"]),
		UpdatedAt: asInt64(body["time"], body["timeMs"], body["updatedAt"], body["timestamp"]),
	}
	if lv, ok := body["levels"].(map[string]any); ok {
		book.Bids = parseBookLevels(lv["bids"])
		book.Asks = parseBookLevels(lv["asks"])
	} else if lv, ok := body["levels"].([]any); ok {
		if len(lv) >= 2 {
			book.Bids = parseBookLevels(lv[1])
			book.Asks = parseBookLevels(lv[0])
		} else if len(lv) == 1 {
			book.Asks = parseBookLevels(lv[0])
		}
	} else if bookAsks := parseBookLevels(body["asks"]); bookAsks != nil {
		book.Asks = bookAsks
		book.Bids = parseBookLevels(body["bids"])
	}
	for i := range book.Bids {
		book.Bids[i].Side = "buy"
	}
	for i := range book.Asks {
		book.Asks[i].Side = "sell"
	}
	addCumulativeTotal(book.Bids)
	addCumulativeTotal(book.Asks)
	return book, nil
}

func parseBookLevels(raw any) []OrderBookLevel {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]OrderBookLevel, 0, len(items))
	for _, rawLevel := range items {
		level, ok := rawLevel.([]any)
		if ok {
			if len(level) < 2 {
				continue
			}
			out = append(out, OrderBookLevel{
				Price: asString(level[0]),
				Size:  asString(level[1]),
			})
			continue
		}
		if obj, ok := rawLevel.(map[string]any); ok {
			out = append(out, OrderBookLevel{
				Price: asString(obj["px"], obj["price"], obj["0"]),
				Size:  asString(obj["sz"], obj["size"], obj["1"]),
			})
		}
	}
	return out
}

func flattenBookLevels(book OrderBook) []OrderBookLevel {
	out := make([]OrderBookLevel, 0, len(book.Asks)+len(book.Bids))
	for _, level := range book.Bids {
		if level.Side == "" {
			level.Side = "buy"
		}
		out = append(out, level)
	}
	for _, level := range book.Asks {
		if level.Side == "" {
			level.Side = "sell"
		}
		out = append(out, level)
	}
	return out
}

func parseTrades(raw any) []Trade {
	candidates := asArray(raw)
	if len(candidates) == 0 {
		if obj, ok := raw.(map[string]any); ok {
			candidates = asArray(obj["trades"], obj["recentTrades"])
		}
	}
	out := make([]Trade, 0, len(candidates))
	for _, item := range candidates {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if wrapped, ok := obj["trade"].(map[string]any); ok {
			obj = wrapped
		}
		out = append(out, Trade{
			Symbol:   asString(obj["coin"], obj["symbol"]),
			Side:     normalizeSide(asString(obj["side"], obj["dir"])),
			Size:     asString(obj["sz"], obj["size"]),
			Px:       asString(obj["px"], obj["price"]),
			Price:    asString(obj["px"], obj["price"]),
			Ts:       asInt64(obj["time"], obj["ts"], obj["timestamp"]),
			Sequence: asInt64(obj["seq"], obj["sequence"]),
		})
	}
	return nonNil(out)
}

func parseCandles(raw any) []Candle {
	candidates := asArray(raw)
	if len(candidates) == 0 {
		if obj, ok := raw.(map[string]any); ok {
			candidates = asArray(obj["candles"], obj["data"])
		}
	}
	out := make([]Candle, 0, len(candidates))
	for _, item := range candidates {
		switch value := item.(type) {
		case []any:
			if len(value) < 6 {
				continue
			}
			out = append(out, Candle{
				Timestamp: toInt64(value[0]),
				Open:      asString(value[1]),
				High:      asString(value[2]),
				Low:       asString(value[3]),
				Close:     asString(value[4]),
				Volume:    asString(value[5]),
			})
		case map[string]any:
			out = append(out, Candle{
				Timestamp: toInt64(value["t"], value["time"], value["openTime"], value["startTime"]),
				Open:      asString(value["o"], value["open"]),
				High:      asString(value["h"], value["high"]),
				Low:       asString(value["l"], value["low"]),
				Close:     asString(value["c"], value["close"]),
				Volume:    asString(value["v"], value["volume"]),
			})
		}
	}
	return out
}

func parseSingleCandle(raw any) (Candle, bool) {
	candles := parseCandles(raw)
	if len(candles) == 0 {
		return Candle{}, false
	}
	return candles[len(candles)-1], true
}

func parseClearinghouseState(raw any, address string) (Account, error) {
	body, err := asMap(raw)
	if err != nil {
		return Account{}, err
	}
	marginSummary := firstMap(body["marginSummary"], body["crossMarginSummary"], body["marginSummarys"], body["crossMaintenanceMargin"])
	crossMarginSummary := firstMap(body["crossMarginSummary"], body["marginSummary"])
	acc := Account{
		Address: address,
	}
	acc.Positions = parsePositions(rawValues(body, "assetPositions"), rawValues(body, "positions"))
	acc.Balances = parseAssets(rawValues(body, "assets"), rawValues(body, "balances"))
	if acc.Positions == nil {
		acc.Positions = []Position{}
	}
	acc.Margin = Margin{
		CrossBalance: firstNonEmptyString(
			asString(body["crossBalance"]),
			asString(crossMarginSummary["crossMargin"], crossMarginSummary["crossBalance"], crossMarginSummary["accountValue"]),
			asString(marginSummary["accountValue"]),
			asString(body["margin"], body["walletBalance"]),
		),
		AvailableBalance: firstNonEmptyString(
			asString(body["available"]),
			asString(body["withdrawable"]),
			asString(marginSummary["availableBalance"]),
			asString(body["availableBalance"]),
		),
		TotalMarginUsed: firstNonEmptyString(
			asString(body["totalMargin"]),
			asString(marginSummary["totalMarginUsed"]),
			asString(crossMarginSummary["totalMarginUsed"]),
			asString(marginSummary["crossMaintenanceMargin"]),
			asString(marginSummary["totalMaintenanceMargin"]),
		),
	}
	acc.CrossBalance = acc.Margin.CrossBalance
	acc.Available = acc.Margin.AvailableBalance
	acc.TotalMarginUsed = acc.Margin.TotalMarginUsed
	acc.LeverageMode = chooseLeverageMode(asString(body["marginMode"], marginSummary["marginMode"]))
	if acc.LeverageMode == "" {
		acc.LeverageMode = "cross"
	}
	acc.LastUpdate = asInt64(body["time"], body["ts"], body["timestamp"], body["updatedAt"])
	acc.Assets = acc.Balances
	return acc, nil
}

func parsePositions(rawValues ...any) []Position {
	var raw any
	for _, candidate := range rawValues {
		if candidate != nil {
			raw = candidate
			break
		}
	}
	if raw == nil {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]Position, 0, len(arr))
	for _, item := range arr {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if nested, ok := entry["position"].(map[string]any); ok {
			entry = nested
		}
		size := asString(entry["szi"], entry["size"])
		side := normalizeSideFromSize(size)
		position := Position{
			Symbol:         asString(entry["coin"], entry["symbol"]),
			Side:           side,
			Size:           absString(size),
			Leverage:       firstPositiveInt(asInt(entry["leverage"]), asInt(entry["leverageRaw"])),
			EntryPrice:     asString(entry["entryPx"], entry["entryPrice"]),
			MarkPrice:      asString(entry["markPx"], entry["markPrice"]),
			UnrealizedPnl:  firstNonEmptyString(asString(entry["unrealizedPnl"]), asString(entry["upll"]), asString(entry["uPnl"])),
			RealizedPnl:    firstNonEmptyString(asString(entry["realizedPnl"]), asString(entry["rpl"])),
			OpenPnlPercent: asString(entry["unrealizedPnlPercent"], entry["openPnlPercent"]),
			MarginType:     chooseMarginType(asString(entry["marginType"], entry["marginMode"], entry["type"])),
			Liquidation:    asString(entry["liquidationPx"], entry["liquidationPrice"]),
			Margin:         firstNonEmptyString(asString(entry["positionValue"], entry["positionValue"]), asString(entry["margin"]), asString(entry["marginUsed"])),
		}
		out = append(out, position)
	}
	return nonNil(out)
}

func parseAssets(rawValues ...any) []Asset {
	var raw any
	for _, candidate := range rawValues {
		if candidate != nil {
			raw = candidate
			break
		}
	}
	if raw == nil {
		return nil
	}
	items := asArray(raw)
	if items == nil {
		obj, ok := raw.(map[string]any)
		if ok {
			items = asArray(obj["balances"])
		}
	}
	if len(items) == 0 {
		return nil
	}
	out := make([]Asset, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		wallet := firstNonEmptyString(
			asString(entry["wallet"], entry["position"], entry["balance"]),
			asString(entry["total"]),
		)
		used := firstNonEmptyString(
			asString(entry["crossMarginUsed"], entry["marginUsed"]),
			asString(entry["hold"]),
		)
		available := asString(entry["available"], entry["availableBalance"])
		if available == "" {
			available = subtractDecimalStrings(wallet, used)
		}
		out = append(out, Asset{
			Coin:            asString(entry["coin"], entry["symbol"]),
			Wallet:          wallet,
			CrossMarginUsed: used,
			Available:       available,
		})
	}
	return nonNil(out)
}

func assetsFromMargin(margin Margin) []Asset {
	if margin.CrossBalance == "" && margin.AvailableBalance == "" && margin.TotalMarginUsed == "" {
		return nil
	}
	return []Asset{{
		Coin:            "USDC",
		Wallet:          margin.CrossBalance,
		Available:       margin.AvailableBalance,
		CrossMarginUsed: margin.TotalMarginUsed,
	}}
}

func subtractDecimalStrings(total, used string) string {
	if total == "" {
		return ""
	}
	if used == "" {
		return total
	}
	totalValue, ok := new(big.Rat).SetString(total)
	if !ok {
		return total
	}
	usedValue, ok := new(big.Rat).SetString(used)
	if !ok {
		return total
	}
	difference := new(big.Rat).Sub(totalValue, usedValue)
	if difference.Sign() < 0 {
		return "0"
	}
	scale := max(decimalPlaces(total), decimalPlaces(used))
	text := difference.FloatString(scale)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func decimalPlaces(value string) int {
	dot := strings.IndexByte(value, '.')
	if dot < 0 {
		return 0
	}
	return len(value) - dot - 1
}

func parseFrontendOrders(raw any, view string) []Order {
	orders := parseGenericOrders(raw)
	if view == "" {
		view = "open"
	}
	if view == "trigger" {
		out := make([]Order, 0, len(orders))
		for _, order := range orders {
			if isTriggerOrder(order.Kind) {
				out = append(out, order)
			}
		}
		return out
	}
	out := make([]Order, 0, len(orders))
	for _, order := range orders {
		if !isTriggerOrder(order.Kind) {
			out = append(out, order)
		}
	}
	return out
}

func parseHistoricalOrders(raw any) []Order {
	return parseGenericOrders(raw)
}

func parseGenericOrders(raw any) []Order {
	candidates := asArray(raw)
	if len(candidates) == 0 {
		if obj, ok := raw.(map[string]any); ok {
			candidates = asArray(obj["orders"], obj["openOrders"], obj["history"])
		}
	}
	out := make([]Order, 0, len(candidates))
	for _, item := range candidates {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if nested, ok := entry["order"].(map[string]any); ok {
			entry = nested
		}
		order := Order{
			ID:                asString(entry["oid"], entry["id"], entry["orderId"]),
			Oid:               firstNonEmptyString(asString(entry["oid"]), asString(entry["orderId"]), asString(entry["id"])),
			Symbol:            asString(entry["coin"], entry["symbol"]),
			Side:              normalizeSide(asString(entry["side"], entry["direction"])),
			Kind:              mapOrderKind(asString(entry["orderType"], entry["orderTypeStr"], entry["type"])),
			Size:              asString(entry["sz"], entry["size"]),
			Price:             firstNonEmptyString(asString(entry["px"], entry["price"], entry["limitPx"]), asString(entry["avgPx"])),
			TriggerPrice:      firstNonEmptyString(asString(entry["triggerPx"], entry["triggerPrice"]), asString(entry["tp"])),
			TriggerLimitPrice: asString(entry["tpslPx"], entry["triggerLimitPrice"], entry["limitPx"]),
			TimeInForce:       firstNonEmptyString(asString(entry["timeInForce"]), "gtc"),
			ReduceOnly:        asBool(entry["reduceOnly"]),
			Status:            normalizeOrderStatus(asString(entry["status"], entry["state"])),
			CreatedAt:         asInt64(entry["createdAt"], entry["createdTime"], entry["time"]),
			UpdatedAt:         asInt64(entry["updatedAt"], entry["modifiedAt"]),
			FilledSize:        asString(entry["filledSz"], entry["filled"], entry["filledSize"]),
			AveragePrice:      asString(entry["avgPx"], entry["avgPrice"]),
			Filled:            asString(entry["filledSz"], entry["filled"], entry["filledSize"]),
			ClientOrderID:     asString(entry["cloid"], entry["clientOrderId"], entry["clientId"]),
		}
		order.ID = firstNonEmptyString(order.Oid, order.ID)
		if order.TimeInForce == "" {
			order.TimeInForce = "gtc"
		}
		out = append(out, order)
	}
	return nonNil(out)
}

func parseUserFills(raw any) []Fill {
	candidates := asArray(raw)
	if len(candidates) == 0 {
		if obj, ok := raw.(map[string]any); ok {
			candidates = asArray(obj["fills"])
		}
	}
	out := make([]Fill, 0, len(candidates))
	for _, item := range candidates {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		fill := Fill{
			Symbol:  asString(entry["coin"], entry["symbol"]),
			Side:    normalizeSide(asString(entry["side"])),
			Size:    absString(asString(entry["sz"], entry["size"])),
			Px:      asString(entry["px"], entry["price"]),
			Fee:     asString(entry["fee"]),
			Ts:      asInt64(entry["time"], entry["ts"], entry["timestamp"]),
			FillID:  firstNonEmptyString(asString(entry["fillId"], entry["id"], entry["hash"])),
			OrderID: firstNonEmptyString(asString(entry["oid"], entry["orderId"]), asString(entry["id"])),
		}
		out = append(out, fill)
	}
	return nonNil(out)
}

func parseUserFunding(raw any) []FundingEvent {
	candidates := asArray(raw)
	if len(candidates) == 0 {
		if obj, ok := raw.(map[string]any); ok {
			candidates = asArray(obj["funding"], obj["history"], obj["events"])
		}
	}
	return parseFundingEvents(candidates)
}

func asArray(values ...any) []any {
	for _, value := range values {
		if arr, ok := value.([]any); ok {
			return arr
		}
		if value != nil {
			if arr, ok := value.([]interface{}); ok {
				return arr
			}
		}
	}
	return nil
}

func isSlice(v any) bool {
	if v == nil {
		return false
	}
	switch v.(type) {
	case []any:
		return true
	default:
		return false
	}
}

func asMap(value any) (map[string]any, error) {
	if obj, ok := value.(map[string]any); ok {
		return obj, nil
	}
	if obj, ok := value.(map[string]interface{}); ok {
		return map[string]any(obj), nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		return parsed, nil
	}
	return nil, errors.New("unexpected payload")
}

func asString(values ...any) string {
	for _, value := range values {
		switch v := value.(type) {
		case nil:
			continue
		case string:
			v = strings.TrimSpace(v)
			if v != "" {
				return v
			}
		case float64:
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				return trimDecimal(v)
			}
		case float32:
			return trimDecimal(float64(v))
		case int:
			return strconv.Itoa(v)
		case int64:
			return strconv.FormatInt(v, 10)
		case uint64:
			return strconv.FormatUint(v, 10)
		case json.Number:
			return v.String()
		}
	}
	return ""
}

func asInt(values ...any) int {
	for _, value := range values {
		switch v := value.(type) {
		case nil:
			continue
		case float64:
			return int(v)
		case float32:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		case json.Number:
			if i, err := v.Int64(); err == nil {
				return int(i)
			}
		case string:
			if v == "" {
				continue
			}
			if i, err := strconv.Atoi(v); err == nil {
				return i
			}
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return int(f)
			}
		}
	}
	return 0
}

func asInt64(values ...any) int64 {
	for _, value := range values {
		switch v := value.(type) {
		case nil:
			continue
		case float64:
			return int64(v)
		case float32:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		case uint64:
			if v > math.MaxInt64 {
				continue
			}
			return int64(v)
		case json.Number:
			if i, err := v.Int64(); err == nil {
				return i
			}
			if f, err := v.Float64(); err == nil {
				return int64(f)
			}
		case string:
			if s := strings.TrimSpace(v); s != "" {
				if i, err := strconv.ParseInt(s, 10, 64); err == nil {
					return i
				}
				if f, err := strconv.ParseFloat(s, 64); err == nil {
					return int64(f)
				}
			}
		}
	}
	return 0
}

func toInt64(values ...any) int64 {
	for _, value := range values {
		if value == nil {
			continue
		}
		if n := asInt64(value); n != 0 {
			return n
		}
	}
	return 0
}

func asBool(v any) bool {
	switch value := v.(type) {
	case nil:
		return false
	case bool:
		return value
	case string:
		v := strings.TrimSpace(strings.ToLower(value))
		return v == "true" || v == "1" || v == "yes"
	case float64:
		return value != 0
	case int:
		return value != 0
	case int64:
		return value != 0
	case uint64:
		return value != 0
	default:
		return false
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func maxInt(values ...int) int {
	max := 0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstMap(values ...any) map[string]any {
	for _, value := range values {
		if value == nil {
			continue
		}
		if obj, ok := value.(map[string]any); ok {
			return obj
		}
		if obj, ok := value.(map[string]interface{}); ok {
			return map[string]any(obj)
		}
	}
	return map[string]any{}
}

func chooseLeverageMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "isolated":
		return "isolated"
	case "cross":
		return "cross"
	default:
		return "cross"
	}
}

func chooseMarginType(v string) string {
	if strings.EqualFold(v, "isolated") {
		return "isolated"
	}
	return "cross"
}

func normalizeSide(in string) string {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "b":
		return "buy"
	case "s":
		return "sell"
	case "a":
		return "sell"
	case "buy":
		return "buy"
	case "sell":
		return "sell"
	default:
		return in
	}
}

func normalizeSideFromSize(size string) string {
	if strings.HasPrefix(strings.TrimSpace(size), "-") {
		return "sell"
	}
	return "buy"
}

func normalizeOrderStatus(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "open", "new", "active":
		return "open"
	case "filled", "closed":
		return "filled"
	case "cancelled", "canceled":
		return "cancelled"
	case "triggered":
		return "triggered"
	case "reduce-only":
		return "closed"
	default:
		return v
	}
}

func mapOrderKind(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "limit":
		return "limit"
	case "market":
		return "market"
	case "stop_market", "stopmarket", "stop":
		return "stopMarket"
	case "stop_limit", "stoplimit":
		return "stopLimit"
	case "take_profit_market", "takeprofitmarket", "takeprofit":
		return "takeProfitMarket"
	case "take_profit_limit", "takeprofitlimit":
		return "takeProfitLimit"
	default:
		return firstNonEmptyString(v, strings.ToLower(strings.TrimSpace(v)))
	}
}

func isTriggerOrder(kind string) bool {
	switch kind {
	case "stopMarket", "stopLimit", "takeProfitMarket", "takeProfitLimit":
		return true
	default:
		return false
	}
}

func rawValues(obj map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			return value
		}
	}
	return nil
}

func addCumulativeTotal(levels []OrderBookLevel) {
	var running float64
	for i := range levels {
		running += toFloat64(levels[i].Size)
		levels[i].Total = trimDecimal(running)
	}
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		value, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err == nil {
			return value
		}
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case uint64:
		return float64(n)
	case json.Number:
		f, err := n.Float64()
		if err == nil {
			return f
		}
	}
	return 0
}

func absString(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "-")
}

func trimDecimal(v float64) string {
	return trimFixedDecimal(strconv.FormatFloat(v, 'f', -1, 64))
}

func intervalToMs(interval string) int64 {
	switch strings.TrimSpace(interval) {
	case "1m":
		return 60 * 1000
	case "3m":
		return 3 * 60 * 1000
	case "5m":
		return 5 * 60 * 1000
	case "15m":
		return 15 * 60 * 1000
	case "30m":
		return 30 * 60 * 1000
	case "1h", "60m":
		return 60 * 60 * 1000
	case "2h":
		return 2 * 60 * 60 * 1000
	case "4h":
		return 4 * 60 * 60 * 1000
	case "6h":
		return 6 * 60 * 60 * 1000
	case "8h":
		return 8 * 60 * 60 * 1000
	case "12h":
		return 12 * 60 * 60 * 1000
	case "1d":
		return 24 * 60 * 60 * 1000
	case "1w":
		return 7 * 24 * 60 * 60 * 1000
	case "1M":
		return 30 * 24 * 60 * 60 * 1000
	default:
		return 0
	}
}

func nonNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

func sendEvent(out chan<- StreamEvent, event StreamEvent) {
	select {
	case out <- event:
	default:
	}
}

func waitOrDone(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
