package stream_test

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"fake-mex-backend/internal/exchange"
	"fake-mex-backend/internal/stream"
	"log/slog"
)

type fakeExchange struct {
	marketsFn      func(context.Context) ([]exchange.Market, error)
	bookFn         func(context.Context, string) (exchange.OrderBook, error)
	tradesFn       func(context.Context, string, int) ([]exchange.Trade, error)
	candlesFn      func(context.Context, string, string, int) ([]exchange.Candle, error)
	subscribeFn    func(context.Context, []string, string) (<-chan exchange.StreamEvent, error)
	cleanupCounter uint64
}

func (f *fakeExchange) Health(context.Context) (exchange.Health, error) {
	return exchange.Health{Backend: "up", Upstream: "connected", Connected: true}, nil
}

func (f *fakeExchange) Markets(ctx context.Context) ([]exchange.Market, error) {
	if f.marketsFn == nil {
		return nil, nil
	}
	return f.marketsFn(ctx)
}

func (f *fakeExchange) AssetContexts(ctx context.Context) ([]exchange.AssetContext, error) {
	return nil, nil
}

func (f *fakeExchange) Book(ctx context.Context, symbol string) (exchange.OrderBook, error) {
	if f.bookFn == nil {
		return exchange.OrderBook{Symbol: symbol}, nil
	}
	return f.bookFn(ctx, symbol)
}

func (f *fakeExchange) Trades(ctx context.Context, symbol string, limit int) ([]exchange.Trade, error) {
	if f.tradesFn == nil {
		return nil, nil
	}
	return f.tradesFn(ctx, symbol, limit)
}

func (f *fakeExchange) Candles(ctx context.Context, symbol, interval string, limit int) ([]exchange.Candle, error) {
	if f.candlesFn == nil {
		return nil, nil
	}
	return f.candlesFn(ctx, symbol, interval, limit)
}

func (f *fakeExchange) AccountSnapshot(context.Context, string) (exchange.Account, error) {
	return exchange.Account{}, errors.New("account snapshot unavailable in fake exchange")
}

func (f *fakeExchange) Orders(context.Context, string, string) ([]exchange.Order, error) {
	return nil, nil
}

func (f *fakeExchange) Fills(context.Context, string) ([]exchange.Fill, error) {
	return nil, nil
}

func (f *fakeExchange) Funding(context.Context, string) ([]exchange.FundingEvent, error) {
	return nil, nil
}

func (f *fakeExchange) PlaceOrder(context.Context, exchange.OrderRequest) (exchange.WriteResponse, error) {
	return exchange.WriteResponse{}, nil
}

func (f *fakeExchange) ModifyOrder(context.Context, string, exchange.ModifyOrderRequest) (exchange.WriteResponse, error) {
	return exchange.WriteResponse{}, nil
}

func (f *fakeExchange) CancelOrder(context.Context, string, string, string) (exchange.WriteResponse, error) {
	return exchange.WriteResponse{}, nil
}

func (f *fakeExchange) CancelAllOrders(context.Context, string, string) (exchange.WriteResponse, error) {
	return exchange.WriteResponse{}, nil
}

func (f *fakeExchange) SetLeverage(context.Context, string, string, exchange.LeverageRequest) (exchange.WriteResponse, error) {
	return exchange.WriteResponse{}, nil
}

func (f *fakeExchange) ClosePosition(context.Context, string, string, exchange.ClosePositionRequest) (exchange.WriteResponse, error) {
	return exchange.WriteResponse{}, nil
}

func (f *fakeExchange) Subscribe(ctx context.Context, symbols []string, interval string) (<-chan exchange.StreamEvent, error) {
	atomic.AddUint64(&f.cleanupCounter, 1)
	if f.subscribeFn == nil {
		return nil, errors.New("subscribe not implemented")
	}
	return f.subscribeFn(ctx, symbols, interval)
}

func (f *fakeExchange) Close() {
	atomic.AddUint64(&f.cleanupCounter, 1)
}

