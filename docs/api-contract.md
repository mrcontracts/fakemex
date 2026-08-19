# FakeMex API contract

All JSON decimal values are strings. Times are Unix milliseconds. The backend
is hard-locked to Hyperliquid testnet and never returns signing credentials.

## HTTP

- `GET /api/v1/health` returns backend and upstream connection status.
- `GET /api/v1/trading` returns `{ available, enabled, network }`.
- `PUT /api/v1/trading` accepts `{ enabled: boolean }`. Enabling succeeds only
  when the backend was started with signed testnet trading permitted.
- `GET /api/v1/bootstrap?symbol=BTC&interval=15m` returns `markets`, selected
  `market`, `candles`, `book`, `trades`, and the configured account snapshot.
- `GET /api/v1/markets` returns normalized perpetual metadata and contexts.
- `GET /api/v1/account` returns margin, positions, balances, and connection state.
- `GET /api/v1/orders?view=open|trigger|history` returns normalized orders.
- `GET /api/v1/fills` and `GET /api/v1/funding` return account history.
- `POST /api/v1/orders` accepts `OrderRequest`.
- `PATCH /api/v1/orders/{oid}` accepts `ModifyOrderRequest`.
- `DELETE /api/v1/orders/{oid}?symbol=BTC` cancels one order.
- `DELETE /api/v1/orders?symbol=BTC` cancels all, optionally by symbol.
- `PUT /api/v1/positions/{symbol}/leverage` accepts `LeverageRequest`.
- `POST /api/v1/positions/{symbol}/close` accepts `ClosePositionRequest`.
- `GET /api/v1/stream?symbol=BTC&interval=15m` upgrades to WebSocket.

Perpetual market metadata includes `symbol`, `base`, `quote`, and `indexName`.
The main Hyperliquid perpetual book is normalized as, for example,
`{ symbol: "BTC", base: "BTC", quote: "USD", indexName: "BTC-PERP" }`.
`quote` describes the contract's price denomination; account collateral remains
a separate balance concept.

## Write models

```ts
type OrderKind = 'limit' | 'market' | 'stopMarket' | 'stopLimit' |
  'takeProfitMarket' | 'takeProfitLimit';
type TimeInForce = 'gtc' | 'ioc' | 'alo';

interface OrderRequest {
  symbol: string;
  side: 'buy' | 'sell';
  kind: OrderKind;
  size: string;
  price?: string;
  triggerPrice?: string;
  triggerLimitPrice?: string;
  timeInForce?: TimeInForce;
  reduceOnly: boolean;
  slippagePercent?: string;
  clientOrderId?: string;
  attachedTakeProfit?: { triggerPrice: string; limitPrice?: string };
  attachedStopLoss?: { triggerPrice: string; limitPrice?: string };
}

interface ModifyOrderRequest {
  symbol: string;
  side: 'buy' | 'sell';
  size: string;
  price: string;
  timeInForce: TimeInForce;
  reduceOnly: boolean;
}

interface LeverageRequest { mode: 'cross' | 'isolated'; leverage: number }
interface ClosePositionRequest {
  percent: 25 | 50 | 75 | 100;
  kind: 'market' | 'limit';
  price?: string;
  slippagePercent?: string;
}
```

Write responses use `{ requestId, status, orderId?, clientOrderId?, filled?,
averagePrice?, message? }`. Validation and upstream failures use RFC 9457-style
problem objects `{ type, title, status, detail, code, requestId, fields? }`.
All state-changing requests require the configured local frontend `Origin`.
Writes return `trading_disabled` until the runtime toggle is armed; it resets
to off on every backend restart.

## WebSocket

Each server message uses:

```ts
interface StreamEnvelope<T = unknown> {
  type: 'snapshot' | 'markets' | 'book' | 'trades' | 'candle' | 'assetContext' |
    'account' | 'orders' | 'fills' | 'funding' | 'connection' | 'error';
  symbol?: string;
  sequence: number;
  serverTime: number;
  data: T;
}
```

The first message is a complete `snapshot`. Reconnects receive a new snapshot;
clients replace state, then apply later sequence numbers. The server sends ping
frames and marks upstream connection changes with `connection` events.

Public order book, trades, candles, and active asset context are forwarded from
Hyperliquid websocket subscriptions without client-side polling. When an
account address is configured, clearinghouse state, fills, and funding updates
are also forwarded through read-only user subscriptions. The frontend recovery
frequency setting controls websocket reconnect backoff (500–5000 ms base) and
is persisted locally; it does not throttle live messages or repeatedly poll the
heavy bootstrap endpoint.
