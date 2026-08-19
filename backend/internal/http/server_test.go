package apiv1

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"log/slog"

	"fake-mex-backend/internal/config"
	"fake-mex-backend/internal/exchange"

	"github.com/gorilla/websocket"
)

type fakeExchange struct {
	HealthFn          func(context.Context) (exchange.Health, error)
	MarketsFn         func(context.Context) ([]exchange.Market, error)
	AssetContextsFn   func(context.Context) ([]exchange.AssetContext, error)
	BookFn            func(context.Context, string) (exchange.OrderBook, error)
	TradesFn          func(context.Context, string, int) ([]exchange.Trade, error)
	CandlesFn         func(context.Context, string, string, int) ([]exchange.Candle, error)
	AccountSnapshotFn func(context.Context, string) (exchange.Account, error)
	OrdersFn          func(context.Context, string, string) ([]exchange.Order, error)
	FillsFn           func(context.Context, string) ([]exchange.Fill, error)
	FundingFn         func(context.Context, string) ([]exchange.FundingEvent, error)
	PlaceOrderFn      func(context.Context, exchange.OrderRequest) (exchange.WriteResponse, error)
	ModifyOrderFn     func(context.Context, string, exchange.ModifyOrderRequest) (exchange.WriteResponse, error)
	CancelOrderFn     func(context.Context, string, string, string) (exchange.WriteResponse, error)
	CancelAllFn       func(context.Context, string, string) (exchange.WriteResponse, error)
	SetLeverageFn     func(context.Context, string, string, exchange.LeverageRequest) (exchange.WriteResponse, error)
	ClosePositionFn   func(context.Context, string, string, exchange.ClosePositionRequest) (exchange.WriteResponse, error)
	SubscribeFn       func(context.Context, []string, string) (<-chan exchange.StreamEvent, error)
	CloseFn           func()
}

func (f *fakeExchange) Health(ctx context.Context) (exchange.Health, error) {
	if f.HealthFn == nil {
		return exchange.Health{Backend: "up", Upstream: "connected", Connected: true}, nil
	}
	return f.HealthFn(ctx)
}

func (f *fakeExchange) Markets(ctx context.Context) ([]exchange.Market, error) {
	if f.MarketsFn == nil {
		return nil, nil
	}
	return f.MarketsFn(ctx)
}

func (f *fakeExchange) AssetContexts(ctx context.Context) ([]exchange.AssetContext, error) {
	if f.AssetContextsFn == nil {
		return nil, nil
	}
	return f.AssetContextsFn(ctx)
}

func (f *fakeExchange) Book(ctx context.Context, symbol string) (exchange.OrderBook, error) {
	if f.BookFn == nil {
		return exchange.OrderBook{Symbol: symbol}, nil
	}
	return f.BookFn(ctx, symbol)
}

func (f *fakeExchange) Trades(ctx context.Context, symbol string, limit int) ([]exchange.Trade, error) {
	if f.TradesFn == nil {
		return nil, nil
	}
	return f.TradesFn(ctx, symbol, limit)
}

func (f *fakeExchange) Candles(ctx context.Context, symbol, interval string, limit int) ([]exchange.Candle, error) {
	if f.CandlesFn == nil {
		return nil, nil
	}
	return f.CandlesFn(ctx, symbol, interval, limit)
}

func (f *fakeExchange) AccountSnapshot(ctx context.Context, address string) (exchange.Account, error) {
	if f.AccountSnapshotFn == nil {
		return exchange.Account{}, nil
	}
	return f.AccountSnapshotFn(ctx, address)
}

func (f *fakeExchange) Orders(ctx context.Context, address, view string) ([]exchange.Order, error) {
	if f.OrdersFn == nil {
		return nil, nil
	}
	return f.OrdersFn(ctx, address, view)
}

func (f *fakeExchange) Fills(ctx context.Context, address string) ([]exchange.Fill, error) {
	if f.FillsFn == nil {
		return nil, nil
	}
	return f.FillsFn(ctx, address)
}

