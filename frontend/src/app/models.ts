export type OrderSide = 'buy' | 'sell';
export type OrderKind =
  | 'limit'
  | 'market'
  | 'stopMarket'
  | 'stopLimit'
  | 'takeProfitMarket'
  | 'takeProfitLimit';
export type TimeInForce = 'gtc' | 'ioc' | 'alo';

export interface MarketMeta {
  symbol: string;
  base: string;
  quote: string;
  indexName: string;
  markPx: string;
  funding: string;
  leverage: {
    maxLeverage: number;
    currentMode: 'cross' | 'isolated';
    currentLeverage: number;
  };
  baseDecimals: number;
  quoteDecimals: number;
  active?: boolean;
}

export interface Candle {
  t: number;
  o: string;
  h: string;
  l: string;
  c: string;
  v: string;
}

export interface BookLevel {
  price: string;
  size: string;
  side: 'buy' | 'sell';
  total: string;
}

export interface Trade {
  px: string;
  size: string;
  side: 'buy' | 'sell';
  ts: number;
}

export interface Position {
  symbol: string;
  side: OrderSide;
  size: string;
  entryPrice: string;
  markPrice: string;
  unrealizedPnl: string;
  realizedPnl: string;
  leverage: number;
  margin: string;
  liquidation?: string;
  openPnlPercent?: string;
}

export interface Order {
  oid: string;
  symbol: string;
  side: OrderSide;
  kind: OrderKind;
  size: string;
  price?: string;
  triggerPrice?: string;
  triggerLimitPrice?: string;
  timeInForce: TimeInForce;
  reduceOnly: boolean;
  status: 'open' | 'filled' | 'cancelled' | 'triggered' | 'closed';
  createdAt: number;
  avgPrice?: string;
}

export interface TriggerOrder {
  oid: string;
  symbol: string;
  side: OrderSide;
  kind: 'stopMarket' | 'takeProfitMarket' | 'stopLimit' | 'takeProfitLimit';
  price?: string;
  triggerPrice: string;
  qty?: string;
  status: 'armed' | 'triggered' | 'cancelled';
}

export interface Fill {
  fillId: string;
  oid: string;
  symbol: string;
  side: OrderSide;
  px: string;
  size: string;
  fee: string;
  ts: number;
}

export interface Funding {
  symbol: string;
  rate: string;
  payment: string;
  ts: number;
}

export interface Asset {
  coin: string;
  wallet: string;
  crossMarginUsed: string;
  available: string;
}

export interface StreamEnvelope<T = unknown> {
  type:
    | 'snapshot'
    | 'markets'
    | 'book'
    | 'trades'
    | 'candle'
    | 'assetContext'
    | 'account'
    | 'orders'
    | 'fills'
    | 'funding'
    | 'connection'
    | 'error';
  symbol?: string;
  sequence: number;
  serverTime: number;
  data: T;
}

export interface MarketSnapshot {
  markets: MarketMeta[];
  market: MarketMeta;
  candles: Candle[];
  book: BookLevel[];
  trades: Trade[];
  account: {
    margin: {
      crossBalance: string;
      availableBalance: string;
      totalMarginUsed: string;
    };
    positions: Position[];
    balances: Asset[];
    leverageMode: 'cross' | 'isolated';
  };
  orders: {
    open: Order[];
    trigger: TriggerOrder[];
    history: Order[];
  };
  fills: Fill[];
  funding: Funding[];
  assets: Asset[];
}

export interface Health {
  backend: string;
  upstream: string;
  connected: boolean;
  accountReady: boolean;
  tradingAvailable: boolean;
  tradingEnabled: boolean;
  network: string;
  timestamp: number;
}

export interface TradingStatus {
  available: boolean;
  enabled: boolean;
  network: string;
}

export interface ConnectionState {
  phase: 'offline' | 'reconnecting' | 'online';
  detail: string;
}

export interface ApiError {
  type: string;
  title: string;
  status: number;
  detail: string;
  code?: string;
  requestId?: string;
  fields?: Record<string, string>;
}

export interface OrderRequest {
  symbol: string;
  side: OrderSide;
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

export interface ModifyOrderRequest {
  symbol: string;
  side: OrderSide;
  size: string;
  price: string;
  timeInForce: TimeInForce;
  reduceOnly: boolean;
}

export interface LeverageRequest {
  mode: 'cross' | 'isolated';
  leverage: number;
}

export interface ClosePositionRequest {
  percent: 25 | 50 | 75 | 100;
  kind: 'market' | 'limit';
  price?: string;
  slippagePercent?: string;
}

export interface OrderWriteResult {
  requestId: string;
  status: 'ok' | 'error';
  orderId?: string;
  clientOrderId?: string;
  filled?: string;
  averagePrice?: string;
  message?: string;
}
