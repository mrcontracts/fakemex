package exchange

import "time"

type Market struct {
	AssetIndex      int    `json:"assetIndex"`
	Symbol          string `json:"symbol"`
	Base            string `json:"base"`
	Quote           string `json:"quote"`
	Active          bool   `json:"active"`
	TickSize        string `json:"tickSize"`
	SizeTick        string `json:"sizeTick"`
	MaxLeverage     int    `json:"maxLeverage"`
	IndexPrice      string `json:"indexPrice"`
	MarkPx          string `json:"markPx"`
	MarkPrice       string `json:"markPrice"`
	Contract        string `json:"contract"`
	TimeInForce     string `json:"timeInForce"`
	MaxOrderSize    string `json:"maxOrderSize"`
	PricePrecision  int    `json:"pricePrecision"`
	SizePrecision   int    `json:"sizePrecision"`
	LastUpdateTime  int64  `json:"lastUpdateTime"`
	LastFundingRate string `json:"lastFundingRate"`

	IndexName     string         `json:"indexName"`
	Funding       string         `json:"funding"`
	LeverageInfo  MarketLeverage `json:"leverage"`
	BaseDecimals  int            `json:"baseDecimals"`
	QuoteDecimals int            `json:"quoteDecimals"`
}

type MarketLeverage struct {
	MaxLeverage     int    `json:"maxLeverage"`
	CurrentMode     string `json:"currentMode"`
	CurrentLeverage int    `json:"currentLeverage"`
}

type AssetContext struct {
	Symbol      string `json:"symbol"`
	Leverage    int    `json:"leverage"`
	Type        string `json:"type"`
	MaxLeverage int    `json:"maxLeverage"`
	IsCross     bool   `json:"isCross"`
	MarkPx      string `json:"markPx"`
	LastFunding string `json:"funding"`
	Mark        string `json:"markPrice"`
}

type OrderBookLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
	Side  string `json:"side"`
	Total string `json:"total"`
}

type OrderBook struct {
	Symbol    string           `json:"symbol"`
	Asks      []OrderBookLevel `json:"asks"`
	Bids      []OrderBookLevel `json:"bids"`
	UpdatedAt int64            `json:"updatedAt"`
}

type Trade struct {
	Symbol   string `json:"symbol,omitempty"`
	Side     string `json:"side"`
	Size     string `json:"size"`
	Px       string `json:"px"`
	Price    string `json:"-"`
	Ts       int64  `json:"ts"`
	Sequence int64  `json:"sequence,omitempty"`
}

type Candle struct {
	Symbol    string `json:"symbol,omitempty"`
	Timestamp int64  `json:"t"`
	Open      string `json:"o"`
	High      string `json:"h"`
	Low       string `json:"l"`
	Close     string `json:"c"`
	Volume    string `json:"v"`
}

type Position struct {
	Symbol         string `json:"symbol"`
	Side           string `json:"side"`
	Size           string `json:"size"`
	Leverage       int    `json:"leverage"`
	EntryPrice     string `json:"entryPrice"`
	MarkPrice      string `json:"markPrice"`
	UnrealizedPnl  string `json:"unrealizedPnl"`
	RealizedPnl    string `json:"realizedPnl"`
	OpenPnlPercent string `json:"openPnlPercent"`
	Liquidation    string `json:"liquidation"`
	MarginType     string `json:"marginType"`
	Margin         string `json:"margin"`
}

type Margin struct {
	CrossBalance     string `json:"crossBalance"`
	AvailableBalance string `json:"availableBalance"`
	TotalMarginUsed  string `json:"totalMarginUsed"`
}

type Account struct {
	Address      string     `json:"address"`
	Margin       Margin     `json:"margin"`
	LeverageMode string     `json:"leverageMode"`
	Positions    []Position `json:"positions"`
	Balances     []Asset    `json:"balances"`
	Assets       []Asset    `json:"assets"`

	CrossBalance    string    `json:"crossBalance,omitempty"`
	Available       string    `json:"available,omitempty"`
	TotalMarginUsed string    `json:"totalMarginUsed,omitempty"`
	LastUpdate      int64     `json:"lastUpdate,omitempty"`
	UpdatedAt       time.Time `json:"-"`
}

type Asset struct {
	Coin            string `json:"coin"`
	Wallet          string `json:"wallet"`
	CrossMarginUsed string `json:"crossMarginUsed"`
	Available       string `json:"available"`
}

type OrdersSnapshot struct {
	Open    []Order `json:"open"`
	Trigger []Order `json:"trigger"`
	History []Order `json:"history"`
}

