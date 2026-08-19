package exchange

import "context"

type UpstreamEventType string

const (
	EventBook         UpstreamEventType = "book"
	EventTrades       UpstreamEventType = "trades"
	EventCandle       UpstreamEventType = "candle"
	EventMarkets      UpstreamEventType = "markets"
	EventAccount      UpstreamEventType = "account"
	EventAssetContext UpstreamEventType = "assetContext"
	EventOrders       UpstreamEventType = "orders"
	EventFills        UpstreamEventType = "fills"
	EventFunding      UpstreamEventType = "funding"
	EventError        UpstreamEventType = "error"
	EventConnection   UpstreamEventType = "connection"
)

type StreamEvent struct {
	Type       UpstreamEventType `json:"type"`
	Symbol     string            `json:"symbol,omitempty"`
	Sequence   uint64            `json:"sequence"`
	ServerTime int64             `json:"serverTime"`
	Data       any               `json:"data"`
}

type ExchangeClient interface {
	Health(ctx context.Context) (Health, error)
	Markets(ctx context.Context) ([]Market, error)
	AssetContexts(ctx context.Context) ([]AssetContext, error)
	Book(ctx context.Context, symbol string) (OrderBook, error)
	Trades(ctx context.Context, symbol string, limit int) ([]Trade, error)
	Candles(ctx context.Context, symbol, interval string, limit int) ([]Candle, error)
	AccountSnapshot(ctx context.Context, address string) (Account, error)
	Orders(ctx context.Context, address, view string) ([]Order, error)
	Fills(ctx context.Context, address string) ([]Fill, error)
	Funding(ctx context.Context, address string) ([]FundingEvent, error)
	PlaceOrder(ctx context.Context, request OrderRequest) (WriteResponse, error)
	ModifyOrder(ctx context.Context, oid string, request ModifyOrderRequest) (WriteResponse, error)
	CancelOrder(ctx context.Context, address, symbol, oid string) (WriteResponse, error)
	CancelAllOrders(ctx context.Context, address, symbol string) (WriteResponse, error)
	SetLeverage(ctx context.Context, address, symbol string, request LeverageRequest) (WriteResponse, error)
	ClosePosition(ctx context.Context, address, symbol string, request ClosePositionRequest) (WriteResponse, error)
	Subscribe(ctx context.Context, symbols []string, interval string) (<-chan StreamEvent, error)
	Close()
}
