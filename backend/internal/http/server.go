package apiv1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"log/slog"

	"fake-mex-backend/internal/config"
	"fake-mex-backend/internal/exchange"
	"fake-mex-backend/internal/rate"
	"fake-mex-backend/internal/stream"
	"fake-mex-backend/internal/validation"
)

var (
	allowedSides     = map[string]struct{}{"buy": {}, "sell": {}}
	allowedKinds     = map[string]struct{}{"limit": {}, "market": {}, "stopMarket": {}, "stopLimit": {}, "takeProfitMarket": {}, "takeProfitLimit": {}}
	allowedTIF       = map[string]struct{}{"gtc": {}, "ioc": {}, "alo": {}}
	allowedViews     = map[string]struct{}{"open": {}, "trigger": {}, "history": {}}
	allowedModes     = map[string]struct{}{"cross": {}, "isolated": {}}
	closeKinds       = map[string]struct{}{"market": {}, "limit": {}}
	allowedPercents  = map[int]struct{}{25: {}, 50: {}, 75: {}, 100: {}}
	allowedIntervals = map[string]struct{}{"1m": {}, "3m": {}, "5m": {}, "15m": {}, "30m": {}, "1h": {}, "2h": {}, "4h": {}, "8h": {}, "12h": {}, "1d": {}, "3d": {}, "1w": {}, "1M": {}}
	symbolRE         = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{0,31}$`)
)

type Server struct {
	cfg            config.Config
	client         exchange.ExchangeClient
	stream         *stream.Manager
	logger         *slog.Logger
	limiter        *rate.Limiter
	tradingEnabled atomic.Bool
}

func NewServer(cfg config.Config, client exchange.ExchangeClient, logger *slog.Logger) *Server {
	return &Server{
		cfg:     cfg,
		client:  client,
		stream:  stream.NewManager(client, logger),
		logger:  logger,
		limiter: rate.NewLimiter(30, 120),
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(LoggingMiddleware(s.logger))
	r.Use(RequestID)
	r.Use(s.limiter.Middleware)
	localOnly := config.IsLoopbackAddress(s.cfg.ServerAddr)
	r.Use(SecurityMiddleware(s.cfg.FrontendOrigin, localOnly, s.logger))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.healthHandler)
		r.Get("/trading", s.tradingStatusHandler)
		r.Put("/trading", s.tradingToggleHandler)
		r.Get("/bootstrap", s.bootstrapHandler)
		r.Get("/markets", s.marketsHandler)
		r.Get("/account", s.accountHandler)
		r.Get("/orders", s.ordersHandler)
		r.Get("/fills", s.fillsHandler)
		r.Get("/funding", s.fundingHandler)
		r.Post("/orders", s.createOrderHandler)
		r.Patch("/orders/{oid}", s.modifyOrderHandler)
		r.Delete("/orders/{oid}", s.cancelOrderHandler)
		r.Delete("/orders", s.cancelAllOrdersHandler)
		r.Put("/positions/{symbol}/leverage", s.leverageHandler)
		r.Post("/positions/{symbol}/close", s.closePositionHandler)
		r.Get("/stream", s.streamHandler)
	})
	return r
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	h, err := s.client.Health(ctx)
	if err != nil {
		h.Connected = false
		h.Upstream = "error"
		h.Backend = "ok"
	}
	h.AccountReady = s.cfg.AccountConfigured
	h.TradingAvailable = s.tradingAvailable()
	h.TradingEnabled = s.tradingEnabled.Load()
	h.Network = s.cfg.HLNetwork
	h.Timestamp = time.Now().UnixMilli()
	_ = writeJSON(w, http.StatusOK, h)
}

func (s *Server) tradingStatusHandler(w http.ResponseWriter, _ *http.Request) {
	_ = writeJSON(w, http.StatusOK, s.tradingStatus())
}

func (s *Server) tradingToggleHandler(w http.ResponseWriter, r *http.Request) {
	var request TradingToggleRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid payload", err.Error(), "invalid_json", nil)
		return
	}
	if request.Enabled && !s.tradingAvailable() {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/trading", "Trading unavailable", "signed testnet trading is not enabled in the backend configuration", "trading_unavailable", nil)
		return
	}
	s.tradingEnabled.Store(request.Enabled)
	_ = writeJSON(w, http.StatusOK, s.tradingStatus())
}

func (s *Server) tradingAvailable() bool {
	return s.cfg.TradingAllowed && s.cfg.AccountConfigured
}

func (s *Server) tradingStatus() TradingStatus {
	return TradingStatus{Available: s.tradingAvailable(), Enabled: s.tradingEnabled.Load(), Network: s.cfg.HLNetwork}
}

func (s *Server) requireTrading(w http.ResponseWriter, r *http.Request) bool {
	if !s.tradingAvailable() {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/trading", "Trading unavailable", "signed testnet trading is not enabled in the backend configuration", "trading_unavailable", nil)
		return false
	}
	if !s.tradingEnabled.Load() {
		s.handleProblem(w, r, http.StatusConflict, "https://fakemex.local/problems/trading", "Trading disabled", "enable the trading toggle before submitting this action", "trading_disabled", nil)
		return false
	}
	return true
}

func (s *Server) marketsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	markets, err := s.client.Markets(ctx)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/markets", "Market feed unavailable", err.Error(), "upstream_error", nil)
		return
	}
	_ = markets
	assetCtx, err := s.client.AssetContexts(ctx)
	if err != nil {
		s.logger.Warn("asset contexts unavailable", "error", err)
	}
	res := map[string]any{
		"markets":  markets,
		"contexts": assetCtx,
	}
	_ = writeJSON(w, http.StatusOK, res)
}

func (s *Server) bootstrapHandler(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	interval := strings.TrimSpace(r.URL.Query().Get("interval"))
	if interval == "" {
		interval = "15m"
	}
	if symbol == "" {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "symbol required", "symbol query parameter is required", "validation_error", map[string]string{"symbol": "required"})
		return
	}
	if !symbolRE.MatchString(symbol) {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid symbol", "symbol format is invalid", "validation_error", map[string]string{"symbol": "invalid"})
		return
	}
	if _, ok := allowedIntervals[interval]; !ok {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid interval", "interval is not supported", "validation_error", map[string]string{"interval": "invalid"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	snapshot, err := s.stream.Snapshot(ctx, symbol, interval)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/bootstrap", "Bootstrap failed", err.Error(), "upstream_error", nil)
		return
	}
	if s.cfg.AccountConfigured {
		s.enrichSnapshotWithAccount(ctx, &snapshot)
	}
	_ = writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) accountHandler(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AccountConfigured {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/account", "Account not configured", "master account credentials are missing", "account_not_configured", map[string]string{"account": "missing"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	acc, err := s.client.AccountSnapshot(ctx, s.cfg.HLAccountAddress)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/account", "Account unavailable", err.Error(), "upstream_error", nil)
		return
	}
	_ = writeJSON(w, http.StatusOK, acc)
}

func (s *Server) ordersHandler(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AccountConfigured {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/account", "Account not configured", "master account credentials are missing", "account_not_configured", map[string]string{"account": "missing"})
		return
	}
	view := strings.TrimSpace(r.URL.Query().Get("view"))
	if view == "" {
		view = "open"
	}
	if _, ok := allowedViews[view]; !ok {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid view", "view must be open|trigger|history", "validation", map[string]string{"view": "open|trigger|history"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	res, err := s.client.Orders(ctx, s.cfg.HLAccountAddress, view)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/orders", "Orders unavailable", err.Error(), "upstream_error", nil)
		return
	}
	_ = writeJSON(w, http.StatusOK, res)
}

func (s *Server) fillsHandler(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AccountConfigured {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/account", "Account not configured", "master account credentials are missing", "account_not_configured", map[string]string{"account": "missing"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	res, err := s.client.Fills(ctx, s.cfg.HLAccountAddress)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/fills", "Fills unavailable", err.Error(), "upstream_error", nil)
		return
	}
	_ = writeJSON(w, http.StatusOK, res)
}

func (s *Server) fundingHandler(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AccountConfigured {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/account", "Account not configured", "master account credentials are missing", "account_not_configured", map[string]string{"account": "missing"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	res, err := s.client.Funding(ctx, s.cfg.HLAccountAddress)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/funding", "Funding unavailable", err.Error(), "upstream_error", nil)
		return
	}
	_ = writeJSON(w, http.StatusOK, res)
}

func (s *Server) createOrderHandler(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AccountConfigured {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/account", "Account not configured", "master account credentials are missing", "account_not_configured", map[string]string{"account": "missing"})
		return
	}
	if !s.requireTrading(w, r) {
		return
	}
	defer r.Body.Close()
	var req exchange.OrderRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid payload", "invalid JSON body", "invalid_json", nil)
		return
	}
	if err := validateOrderRequest(req); err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Validation failed", err.Error(), "validation", nil)
		return
	}
	market, err := s.marketBySymbol(r.Context(), req.Symbol)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid symbol", err.Error(), "validation", map[string]string{"symbol": "invalid"})
		return
	}
	if err := validateOrderRequestPrecision(req, market); err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Validation failed", err.Error(), "validation", map[string]string{"symbol": req.Symbol})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	resp, err := s.client.PlaceOrder(ctx, req)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/trading", "Write failed", err.Error(), "upstream_error", nil)
		return
	}
	if resp.RequestID == "" {
		resp.RequestID = uuid.NewString()
	}
	_ = writeJSON(w, http.StatusOK, resp)
}

func (s *Server) modifyOrderHandler(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AccountConfigured {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/account", "Account not configured", "master account credentials are missing", "account_not_configured", map[string]string{"account": "missing"})
		return
	}
	if !s.requireTrading(w, r) {
		return
	}
	requestID := chi.URLParam(r, "oid")
	if requestID == "" {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Missing order id", "oid required", "validation", map[string]string{"oid": "required"})
		return
	}
	var req exchange.ModifyOrderRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid payload", "invalid JSON body", "invalid_json", nil)
		return
	}
	if err := validateModifyOrderRequest(req); err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Validation failed", err.Error(), "validation", nil)
		return
	}
	market, err := s.marketBySymbol(r.Context(), req.Symbol)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid symbol", err.Error(), "validation", map[string]string{"symbol": "invalid"})
		return
	}
	if err := validateModifyOrderRequestPrecision(req, market); err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Validation failed", err.Error(), "validation", map[string]string{"symbol": req.Symbol})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	resp, err := s.client.ModifyOrder(ctx, requestID, req)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/trading", "Modify failed", err.Error(), "upstream_error", nil)
		return
	}
	if resp.RequestID == "" {
		resp.RequestID = uuid.NewString()
	}
	_ = writeJSON(w, http.StatusOK, resp)
}

func (s *Server) cancelOrderHandler(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AccountConfigured {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/account", "Account not configured", "master account credentials are missing", "account_not_configured", map[string]string{"account": "missing"})
		return
	}
	if !s.requireTrading(w, r) {
		return
	}
	requestID := chi.URLParam(r, "oid")
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if symbol == "" {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Symbol required", "symbol query parameter required", "validation", map[string]string{"symbol": "required"})
		return
	}
	if !symbolRE.MatchString(symbol) {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid symbol", "symbol format is invalid", "validation_error", map[string]string{"symbol": "invalid"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	resp, err := s.client.CancelOrder(ctx, s.cfg.HLAccountAddress, symbol, requestID)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/trading", "Cancel failed", err.Error(), "upstream_error", nil)
		return
	}
	if resp.RequestID == "" {
		resp.RequestID = uuid.NewString()
	}
	_ = writeJSON(w, http.StatusOK, resp)
}

func (s *Server) cancelAllOrdersHandler(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AccountConfigured {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/account", "Account not configured", "master account credentials are missing", "account_not_configured", map[string]string{"account": "missing"})
		return
	}
	if !s.requireTrading(w, r) {
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	resp, err := s.client.CancelAllOrders(ctx, s.cfg.HLAccountAddress, symbol)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/trading", "Cancel failed", err.Error(), "upstream_error", nil)
		return
	}
	if resp.RequestID == "" {
		resp.RequestID = uuid.NewString()
	}
	_ = writeJSON(w, http.StatusOK, resp)
}

func (s *Server) leverageHandler(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AccountConfigured {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/account", "Account not configured", "master account credentials are missing", "account_not_configured", map[string]string{"account": "missing"})
		return
	}
	if !s.requireTrading(w, r) {
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "symbol")))
	var req exchange.LeverageRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid payload", "invalid JSON body", "invalid_json", nil)
		return
	}
	if err := validateLeverageRequest(req); err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Validation failed", err.Error(), "validation", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	resp, err := s.client.SetLeverage(ctx, s.cfg.HLAccountAddress, symbol, req)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/trading", "Leverage failed", err.Error(), "upstream_error", nil)
		return
	}
	if resp.RequestID == "" {
		resp.RequestID = uuid.NewString()
	}
	_ = writeJSON(w, http.StatusOK, resp)
}

func (s *Server) closePositionHandler(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AccountConfigured {
		s.handleProblem(w, r, http.StatusPreconditionFailed, "https://fakemex.local/problems/account", "Account not configured", "master account credentials are missing", "account_not_configured", map[string]string{"account": "missing"})
		return
	}
	if !s.requireTrading(w, r) {
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "symbol")))
	var req exchange.ClosePositionRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid payload", "invalid JSON body", "invalid_json", nil)
		return
	}
	if err := validateCloseRequest(req); err != nil {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Validation failed", err.Error(), "validation", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	resp, err := s.client.ClosePosition(ctx, s.cfg.HLAccountAddress, symbol, req)
	if err != nil {
		s.handleProblem(w, r, http.StatusBadGateway, "https://fakemex.local/problems/trading", "Close failed", err.Error(), "upstream_error", nil)
		return
	}
	if resp.RequestID == "" {
		resp.RequestID = uuid.NewString()
	}
	_ = writeJSON(w, http.StatusOK, resp)
}

func (s *Server) streamHandler(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	interval := strings.TrimSpace(r.URL.Query().Get("interval"))
	if interval == "" {
		interval = "15m"
	}
	if symbol == "" {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Symbol required", "symbol query parameter required", "validation", map[string]string{"symbol": "required"})
		return
	}
	if !symbolRE.MatchString(symbol) {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid symbol", "symbol format is invalid", "validation_error", map[string]string{"symbol": "invalid"})
		return
	}
	if _, ok := allowedIntervals[interval]; !ok {
		s.handleProblem(w, r, http.StatusBadRequest, "https://fakemex.local/problems/validation", "Invalid interval", "interval is not supported", "validation_error", map[string]string{"interval": "invalid"})
		return
	}
	upgrader := websocket.Upgrader{
		CheckOrigin: func(request *http.Request) bool {
			requestOrigin := request.Header.Get("Origin")
			return requestOrigin == s.cfg.FrontendOrigin || equivalentLoopbackOrigins(requestOrigin, s.cfg.FrontendOrigin)
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.handleProblem(w, r, http.StatusUpgradeRequired, "https://fakemex.local/problems/websocket", "Upgrade failed", err.Error(), "upgrade_error", nil)
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	sub, err := s.stream.Subscribe(ctx, symbol, interval)
	if err != nil {
		s.writeWSProblem(
			conn,
			ContextRequestID(r),
			http.StatusBadGateway,
			"https://fakemex.local/problems/stream",
			"Stream unavailable",
			"stream unavailable",
			map[string]string{"symbol": symbol},
			"upstream_error",
		)
		return
	}
	snapshot, err := s.stream.Snapshot(ctx, symbol, interval)
	if err != nil {
		s.writeWSProblem(
			conn,
			ContextRequestID(r),
			http.StatusBadGateway,
			"https://fakemex.local/problems/bootstrap",
			"Bootstrap unavailable",
			"snapshot unavailable",
			map[string]string{"symbol": symbol},
			"upstream_error",
		)
		return
	}
	_ = conn.WriteJSON(StreamEnvelope[exchange.Bootstrap]{Type: "snapshot", Sequence: 1, Symbol: symbol, ServerTime: time.Now().UnixMilli(), Data: snapshot})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(2*time.Second))
			}
		}
	}()
	seq := uint64(1)
	for event := range sub.Events {
		seq++
		if event.Symbol == "" {
			event.Symbol = symbol
		}
		_ = conn.WriteJSON(StreamEnvelope[any]{Type: string(event.Type), Symbol: event.Symbol, Sequence: seq, ServerTime: event.ServerTime, Data: event.Data})
	}
}

func (s *Server) handleProblem(w http.ResponseWriter, r *http.Request, status int, typ, title, detail, code string, fields map[string]string) {
	reqID := ContextRequestID(r)
	writeProblem(w, status, ProblemResponse{
		Type:      typ,
		Title:     title,
		Status:    status,
		Detail:    detail,
		Code:      code,
		RequestID: reqID,
		Fields:    fields,
	})
}

func (s *Server) writeWSProblem(
	conn *websocket.Conn,
	requestID string,
	status int,
	typ string,
	title string,
	detail string,
	fields map[string]string,
	code string,
) {
	_ = conn.WriteJSON(StreamEnvelope[ProblemResponse]{
		Type:       "error",
		Sequence:   1,
		ServerTime: time.Now().UnixMilli(),
		Data: ProblemResponse{
			Type:      typ,
			Title:     title,
			Status:    status,
			Detail:    detail,
			Code:      code,
			RequestID: requestID,
			Fields:    fields,
		},
	})
}

func (s *Server) enrichSnapshotWithAccount(ctx context.Context, snapshot *exchange.Bootstrap) {
	if snapshot == nil || !s.cfg.AccountConfigured {
		return
	}
	acc, err := s.client.AccountSnapshot(ctx, s.cfg.HLAccountAddress)
	if err != nil {
		s.logger.Warn("account snapshot unavailable", "error", err)
	} else {
		snapshot.Account = &acc
		snapshot.Assets = ensureSlice[exchange.Asset](acc.Assets)
	}
	open, err := s.client.Orders(ctx, s.cfg.HLAccountAddress, "open")
	if err != nil {
		s.logger.Warn("open orders unavailable", "error", err)
		open = make([]exchange.Order, 0)
	}
	trigger, err := s.client.Orders(ctx, s.cfg.HLAccountAddress, "trigger")
	if err != nil {
		s.logger.Warn("trigger orders unavailable", "error", err)
		trigger = make([]exchange.Order, 0)
	}
	history, err := s.client.Orders(ctx, s.cfg.HLAccountAddress, "history")
	if err != nil {
		s.logger.Warn("order history unavailable", "error", err)
		history = make([]exchange.Order, 0)
	}
	fills, err := s.client.Fills(ctx, s.cfg.HLAccountAddress)
	if err != nil {
		s.logger.Warn("fills unavailable", "error", err)
		fills = make([]exchange.Fill, 0)
	}
	funding, err := s.client.Funding(ctx, s.cfg.HLAccountAddress)
	if err != nil {
		s.logger.Warn("funding unavailable", "error", err)
		funding = make([]exchange.FundingEvent, 0)
	}
	snapshot.Orders = &exchange.OrdersSnapshot{
		Open:    ensureSlice(open),
		Trigger: ensureSlice(trigger),
		History: ensureSlice(history),
	}
	snapshot.Fills = ensureSlice(fills)
	snapshot.Funding = ensureSlice(funding)
	if snapshot.Assets == nil {
		snapshot.Assets = make([]exchange.Asset, 0)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func writeProblem(w http.ResponseWriter, status int, p ProblemResponse) {
	_ = writeJSON(w, status, p)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, destination any) error {
	if r.Body == nil {
		return errors.New("JSON body is required")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("invalid JSON body: multiple values are not allowed")
	}
	return nil
}

func ensureSlice[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func validateOrderRequest(req exchange.OrderRequest) error {
	if req.Symbol == "" {
		return errors.New("symbol is required")
	}
	if _, ok := allowedSides[strings.ToLower(req.Side)]; !ok {
		return errors.New("side must be buy or sell")
	}
	if _, ok := allowedKinds[req.Kind]; !ok {
		return errors.New("invalid kind")
	}
	if err := validation.ValidatePositiveDecimal(req.Size); err != nil {
		return err
	}
	switch req.Kind {
	case "limit":
		if req.Price == "" {
			return errors.New("price required")
		}
	case "stopMarket", "takeProfitMarket":
		if req.TriggerPrice == "" {
			return errors.New("triggerPrice required")
		}
	case "stopLimit", "takeProfitLimit":
		if req.TriggerPrice == "" {
			return errors.New("triggerPrice required")
		}
		if req.TriggerLimitPrice == "" && req.Price == "" {
			return errors.New("triggerLimitPrice required")
		}
	}
	if req.Price != "" {
		if err := validation.ValidatePositiveDecimal(req.Price); err != nil {
			return err
		}
	}
	if req.TriggerPrice != "" {
		if err := validation.ValidatePositiveDecimal(req.TriggerPrice); err != nil {
			return err
		}
	}
	if req.TriggerLimitPrice != "" {
		if err := validation.ValidatePositiveDecimal(req.TriggerLimitPrice); err != nil {
			return err
		}
	}
	if req.SlippagePercent != "" {
		if err := validateSlippage(req.SlippagePercent); err != nil {
			return err
		}
	}
	if req.TimeInForce != "" {
		if _, ok := allowedTIF[req.TimeInForce]; !ok {
			return errors.New("invalid timeInForce")
		}
	}
	if req.AttachedTakeProfit != nil || req.AttachedStopLoss != nil {
		if req.Kind != "limit" && req.Kind != "market" {
			return errors.New("attached TP/SL requires a limit or market parent order")
		}
		for _, attached := range []*exchange.AttachedOrder{req.AttachedTakeProfit, req.AttachedStopLoss} {
			if attached == nil {
				continue
			}
			if err := validation.ValidatePositiveDecimal(attached.TriggerPrice); err != nil {
				return fmt.Errorf("attached triggerPrice: %w", err)
			}
			if attached.LimitPrice != "" {
				if err := validation.ValidatePositiveDecimal(attached.LimitPrice); err != nil {
					return fmt.Errorf("attached limitPrice: %w", err)
				}
			}
		}
	}
	return nil
}

func validateModifyOrderRequest(req exchange.ModifyOrderRequest) error {
	if req.Symbol == "" {
		return errors.New("symbol is required")
	}
	if _, ok := allowedSides[strings.ToLower(req.Side)]; !ok {
		return errors.New("side must be buy or sell")
	}
	if err := validation.ValidatePositiveDecimal(req.Size); err != nil {
		return err
	}
	if err := validation.ValidatePositiveDecimal(req.Price); err != nil {
		return err
	}
	if req.TimeInForce != "" {
		if _, ok := allowedTIF[req.TimeInForce]; !ok {
			return errors.New("invalid timeInForce")
		}
	}
	return nil
}

func validateLeverageRequest(req exchange.LeverageRequest) error {
	if _, ok := allowedModes[req.Mode]; !ok {
		return errors.New("invalid mode")
	}
	if req.Leverage < 1 || req.Leverage > 100 {
		return errors.New("invalid leverage")
	}
	return nil
}

func validateCloseRequest(req exchange.ClosePositionRequest) error {
	if _, ok := closeKinds[req.Kind]; !ok {
		return errors.New("invalid kind")
	}
	if _, ok := allowedPercents[req.Percent]; !ok {
		return errors.New("invalid percent")
	}
	if req.Kind == "limit" && req.Price == "" {
		return errors.New("price required for limit close")
	}
	if req.Price != "" {
		if err := validation.ValidatePositiveDecimal(req.Price); err != nil {
			return err
		}
	}
	if req.SlippagePercent != "" {
		if err := validateSlippage(req.SlippagePercent); err != nil {
			return err
		}
	}
	return nil
}

func validateSlippage(value string) error {
	if err := validation.ValidatePositiveDecimal(value); err != nil {
		return err
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed > 5 {
		return errors.New("slippagePercent must be greater than 0 and at most 5")
	}
	return nil
}

func (s *Server) marketBySymbol(ctx context.Context, symbol string) (*exchange.Market, error) {
	markets, err := s.client.Markets(ctx)
	if err != nil {
		return nil, err
	}
	for _, market := range markets {
		if strings.EqualFold(market.Symbol, symbol) {
			copied := market
			return &copied, nil
		}
	}
	return nil, errors.New("symbol not found")
}

func validateOrderRequestPrecision(req exchange.OrderRequest, market *exchange.Market) error {
	if market == nil {
		return nil
	}
	if err := validation.ValidateScale(req.Size, market.SizePrecision); err != nil {
		return fmt.Errorf("size precision is %d: %w", market.SizePrecision, err)
	}
	for name, value := range map[string]string{
		"price":             req.Price,
		"triggerPrice":      req.TriggerPrice,
		"triggerLimitPrice": req.TriggerLimitPrice,
	} {
		if value != "" {
			if err := validateHyperliquidPrice(value, market.SizePrecision); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	for name, attached := range map[string]*exchange.AttachedOrder{
		"attachedTakeProfit": req.AttachedTakeProfit,
		"attachedStopLoss":   req.AttachedStopLoss,
	} {
		if attached == nil {
			continue
		}
		if err := validateHyperliquidPrice(attached.TriggerPrice, market.SizePrecision); err != nil {
			return fmt.Errorf("%s triggerPrice: %w", name, err)
		}
		if attached.LimitPrice != "" {
			if err := validateHyperliquidPrice(attached.LimitPrice, market.SizePrecision); err != nil {
				return fmt.Errorf("%s limitPrice: %w", name, err)
			}
		}
	}
	return nil
}

func validateModifyOrderRequestPrecision(req exchange.ModifyOrderRequest, market *exchange.Market) error {
	if market == nil {
		return nil
	}
	if err := validation.ValidateScale(req.Size, market.SizePrecision); err != nil {
		return fmt.Errorf("size precision is %d: %w", market.SizePrecision, err)
	}
	if err := validateHyperliquidPrice(req.Price, market.SizePrecision); err != nil {
		return fmt.Errorf("price: %w", err)
	}
	return nil
}

func validateHyperliquidPrice(value string, sizePrecision int) error {
	normalized := strings.TrimRight(strings.TrimRight(strings.TrimSpace(value), "0"), ".")
	if normalized == "" {
		normalized = "0"
	}
	parts := strings.SplitN(normalized, ".", 2)
	fractionDigits := 0
	if len(parts) == 2 {
		fractionDigits = len(parts[1])
	}
	maxDecimals := 6 - sizePrecision
	if maxDecimals < 0 {
		maxDecimals = 0
	}
	if fractionDigits > maxDecimals {
		return fmt.Errorf("too many decimals (max %d)", maxDecimals)
	}
	digits := strings.TrimLeft(strings.Join(parts, ""), "0")
	if len(parts) == 1 {
		digits = strings.TrimRight(digits, "0")
	}
	if len(digits) > 5 {
		return errors.New("more than 5 significant figures")
	}
	return nil
}