func (f *fakeExchange) Funding(ctx context.Context, address string) ([]exchange.FundingEvent, error) {
	if f.FundingFn == nil {
		return nil, nil
	}
	return f.FundingFn(ctx, address)
}

func (f *fakeExchange) PlaceOrder(ctx context.Context, req exchange.OrderRequest) (exchange.WriteResponse, error) {
	if f.PlaceOrderFn == nil {
		return exchange.WriteResponse{}, nil
	}
	return f.PlaceOrderFn(ctx, req)
}

func (f *fakeExchange) ModifyOrder(ctx context.Context, oid string, req exchange.ModifyOrderRequest) (exchange.WriteResponse, error) {
	if f.ModifyOrderFn == nil {
		return exchange.WriteResponse{}, nil
	}
	return f.ModifyOrderFn(ctx, oid, req)
}

func (f *fakeExchange) CancelOrder(ctx context.Context, address, symbol, oid string) (exchange.WriteResponse, error) {
	if f.CancelOrderFn == nil {
		return exchange.WriteResponse{}, nil
	}
	return f.CancelOrderFn(ctx, address, symbol, oid)
}

func (f *fakeExchange) CancelAllOrders(ctx context.Context, address, symbol string) (exchange.WriteResponse, error) {
	if f.CancelAllFn == nil {
		return exchange.WriteResponse{}, nil
	}
	return f.CancelAllFn(ctx, address, symbol)
}

func (f *fakeExchange) SetLeverage(ctx context.Context, address, symbol string, req exchange.LeverageRequest) (exchange.WriteResponse, error) {
	if f.SetLeverageFn == nil {
		return exchange.WriteResponse{}, nil
	}
	return f.SetLeverageFn(ctx, address, symbol, req)
}

func (f *fakeExchange) ClosePosition(ctx context.Context, address, symbol string, req exchange.ClosePositionRequest) (exchange.WriteResponse, error) {
	if f.ClosePositionFn == nil {
		return exchange.WriteResponse{}, nil
	}
	return f.ClosePositionFn(ctx, address, symbol, req)
}

func (f *fakeExchange) Subscribe(ctx context.Context, symbols []string, interval string) (<-chan exchange.StreamEvent, error) {
	if f.SubscribeFn == nil {
		ch := make(chan exchange.StreamEvent)
		close(ch)
		return ch, nil
	}
	return f.SubscribeFn(ctx, symbols, interval)
}

func (f *fakeExchange) Close() {
	if f.CloseFn != nil {
		f.CloseFn()
	}
}

func TestAccountEndpointRequiresConfiguredAccount(t *testing.T) {
	t.Parallel()
	client := &fakeExchange{}
	cfg := baseConfig()
	s := NewServer(cfg, client, testingLogger())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/account", nil)
	req.Header.Set("X-Request-ID", "req-1")
	setLocalRemoteAddr(req)

	s.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected %d, got %d", http.StatusPreconditionFailed, rec.Code)
	}
	var p exchange.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if p.Code != "account_not_configured" {
		t.Fatalf("unexpected code %q", p.Code)
	}
	if p.RequestID != "req-1" {
		t.Fatalf("expected request id req-1 got %q", p.RequestID)
	}
}