func TestStreamManagerMultiplexesFeed(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	upstream := make(chan exchange.StreamEvent, 16)
	callCount := uint64(0)

	client := &fakeExchange{
		subscribeFn: func(_ context.Context, symbols []string, interval string) (<-chan exchange.StreamEvent, error) {
			atomic.AddUint64(&callCount, 1)
			if len(symbols) != 1 || symbols[0] != "BTC" || interval != "15m" {
				t.Fatalf("unexpected subscribe params symbols=%v interval=%s", symbols, interval)
			}
			return upstream, nil
		},
		marketsFn: func(context.Context) ([]exchange.Market, error) {
			return []exchange.Market{{Symbol: "BTC"}}, nil
		},
		bookFn: func(_ context.Context, _ string) (exchange.OrderBook, error) {
			return exchange.OrderBook{}, nil
		},
		tradesFn: func(context.Context, string, int) ([]exchange.Trade, error) {
			return nil, nil
		},
		candlesFn: func(context.Context, string, string, int) ([]exchange.Candle, error) {
			return nil, nil
		},
	}

	manager := stream.NewManager(client, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subA, err := manager.Subscribe(ctx, "BTC", "15m")
	if err != nil {
		t.Fatalf("subscribe A failed: %v", err)
	}
	subB, err := manager.Subscribe(ctx, "BTC", "15m")
	if err != nil {
		t.Fatalf("subscribe B failed: %v", err)
	}
	defer subA.Close()
	defer subB.Close()

	expected := exchange.StreamEvent{
		Type:       exchange.EventTrades,
		Symbol:     "BTC",
		Sequence:   0,
		ServerTime: 0,
		Data:       exchange.Trade{Symbol: "BTC", Price: "1", Size: "1"},
	}
	upstream <- expected

	for _, sub := range []<-chan exchange.StreamEvent{subA.Events, subB.Events} {
		got, ok := readEventTypeWithTimeout(t, sub, exchange.EventTrades, 2*time.Second)
		if !ok {
			t.Fatal("did not receive event")
		}
		if got.Type != expected.Type {
			t.Fatalf("unexpected event type %q", got.Type)
		}
		if got.Symbol != "BTC" {
			t.Fatalf("expected symbol BTC got %s", got.Symbol)
		}
	}

	if got := atomic.LoadUint64(&callCount); got != 1 {
		t.Fatalf("expected upstream subscribe once, got %d", got)
	}
}

func TestStreamManagerReconnectsAfterDisconnect(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	snapEvents := make(chan exchange.StreamEvent, 16)
	secondReady := make(chan chan exchange.StreamEvent, 1)
	call := uint64(0)

	client := &fakeExchange{
		subscribeFn: func(_ context.Context, symbols []string, interval string) (<-chan exchange.StreamEvent, error) {
			n := atomic.AddUint64(&call, 1)
			switch n {
			case 1:
				c := make(chan exchange.StreamEvent)
				close(c)
				return c, nil
			case 2:
				c := make(chan exchange.StreamEvent, 16)
				select {
				case secondReady <- c:
				default:
				}
				return c, nil
			default:
				return snapEvents, nil
			}
		},
		marketsFn: func(context.Context) ([]exchange.Market, error) {
			return []exchange.Market{{Symbol: "BTC"}}, nil
		},
		bookFn: func(_ context.Context, _ string) (exchange.OrderBook, error) {
			return exchange.OrderBook{}, nil
		},
		tradesFn: func(context.Context, string, int) ([]exchange.Trade, error) {
			return nil, nil
		},
		candlesFn: func(context.Context, string, string, int) ([]exchange.Candle, error) {
			return nil, nil
		},
	}

	manager := stream.NewManager(client, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := manager.Subscribe(ctx, "BTC", "15m")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer sub.Close()

	var ch chan exchange.StreamEvent
	select {
	case ch = <-secondReady:
	case <-time.After(8 * time.Second):
		t.Fatal("timeout waiting for reconnect channel")
	}
	ch <- exchange.StreamEvent{Type: exchange.EventTrades, Symbol: "BTC", Data: exchange.Trade{Symbol: "BTC", Size: "2"}}

	got, ok := readEventTypeWithTimeout(t, sub.Events, exchange.EventTrades, 5*time.Second)
	if !ok {
		t.Fatal("timeout waiting for event after reconnect")
	}
	if got.Symbol != "BTC" {
		t.Fatalf("expected BTC got %q", got.Symbol)
	}
	if got.Type != exchange.EventTrades {
		t.Fatalf("expected event type trade got %q", got.Type)
	}
}

func readEventTypeWithTimeout(t *testing.T, ch <-chan exchange.StreamEvent, expected exchange.UpstreamEventType, timeout time.Duration) (exchange.StreamEvent, bool) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return exchange.StreamEvent{}, false
			}
			if event.Type == expected {
				return event, true
			}
		case <-deadline:
			return exchange.StreamEvent{}, false
		}
	}
}
