package stream

import (
	"context"
	"fmt"
	"sync"
	"time"

	"log/slog"

	"fake-mex-backend/internal/exchange"
)

type Manager struct {
	client exchange.ExchangeClient
	logger *slog.Logger

	mu    sync.Mutex
	feeds map[string]*feed
}

type feed struct {
	symbol   string
	interval string
	clients  map[chan<- exchange.StreamEvent]struct{}
	sequence uint64
	cancel   context.CancelFunc
}

type Subscription struct {
	Events <-chan exchange.StreamEvent
	Close  func()
}

func NewManager(client exchange.ExchangeClient, logger *slog.Logger) *Manager {
	return &Manager{
		client: client,
		logger: logger,
		feeds:  map[string]*feed{},
	}
}

func (m *Manager) Snapshot(ctx context.Context, symbol, interval string) (exchange.Bootstrap, error) {
	markets, err := m.client.Markets(ctx)
	if err != nil {
		return exchange.Bootstrap{}, err
	}
	book, err := m.client.Book(ctx, symbol)
	if err != nil {
		return exchange.Bootstrap{}, err
	}
	trades, _ := m.client.Trades(ctx, symbol, 50)
	candles, _ := m.client.Candles(ctx, symbol, interval, 200)
	levelBook := flattenLevels(book.Bids, book.Asks)
	return exchange.Bootstrap{
		Markets: markets,
		Market:  firstMarketBySymbol(markets, symbol),
		Book:    levelBook,
		Trades:  trades,
		Candles: candles,
		Account: &exchange.Account{
			Margin: exchange.Margin{
				CrossBalance:     "0",
				AvailableBalance: "0",
				TotalMarginUsed:  "0",
			},
			LeverageMode: "cross",
			Positions:    []exchange.Position{},
			Balances:     []exchange.Asset{},
			Assets:       []exchange.Asset{},
		},
		Orders:  &exchange.OrdersSnapshot{Open: []exchange.Order{}, Trigger: []exchange.Order{}, History: []exchange.Order{}},
		Fills:   []exchange.Fill{},
		Funding: []exchange.FundingEvent{},
		Assets:  []exchange.Asset{},
	}, nil
}

func flattenLevels(bids, asks []exchange.OrderBookLevel) []exchange.OrderBookLevel {
	out := make([]exchange.OrderBookLevel, 0, len(bids)+len(asks))
	for i := range bids {
		level := bids[i]
		level.Side = "buy"
		out = append(out, level)
	}
	for i := range asks {
		level := asks[i]
		level.Side = "sell"
		out = append(out, level)
	}
	return out
}

func firstMarketBySymbol(markets []exchange.Market, symbol string) *exchange.Market {
	for _, m := range markets {
		if m.Symbol == symbol {
			copied := m
			return &copied
		}
	}
	return nil
}

func (m *Manager) Subscribe(ctx context.Context, symbol, interval string) (Subscription, error) {
	if symbol == "" {
		return Subscription{}, fmt.Errorf("symbol is required")
	}
	if interval == "" {
		interval = "15m"
	}

	out := make(chan exchange.StreamEvent, 128)
	key := m.key(symbol, interval)

	m.mu.Lock()
	f, ok := m.feeds[key]
	if !ok {
		f = &feed{
			symbol:   symbol,
			interval: interval,
			clients:  map[chan<- exchange.StreamEvent]struct{}{},
		}
		m.feeds[key] = f
		m.startFeedLocked(key, f)
	}
	f.clients[out] = struct{}{}
	m.mu.Unlock()

	closeFn := func() {
		m.mu.Lock()
		if current, ok := m.feeds[key]; ok {
			delete(current.clients, out)
			if len(current.clients) == 0 && current.cancel != nil {
				current.cancel()
				delete(m.feeds, key)
			}
		}
		m.mu.Unlock()
		close(out)
	}

	return Subscription{Events: out, Close: closeFn}, nil
}

func (m *Manager) key(symbol, interval string) string {
	return symbol + "|" + interval
}

func (m *Manager) startFeedLocked(key string, f *feed) {
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel

	go func() {
		defer func() {
			f.cancel()
			m.mu.Lock()
			if current, ok := m.feeds[key]; ok && current == f {
				for c := range current.clients {
					close(c)
				}
				delete(m.feeds, key)
			}
			m.mu.Unlock()
		}()

	outer:
		for {
			if ctx.Err() != nil {
				return
			}

			events, err := m.client.Subscribe(ctx, []string{f.symbol}, f.interval)
			if err != nil {
				m.broadcastConnectionEvent(f, "disconnected", err.Error())
				m.logger.Warn("stream subscribe failed; reconnecting", "key", key, "error", err)
				if !waitOrDone(ctx, 2*time.Second) {
					return
				}
				continue
			}

			m.broadcastConnectionEvent(f, "connected", "upstream connection active")
			for {
				select {
				case <-ctx.Done():
					return
				case ev, ok := <-events:
					if !ok {
						if !waitOrDone(ctx, 2*time.Second) {
							return
						}
						continue outer
					}
					m.broadcastEvent(f, key, ev)
				}
			}
		}
	}()
}

func (m *Manager) broadcastEvent(f *feed, key string, ev exchange.StreamEvent) {
	if ev.Sequence == 0 {
		f.sequence++
		ev.Sequence = f.sequence
	}
	if ev.ServerTime == 0 {
		ev.ServerTime = time.Now().UnixMilli()
	}

	m.mu.Lock()
	for out := range f.clients {
		copy := ev
		select {
		case out <- copy:
		default:
			m.logger.Warn("dropping stream event due slow consumer", "key", key, "client", out)
		}
	}
	m.mu.Unlock()
}

func (m *Manager) broadcastConnectionEvent(f *feed, status, detail string) {
	ev := exchange.StreamEvent{
		Type:       exchange.EventConnection,
		ServerTime: time.Now().UnixMilli(),
		Data: map[string]any{
			"status": status,
			"detail": detail,
		},
	}
	m.broadcastEvent(f, m.key(f.symbol, f.interval), ev)
}

func waitOrDone(ctx context.Context, delay time.Duration) bool {
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