type Order struct {
	ID                string `json:"id,omitempty"`
	Oid               string `json:"oid"`
	ClientOrderID     string `json:"clientOrderId,omitempty"`
	Symbol            string `json:"symbol"`
	Side              string `json:"side"`
	Kind              string `json:"kind"`
	Size              string `json:"size"`
	Price             string `json:"price,omitempty"`
	TriggerPrice      string `json:"triggerPrice,omitempty"`
	TriggerLimitPrice string `json:"triggerLimitPrice,omitempty"`
	TimeInForce       string `json:"timeInForce"`
	ReduceOnly        bool   `json:"reduceOnly"`
	Status            string `json:"status"`
	CreatedAt         int64  `json:"createdAt"`
	AveragePrice      string `json:"avgPrice,omitempty"`
	UpdatedAt         int64  `json:"updatedAt,omitempty"`
	Filled            string `json:"filled,omitempty"`
	FilledSize        string `json:"filledSize,omitempty"`
}

type Fill struct {
	FillID  string `json:"fillId"`
	OrderID string `json:"oid"`
	Symbol  string `json:"symbol"`
	Side    string `json:"side"`
	Size    string `json:"size"`
	Px      string `json:"px"`
	Fee     string `json:"fee"`
	Ts      int64  `json:"ts"`
}

type FundingEvent struct {
	Symbol  string `json:"symbol"`
	Rate    string `json:"rate"`
	Payment string `json:"payment"`
	Ts      int64  `json:"ts"`
}

type Health struct {
	Backend            string `json:"backend"`
	Upstream           string `json:"upstream"`
	Connected          bool   `json:"connected"`
	UpstreamLatencyMs  int64  `json:"upstreamLatencyMs,omitempty"`
	LastSuccessfulPing int64  `json:"lastSuccessfulPing,omitempty"`
	AccountReady       bool   `json:"accountReady"`
	TradingAvailable   bool   `json:"tradingAvailable"`
	TradingEnabled     bool   `json:"tradingEnabled"`
	Network            string `json:"network"`
	Timestamp          int64  `json:"timestamp"`
}

type WriteRequest interface{}

type OrderRequest struct {
	Symbol             string         `json:"symbol"`
	Side               string         `json:"side"`
	Kind               string         `json:"kind"`
	Size               string         `json:"size"`
	Price              string         `json:"price"`
	TriggerPrice       string         `json:"triggerPrice"`
	TriggerLimitPrice  string         `json:"triggerLimitPrice"`
	TimeInForce        string         `json:"timeInForce"`
	ReduceOnly         bool           `json:"reduceOnly"`
	SlippagePercent    string         `json:"slippagePercent"`
	ClientOrderID      string         `json:"clientOrderId"`
	AttachedTakeProfit *AttachedOrder `json:"attachedTakeProfit"`
	AttachedStopLoss   *AttachedOrder `json:"attachedStopLoss"`
}

type AttachedOrder struct {
	TriggerPrice string `json:"triggerPrice"`
	LimitPrice   string `json:"limitPrice"`
}

type ModifyOrderRequest struct {
	Symbol      string `json:"symbol"`
	Side        string `json:"side"`
	Size        string `json:"size"`
	Price       string `json:"price"`
	TimeInForce string `json:"timeInForce"`
	ReduceOnly  bool   `json:"reduceOnly"`
}

type LeverageRequest struct {
	Mode     string `json:"mode"`
	Leverage int    `json:"leverage"`
}

type ClosePositionRequest struct {
	Percent         int    `json:"percent"`
	Kind            string `json:"kind"`
	Price           string `json:"price"`
	SlippagePercent string `json:"slippagePercent"`
}

type WriteResponse struct {
	RequestID     string `json:"requestId"`
	Status        string `json:"status"`
	OrderID       string `json:"orderId,omitempty"`
	ClientOrderID string `json:"clientOrderId,omitempty"`
	Filled        string `json:"filled,omitempty"`
	AveragePrice  string `json:"averagePrice,omitempty"`
	Message       string `json:"message,omitempty"`
}

type Problem struct {
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Status    int               `json:"status"`
	Detail    string            `json:"detail"`
	Code      string            `json:"code"`
	RequestID string            `json:"requestId"`
	Fields    map[string]string `json:"fields,omitempty"`
}

type Bootstrap struct {
	Markets []Market         `json:"markets"`
	Market  *Market          `json:"market"`
	Candles []Candle         `json:"candles"`
	Book    []OrderBookLevel `json:"book"`
	Trades  []Trade          `json:"trades"`
	Account *Account         `json:"account"`
	Assets  []Asset          `json:"assets"`
	Orders  *OrdersSnapshot  `json:"orders"`
	Fills   []Fill           `json:"fills"`
	Funding []FundingEvent   `json:"funding"`
}