func TestMarketsEndpointWorksWhenAccountMissing(t *testing.T) {
	t.Parallel()
	client := &fakeExchange{
		MarketsFn: func(context.Context) ([]exchange.Market, error) {
			return []exchange.Market{{Symbol: "BTC"}}, nil
		},
		AssetContextsFn: func(context.Context) ([]exchange.AssetContext, error) {
			return []exchange.AssetContext{{Symbol: "BTC"}}, nil
		},
	}
	cfg := baseConfig()
	s := NewServer(cfg, client, testingLogger())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/markets", nil)
	setLocalRemoteAddr(req)
	s.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d got %d", http.StatusOK, rec.Code)
	}
	var payload struct {
		Markets  []exchange.Market       `json:"markets"`
		Contexts []exchange.AssetContext `json:"contexts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Markets) != 1 || len(payload.Contexts) != 1 {
		t.Fatalf("unexpected payload %#v", payload)
	}
}

func TestCreateOrderRejectsInvalidPrecision(t *testing.T) {
	t.Parallel()

	var calledPlace int32
	client := &fakeExchange{
		MarketsFn: func(context.Context) ([]exchange.Market, error) {
			return []exchange.Market{{Symbol: "BTC", SizePrecision: 1, PricePrecision: 1}}, nil
		},
		PlaceOrderFn: func(context.Context, exchange.OrderRequest) (exchange.WriteResponse, error) {
			atomic.AddInt32(&calledPlace, 1)
			return exchange.WriteResponse{Status: "accepted"}, nil
		},
	}
	cfg := baseConfig()
	cfg.AccountConfigured = true
	cfg.HLAccountAddress = "0xabc"
	cfg.TradingAllowed = true
	s := NewServer(cfg, client, testingLogger())
	s.tradingEnabled.Store(true)

	body := `{"symbol":"BTC","side":"buy","kind":"limit","size":"1.11","price":"1.0","reduceOnly":false}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	setLocalRemoteAddr(req)
	s.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d got %d body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if atomic.LoadInt32(&calledPlace) != 0 {
		t.Fatal("place order should not be called on precision failure")
	}
}

func TestCreateOrderGeneratesRequestID(t *testing.T) {
	t.Parallel()

	client := &fakeExchange{
		MarketsFn: func(context.Context) ([]exchange.Market, error) {
			return []exchange.Market{{Symbol: "BTC", SizePrecision: 2, PricePrecision: 2}}, nil
		},
		PlaceOrderFn: func(context.Context, exchange.OrderRequest) (exchange.WriteResponse, error) {
			return exchange.WriteResponse{Status: "accepted", OrderID: "abc"}, nil
		},
	}
	cfg := baseConfig()
	cfg.AccountConfigured = true
	cfg.HLAccountAddress = "0xabc"
	cfg.TradingAllowed = true
	s := NewServer(cfg, client, testingLogger())
	s.tradingEnabled.Store(true)

	body := `{"symbol":"BTC","side":"buy","kind":"limit","size":"1.12","price":"100.00","reduceOnly":false}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	setLocalRemoteAddr(req)
	s.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var out exchange.WriteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.RequestID == "" {
		t.Fatal("expected request ID")
	}
}

func TestStateChangingOriginMustMatchConfiguredOrigin(t *testing.T) {
	t.Parallel()
	client := &fakeExchange{
		MarketsFn: func(context.Context) ([]exchange.Market, error) {
			return []exchange.Market{{Symbol: "BTC", SizePrecision: 2, PricePrecision: 2}}, nil
		},
	}
	cfg := baseConfig()
	cfg.AccountConfigured = true
	cfg.HLAccountAddress = "0xabc"
	s := NewServer(cfg, client, testingLogger())

	body := `{"symbol":"BTC","side":"buy","kind":"limit","size":"1.00","price":"100.00","reduceOnly":false}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Origin", "http://evil.example")
	setLocalRemoteAddr(req)
	s.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d got %d", http.StatusForbidden, rec.Code)
	}
}

