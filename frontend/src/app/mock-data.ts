import {
  Asset,
  BookLevel,
  Candle,
  Fill,
  Funding,
  Health,
  MarketMeta,
  MarketSnapshot,
  Order,
  Position,
  TriggerOrder,
  Trade,
} from './models';

const now = Date.now();

export const demoMarkets: MarketMeta[] = [
  {
    symbol: 'BTC',
    base: 'BTC',
    quote: 'USD',
    indexName: 'BTC-PERP',
    markPx: '62453.21',
    funding: '0.00021',
    leverage: { maxLeverage: 100, currentMode: 'cross', currentLeverage: 20 },
    baseDecimals: 4,
    quoteDecimals: 2,
  },
  {
    symbol: 'ETH',
    base: 'ETH',
    quote: 'USD',
    indexName: 'ETH-PERP',
    markPx: '3251.8',
    funding: '-0.00011',
    leverage: { maxLeverage: 50, currentMode: 'cross', currentLeverage: 10 },
    baseDecimals: 3,
    quoteDecimals: 2,
  },
  {
    symbol: 'SOL',
    base: 'SOL',
    quote: 'USD',
    indexName: 'SOL-PERP',
    markPx: '142.35',
    funding: '0.00008',
    leverage: { maxLeverage: 30, currentMode: 'cross', currentLeverage: 8 },
    baseDecimals: 2,
    quoteDecimals: 2,
  },
];

const candleBase = 8_500_000_000;
export const demoCandles = Array.from({ length: 180 }, (_, index) => {
  const drift = Math.sin(index / 9) * 85;
  const open = 62000 + drift + index * 0.5;
  const close = open + Math.sin(index / 13) * 120;
  const hi = Math.max(open, close) + 55;
  const lo = Math.min(open, close) - 48;
  const vol = ((Math.random() * 120) + 20).toFixed(3);

  return {
    t: candleBase + index * 60_000 * 15,
    o: open.toFixed(2),
    h: hi.toFixed(2),
    l: lo.toFixed(2),
    c: close.toFixed(2),
    v: vol,
  } as Candle;
});

export const demoBook: BookLevel[] = [
  { side: 'sell', price: '62497.20', size: '3.1', total: '193,700' },
  { side: 'sell', price: '62491.80', size: '5.2', total: '325,200' },
  { side: 'sell', price: '62488.10', size: '12.4', total: '774,900' },
  { side: 'sell', price: '62483.70', size: '8.0', total: '499,800' },
  { side: 'buy', price: '62476.30', size: '11.3', total: '706,000' },
  { side: 'buy', price: '62470.20', size: '7.1', total: '444,000' },
  { side: 'buy', price: '62464.40', size: '4.6', total: '287,000' },
  { side: 'buy', price: '62459.00', size: '6.8', total: '424,000' },
];

export const demoTrades: Trade[] = [
  { px: '62480.20', size: '0.120', side: 'buy', ts: now - 1 * 60_000 },
  { px: '62476.15', size: '0.350', side: 'sell', ts: now - 2 * 60_000 },
  { px: '62490.00', size: '0.040', side: 'buy', ts: now - 3 * 60_000 },
  { px: '62510.45', size: '0.500', side: 'sell', ts: now - 4 * 60_000 },
  { px: '62498.05', size: '0.080', side: 'buy', ts: now - 5 * 60_000 },
  { px: '62469.31', size: '0.150', side: 'sell', ts: now - 6 * 60_000 },
];

export const demoPositions: Position[] = [
  {
    symbol: 'BTC',
    side: 'buy',
    size: '0.35',
    entryPrice: '61234.12',
    markPrice: '62480.10',
    unrealizedPnl: '419.42',
    realizedPnl: '0.00',
    leverage: 20,
    margin: '1095.00',
    openPnlPercent: '2.01',
    liquidation: '49800.00',
  },
];

export const demoOrders: Order[] = [
  {
    oid: 'd1f3',
    symbol: 'BTC',
    side: 'buy',
    kind: 'limit',
    size: '0.15',
    price: '62000.00',
    timeInForce: 'gtc',
    reduceOnly: false,
    status: 'open',
    createdAt: now - 6_000_000,
  },
  {
    oid: 'e7b2',
    symbol: 'BTC',
    side: 'sell',
    kind: 'stopMarket',
    size: '0.10',
    triggerPrice: '61800',
    timeInForce: 'gtc',
    reduceOnly: true,
    status: 'open',
    createdAt: now - 5_200_000,
    triggerLimitPrice: undefined,
  },
];

export const demoTriggerOrders: TriggerOrder[] = [
  {
    oid: 't001',
    symbol: 'BTC',
    side: 'buy',
    kind: 'takeProfitMarket',
    triggerPrice: '65000',
    price: undefined,
    status: 'armed',
  },
  {
    oid: 't002',
    symbol: 'BTC',
    side: 'sell',
    kind: 'stopMarket',
    triggerPrice: '56000',
    price: undefined,
    status: 'triggered',
  },
];

export const demoFills: Fill[] = [
  {
    fillId: 'f001',
    oid: 'h100',
    symbol: 'BTC',
    side: 'buy',
    px: '62340',
    size: '0.02',
    fee: '0.23',
    ts: now - 80_000,
  },
  {
    fillId: 'f002',
    oid: 'h101',
    symbol: 'ETH',
    side: 'sell',
    px: '3241',
    size: '1.20',
    fee: '0.09',
    ts: now - 120_000,
  },
];

export const demoFunding: Funding[] = [
  { symbol: 'BTC', rate: '0.00021', payment: '1.31', ts: now - 900_000 },
  { symbol: 'ETH', rate: '-0.00011', payment: '-0.55', ts: now - 900_000 },
];

export const demoAssets: Asset[] = [
  {
    coin: 'USDC',
    wallet: '25000.00',
    crossMarginUsed: '5200.00',
    available: '19800.00',
  },
  {
    coin: 'BTC',
    wallet: '0.52',
    crossMarginUsed: '0.00',
    available: '0.52',
  },
  {
    coin: 'ETH',
    wallet: '3.15',
    crossMarginUsed: '0.00',
    available: '3.15',
  },
];

export const demoHealth: Health = {
  backend: 'offline',
  upstream: 'offline',
  connected: false,
  accountReady: false,
  tradingAvailable: false,
  tradingEnabled: false,
  network: 'testnet',
  timestamp: 0,
};

export const demoSnapshot: MarketSnapshot = {
  markets: demoMarkets,
  market: demoMarkets[0],
  candles: demoCandles,
  book: demoBook,
  trades: demoTrades,
  account: {
    margin: {
      crossBalance: '25000.00',
      availableBalance: '19700.00',
      totalMarginUsed: '5200.00',
    },
    positions: demoPositions,
    balances: demoAssets,
    leverageMode: 'cross',
  },
  orders: {
    open: demoOrders,
    trigger: demoTriggerOrders,
    history: [],
  },
  fills: demoFills,
  funding: demoFunding,
  assets: demoAssets,
};