func TestTradingStartsDisabledAndMustBeArmed(t *testing.T) {
	t.Parallel()

	var calledPlace int32
	client := &fakeExchange{
		MarketsFn: func(context.Context) ([]exchange.Market, error) {
			return []exchange.Market{{Symbol: "BTC", SizePrecision: 2, PricePrecision: 2}}, nil
		},
		PlaceOrderFn: func(context.Context, exchange.OrderRequest) (exchange.WriteResponse, error) {
			atomic.AddInt32(&calledPlace, 1)
			return exchange.WriteResponse{Status: "ok"}, nil
		},
	}
	cfg := baseConfig()
	cfg.AccountConfigured = true
	cfg.TradingAllowed = true
	s := NewServer(cfg, client, testingLogger())
	router := s.Router()

	body := `{"symbol":"BTC","side":"buy","kind":"limit","size":"1.00","price":"100.00","reduceOnly":false}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	setLocalRemoteAddr(req)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected disabled status %d got %d body=%s", http.StatusConflict, rec.Code, rec.Body.String())
	}
	if atomic.LoadInt32(&calledPlace) != 0 {
		t.Fatal("disabled trading must not call exchange")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/trading", strings.NewReader(`{"enabled":true}`))
	setLocalRemoteAddr(req)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected toggle status %d got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	setLocalRemoteAddr(req)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected armed order status %d got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if atomic.LoadInt32(&calledPlace) != 1 {
		t.Fatal("armed trading should call exchange once")
	}
}

func TestTradingCannotBeArmedWhenBackendDisallowsIt(t *testing.T) {
	t.Parallel()

	s := NewServer(baseConfig(), &fakeExchange{}, testingLogger())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/trading", strings.NewReader(`{"enabled":true}`))
	setLocalRemoteAddr(req)
	s.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected %d got %d body=%s", http.StatusPreconditionFailed, rec.Code, rec.Body.String())
	}
}

func TestNetworkSwitchStartsOnTestnetAndDisarmsTrading(t *testing.T) {
	t.Parallel()

	testnet := baseConfig()
	testnet.AccountConfigured = true
	testnet.TradingAllowed = true
	mainnet := testnet
	mainnet.HLNetwork = config.NetworkMainnet
	mainnet.HLAPIURL = config.MainnetAPIURL
	mainnet.HLWsURL = config.MainnetWSURL

	s, err := NewNetworkServer(testnet, map[string]NetworkBinding{
		config.NetworkTestnet: {Config: testnet, Client: &fakeExchange{}, Configured: true, TradingAvailable: true},
		config.NetworkMainnet: {Config: mainnet, Client: &fakeExchange{}, Configured: true, TradingAvailable: true},
	}, config.NetworkTestnet, testingLogger())
	if err != nil {
		t.Fatalf("NewNetworkServer failed: %v", err)
	}
	router := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network", nil)
	setLocalRemoteAddr(req)
	router.ServeHTTP(rec, req)
	var initial NetworkStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial status: %v", err)
	}
	if rec.Code != http.StatusOK || initial.Network != config.NetworkTestnet {
		t.Fatalf("expected testnet startup, status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/trading", strings.NewReader(`{"enabled":true}`))
	setLocalRemoteAddr(req)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("arm testnet trading: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/network", strings.NewReader(`{"network":"mainnet"}`))
	setLocalRemoteAddr(req)
	router.ServeHTTP(rec, req)
	var switched NetworkStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &switched); err != nil {
		t.Fatalf("decode switched status: %v", err)
	}
	if rec.Code != http.StatusOK || switched.Network != config.NetworkMainnet || switched.TradingEnabled || !switched.TradingAvailable {
		t.Fatalf("expected configured, unarmed mainnet, status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/trading", strings.NewReader(`{"enabled":true}`))
	setLocalRemoteAddr(req)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected configured mainnet trading to arm, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNetworkSwitchRejectsUnconfiguredMainnet(t *testing.T) {
	t.Parallel()

	testnet := baseConfig()
	testnet.AccountConfigured = true
	testnet.TradingAllowed = true
	mainnet := testnet
	mainnet.HLNetwork = config.NetworkMainnet
	mainnet.HLAPIURL = config.MainnetAPIURL
	mainnet.HLWsURL = config.MainnetWSURL
	mainnet.HLAccountAddress = ""
	mainnet.HLAPIWalletAddress = ""
	mainnet.AccountConfigured = false

	s, err := NewNetworkServer(testnet, map[string]NetworkBinding{
		config.NetworkTestnet: {Config: testnet, Client: &fakeExchange{}, Configured: true, TradingAvailable: true},
		config.NetworkMainnet: {Config: mainnet, Client: &fakeExchange{}, Configured: false, TradingAvailable: false},
	}, config.NetworkTestnet, testingLogger())
	if err != nil {
		t.Fatalf("NewNetworkServer failed: %v", err)
	}
	s.tradingEnabled.Store(true)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/network", strings.NewReader(`{"network":"mainnet"}`))
	setLocalRemoteAddr(req)
	s.Router().ServeHTTP(rec, req)

	var problem ProblemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if rec.Code != http.StatusPreconditionFailed || problem.Code != "network_unavailable" {
		t.Fatalf("expected mainnet configuration error, status=%d body=%s", rec.Code, rec.Body.String())
	}
	if s.current().cfg.HLNetwork != config.NetworkTestnet || !s.tradingEnabled.Load() {
		t.Fatal("rejected switch must leave the active testnet runtime unchanged")
	}
	status := s.networkStatus(s.current())
	if len(status.AvailableNetworks) != 1 || status.AvailableNetworks[0] != config.NetworkTestnet {
		t.Fatalf("unconfigured mainnet must not be reported as available: %#v", status.AvailableNetworks)
	}
}

func TestConfiguredMainnetCanBeSelectedWhenProcessTradingIsDisabled(t *testing.T) {
	t.Parallel()

	testnet := baseConfig()
	mainnet := testnet
	mainnet.HLNetwork = config.NetworkMainnet
	mainnet.HLAPIURL = config.MainnetAPIURL
	mainnet.HLWsURL = config.MainnetWSURL

	s, err := NewNetworkServer(testnet, map[string]NetworkBinding{
		config.NetworkTestnet: {Config: testnet, Client: &fakeExchange{}, Configured: true, TradingAvailable: false},
		config.NetworkMainnet: {Config: mainnet, Client: &fakeExchange{}, Configured: true, TradingAvailable: false},
	}, config.NetworkTestnet, testingLogger())
	if err != nil {
		t.Fatalf("NewNetworkServer failed: %v", err)
	}
	router := s.Router()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/network", strings.NewReader(`{"network":"mainnet"}`))
	setLocalRemoteAddr(req)
	router.ServeHTTP(rec, req)

	var status NetworkStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode network status: %v", err)
	}
	if rec.Code != http.StatusOK || status.Network != config.NetworkMainnet || status.TradingAvailable || status.TradingEnabled {
		t.Fatalf("expected read-only configured mainnet, status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/trading", strings.NewReader(`{"enabled":true}`))
	setLocalRemoteAddr(req)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("process trading gate must apply on mainnet, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNetworkSwitchWaitsForInflightSignedAction(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	testnetClient := &fakeExchange{
		MarketsFn: func(context.Context) ([]exchange.Market, error) {
			return []exchange.Market{{Symbol: "BTC", SizePrecision: 2, PricePrecision: 2}}, nil
		},
		PlaceOrderFn: func(context.Context, exchange.OrderRequest) (exchange.WriteResponse, error) {
			close(started)
			<-release
			return exchange.WriteResponse{Status: "ok"}, nil
		},
	}
	testnet := baseConfig()
	testnet.AccountConfigured = true
	testnet.TradingAllowed = true
	mainnet := testnet
	mainnet.HLNetwork = config.NetworkMainnet
	mainnet.HLAPIURL = config.MainnetAPIURL
	mainnet.HLWsURL = config.MainnetWSURL

	s, err := NewNetworkServer(testnet, map[string]NetworkBinding{
		config.NetworkTestnet: {Config: testnet, Client: testnetClient, Configured: true, TradingAvailable: true},
		config.NetworkMainnet: {Config: mainnet, Client: &fakeExchange{}, Configured: true, TradingAvailable: true},
	}, config.NetworkTestnet, testingLogger())
	if err != nil {
		t.Fatalf("NewNetworkServer failed: %v", err)
	}
	s.tradingEnabled.Store(true)
	router := s.Router()

	orderDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(`{"symbol":"BTC","side":"buy","kind":"limit","size":"1.00","price":"100.00","reduceOnly":false}`))
		setLocalRemoteAddr(req)
		router.ServeHTTP(rec, req)
		orderDone <- rec
	}()
	<-started

	switchDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/network", strings.NewReader(`{"network":"mainnet"}`))
		setLocalRemoteAddr(req)
		router.ServeHTTP(rec, req)
		switchDone <- rec
	}()
	select {
	case rec := <-switchDone:
		close(release)
		<-orderDone
		t.Fatalf("network switched while a signed action was still in flight: status=%d", rec.Code)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	if rec := <-orderDone; rec.Code != http.StatusOK {
		t.Fatalf("in-flight order failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := <-switchDone; rec.Code != http.StatusOK {
		t.Fatalf("network switch failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if s.tradingEnabled.Load() {
		t.Fatal("network switch must disarm trading")
	}
}

func TestStateChangingOriginIsRequired(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/trading", strings.NewReader(`{"enabled":false}`))
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	NewServer(baseConfig(), &fakeExchange{}, testingLogger()).Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d got %d", http.StatusForbidden, rec.Code)
	}
}

func TestStreamFirstMessageIsSnapshot(t *testing.T) {
	t.Parallel()

	upstream := make(chan exchange.StreamEvent, 1)
	client := &fakeExchange{
		MarketsFn: func(context.Context) ([]exchange.Market, error) {
			return []exchange.Market{{Symbol: "BTC", SizePrecision: 2, PricePrecision: 2}}, nil
		},
		BookFn: func(context.Context, string) (exchange.OrderBook, error) {
			return exchange.OrderBook{Symbol: "BTC"}, nil
		},
		TradesFn: func(context.Context, string, int) ([]exchange.Trade, error) {
			return []exchange.Trade{{Symbol: "BTC", Side: "buy", Size: "1", Price: "100"}}, nil
		},
		CandlesFn: func(context.Context, string, string, int) ([]exchange.Candle, error) {
			return []exchange.Candle{{Symbol: "BTC", Open: "99", Close: "100"}}, nil
		},
		SubscribeFn: func(context.Context, []string, string) (<-chan exchange.StreamEvent, error) {
			return upstream, nil
		},
	}
	server := httptest.NewServer(NewServer(baseConfig(), client, testingLogger()).Router())
	defer server.Close()

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1) + "/api/v1/stream?symbol=BTC&interval=15m"
	headers := http.Header{}
	headers.Set("Origin", "http://localhost:4200")
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			t.Fatalf("websocket dial: %v status=%d body=%s", err, resp.StatusCode, string(body))
		}
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close()

	var msg struct {
		Type       string          `json:"type"`
		Sequence   uint64          `json:"sequence"`
		ServerTime int64           `json:"serverTime"`
		Symbol     string          `json:"symbol"`
		Data       json.RawMessage `json:"data"`
	}
	if _, raw, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read websocket message: %v", err)
	} else if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("decode websocket message: %v", err)
	}
	if msg.Type != "snapshot" {
		t.Fatalf("expected snapshot first message, got %s", msg.Type)
	}
	if msg.Sequence != 1 {
		t.Fatalf("expected sequence 1, got %d", msg.Sequence)
	}
	if len(msg.Data) == 0 {
		t.Fatalf("expected snapshot data")
	}
}

func baseConfig() config.Config {
	return config.Config{
		ServerAddr:             "127.0.0.1:8080",
		FrontendOrigin:         "http://localhost:4200",
		HLAPIURL:               "https://api.hyperliquid-testnet.xyz",
		HLWsURL:                "wss://api.hyperliquid-testnet.xyz/ws",
		HLNetwork:              "testnet",
		LogLevel:               "info",
		AccountConfigured:      false,
		HLAccountAddress:       "0x123",
		HLAPIWalletAddress:     "0x456",
		HLAPIWalletPrivateKey:  "0x789",
		AutoBuilderFeeDisabled: true,
	}
}

func testingLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func setLocalRemoteAddr(req *http.Request) {
	req.RemoteAddr = "127.0.0.1:1234"
	if req.Header.Get("Origin") == "" {
		req.Header.Set("Origin", "http://localhost:4200")
	}
}
