import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';
import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { PLATFORM_ID } from '@angular/core';
import { BrowserDynamicTestingModule, platformBrowserDynamicTesting } from '@angular/platform-browser-dynamic/testing';
import { FakeMexDataService } from './fakemex-data.service';
import type { Asset, MarketMeta, Order, Position, StreamEnvelope, TriggerOrder } from '../models';
import { demoSnapshot } from '../mock-data';

function ensureLocalStorage() {
  if (typeof globalThis.localStorage === 'undefined') {
    const store = new Map<string, string>();
    globalThis.localStorage = {
      getItem: (key) => (store.has(key) ? store.get(key) ?? null : null),
      setItem: (key, value) => {
        store.set(key, String(value));
      },
      removeItem: (key) => {
        store.delete(key);
      },
      clear: () => {
        store.clear();
      },
      key: (index) => Array.from(store.keys())[index] ?? null,
      length: 0,
    } as Storage;
  }
}

type ListenerMap = Map<string, Array<(event?: unknown) => void>>;

class MockWebSocket {
  static instances: MockWebSocket[] = [];
  private readonly handlers: ListenerMap = new Map();
  readyState = 1;

  constructor(public url: string) {
    MockWebSocket.instances.push(this);
  }

  addEventListener(type: string, handler: (event?: unknown) => void): void {
    const next = this.handlers.get(type) ?? [];
    next.push(handler);
    this.handlers.set(type, next);
  }

  close(): void {
    this.readyState = 3;
    this.emit('close', {});
  }

  emit(type: string, event: unknown): void {
    const listeners = this.handlers.get(type) ?? [];
    listeners.forEach((listener) => listener(event));
  }
}

describe('FakeMexDataService', () => {
  try {
    TestBed.initTestEnvironment(BrowserDynamicTestingModule, platformBrowserDynamicTesting());
  } catch {
    // TestBed was likely initialized already, which is fine for this file.
  }

  beforeEach(() => {
    TestBed.resetTestingModule();
    ensureLocalStorage();
    localStorage.clear();
    vi.useFakeTimers();
    MockWebSocket.instances = [];
    (globalThis as { WebSocket: typeof MockWebSocket }).WebSocket = MockWebSocket;
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  function createService() {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting(), { provide: PLATFORM_ID, useValue: 'browser' }, FakeMexDataService],
    });
    const service = TestBed.inject(FakeMexDataService);
    const controller = TestBed.inject(HttpTestingController);
    return { service, controller };
  }

  const bootstrapRequest = (controller: HttpTestingController) => {
    controller.expectOne((request) => request.url.startsWith('/api/v1/bootstrap')).flush(demoSnapshot);
    controller.expectOne('/api/v1/trading').flush({ available: true, enabled: false, network: 'testnet' });
  };

  const emitStreamEvent = (service: FakeMexDataService, event: StreamEnvelope<unknown>) => {
    (service as unknown as { applyStream: (payload: StreamEnvelope<unknown>) => void }).applyStream(event);
  };

  it('loads persisted refresh interval preference and clamps unsafe values', () => {
    localStorage.setItem('fakemex-refresh-interval-ms', '250');
    const { service, controller } = createService();
    bootstrapRequest(controller);

    expect(service.refreshIntervalMs()).toBe(500);
    service.destroy();
    controller.verify();
  });

  it('persists a clamped refresh interval preference', () => {
    const { service, controller } = createService();
    bootstrapRequest(controller);

    service.setRefreshInterval(6_250);
    expect(service.refreshIntervalMs()).toBe(5_000);
    expect(localStorage.getItem('fakemex-refresh-interval-ms')).toBe('5000');
    service.destroy();
    controller.verify();
  });

  it('re-schedules the next recovery attempt when interval changes while reconnecting', () => {
    const { service, controller } = createService();
    bootstrapRequest(controller);

    const firstSocket = MockWebSocket.instances[0];
    firstSocket.close();

    vi.advanceTimersByTime(1);
    expect(service.connection().phase).toBe('reconnecting');

    service.setRefreshInterval(500);

    vi.advanceTimersByTime(499);
    expect(MockWebSocket.instances).toHaveLength(1);

    vi.advanceTimersByTime(1);
    expect(MockWebSocket.instances).toHaveLength(2);

    vi.advanceTimersByTime(8_000);
    expect(MockWebSocket.instances).toHaveLength(2);

    const extraRequests = controller.match((request) => request.url.startsWith('/api/v1/bootstrap'));
    expect(extraRequests).toHaveLength(0);

    service.destroy();
    controller.verify();
  });

  it('does not suppress later reconnects when an intentionally closed old socket finishes close late', () => {
    const { service, controller } = createService();
    bootstrapRequest(controller);

    const initialSocket = MockWebSocket.instances[0];
    const internal = service as unknown as {
      socket: MockWebSocket | null;
      openStream: (symbol: string, intervalValue: string) => void;
      intentionalCloseSockets: WeakSet<WebSocket>;
    };

    internal.socket = null;
    internal.openStream('ETH', '15m');
    const replacementSocket = MockWebSocket.instances[1];

    internal.intentionalCloseSockets.add(initialSocket);
    initialSocket.emit('close', {});

    replacementSocket.close();

    vi.advanceTimersByTime(999);
    expect(MockWebSocket.instances).toHaveLength(2);

    vi.advanceTimersByTime(1_000);
    expect(MockWebSocket.instances).toHaveLength(3);

    service.destroy();
    controller.verify();
  });

  it('merges assetContext partial updates without dropping static market fields', () => {
    const { service, controller } = createService();
    bootstrapRequest(controller);

    const baseline = service.market() as MarketMeta & { active: boolean };
    service.market.set({ ...baseline, active: true });
    service.markets.update((markets) =>
      markets.map((market) => (market.symbol === 'BTC' ? { ...market, active: true } : market)),
    );
    expect(baseline.baseDecimals).toBe(4);
    expect(baseline.leverage.maxLeverage).toBe(100);

    emitStreamEvent(service, {
      type: 'assetContext',
      symbol: 'BTC',
      sequence: 2,
      serverTime: Date.now(),
      data: {
        symbol: 'BTC',
        markPx: '99999.00',
        funding: '0.00099',
        baseDecimals: 0,
        quoteDecimals: 0,
        base: '',
        quote: '',
        indexName: '',
        active: false,
        leverage: {
          currentLeverage: 12,
          maxLeverage: 1,
          currentMode: '' as '',
        },
      },
    });

    expect(service.market()?.markPx).toBe('99999.00');
    expect(service.market()?.funding).toBe('0.00099');
    expect(service.market()?.baseDecimals).toBe(4);
    expect(service.market()?.quoteDecimals).toBe(2);
    expect(service.market()?.leverage.currentLeverage).toBe(12);
    expect(service.market()?.leverage.currentMode).toBe('cross');
    expect(service.market()?.leverage.maxLeverage).toBe(100);
    expect(service.market()?.active).toBe(true);
    expect(service.market()?.base).toBe('BTC');
    expect(service.market()?.quote).toBe('USD');
    expect(service.market()?.indexName).toBe('BTC-PERP');

    const mergedMarket = service.markets().find((item) => item.symbol === 'BTC');
    expect(mergedMarket?.markPx).toBe('99999.00');
    expect(mergedMarket?.funding).toBe('0.00099');
    expect(mergedMarket?.baseDecimals).toBe(4);
    expect(mergedMarket?.quoteDecimals).toBe(2);
    expect(mergedMarket?.leverage.maxLeverage).toBe(100);
    expect(mergedMarket?.leverage.currentLeverage).toBe(12);
    expect(mergedMarket?.active).toBe(true);
  });

  it('updates complete account events while supporting legacy margin-only payloads', () => {
    const { service, controller } = createService();
    bootstrapRequest(controller);

    const updatedPositions: Position[] = [
      {
        symbol: 'ETH',
        side: 'sell',
        size: '0.20',
        entryPrice: '3200.00',
        markPrice: '3240.00',
        unrealizedPnl: '15.00',
        realizedPnl: '0.00',
        leverage: 5,
        margin: '160.00',
        liquidation: '2900.00',
      },
    ];
    const updatedAssets: Asset[] = [
      {
        coin: 'USDC',
        wallet: '12345.00',
        crossMarginUsed: '500.00',
        available: '8000.00',
      },
    ];

    emitStreamEvent(service, {
      type: 'account',
      symbol: 'BTC',
      sequence: 2,
      serverTime: Date.now(),
      data: {
        margin: {
          crossBalance: '30000.00',
          availableBalance: '21000.00',
          totalMarginUsed: '9000.00',
        },
        positions: updatedPositions,
        balances: updatedAssets,
        leverageMode: 'isolated',
      },
    });

    expect(service.margin()).toEqual({
      crossBalance: '30000.00',
      availableBalance: '21000.00',
      totalMarginUsed: '9000.00',
    });
    expect(service.positions()).toEqual(updatedPositions);
    expect(service.assets()).toEqual(updatedAssets);
    expect(service.selectedMode()).toBe('isolated');

    emitStreamEvent(service, {
      type: 'account',
      symbol: 'BTC',
      sequence: 3,
      serverTime: Date.now(),
      data: {
        margin: {
          crossBalance: '31000.00',
          availableBalance: '21500.00',
          totalMarginUsed: '9500.00',
        },
      },
    });

    expect(service.margin().crossBalance).toBe('31000.00');
    expect(service.positions()).toEqual(updatedPositions);
    expect(service.assets()).toEqual(updatedAssets);
    expect(service.selectedMode()).toBe('isolated');
  });

  it('replaces open and trigger orders when both are present in stream events', () => {
    const { service, controller } = createService();
    bootstrapRequest(controller);

    const openReplacement: Order[] = [
      {
        oid: 'stream-open',
        symbol: 'BTC',
        side: 'sell',
        kind: 'market',
        size: '0.50',
        timeInForce: 'gtc',
        reduceOnly: false,
        status: 'open',
        createdAt: Date.now(),
      },
    ];
    const triggerReplacement: TriggerOrder[] = [
      {
        oid: 'stream-trigger',
        symbol: 'BTC',
        side: 'sell',
        kind: 'takeProfitMarket',
        triggerPrice: '61000',
        status: 'armed',
      },
    ];

    emitStreamEvent(service, {
      type: 'orders',
      symbol: 'BTC',
      sequence: 2,
      serverTime: Date.now(),
      data: {
        open: openReplacement,
        trigger: triggerReplacement,
      },
    });

    expect(service.orders()).toEqual(openReplacement);
    expect(service.triggerOrders()).toEqual(triggerReplacement);
  });

  it('updates only open orders when trigger is omitted for backward compatibility', () => {
    const { service, controller } = createService();
    bootstrapRequest(controller);

    const initialTriggerOrders = service.triggerOrders();
    const legacyOpen: Order[] = [
      {
        oid: 'legacy-open',
        symbol: 'BTC',
        side: 'buy',
        kind: 'limit',
        size: '0.25',
        price: '61000.00',
        timeInForce: 'gtc',
        reduceOnly: false,
        status: 'open',
        createdAt: Date.now(),
      },
    ];

    emitStreamEvent(service, {
      type: 'orders',
      symbol: 'BTC',
      sequence: 3,
      serverTime: Date.now(),
      data: { open: legacyOpen },
    });

    expect(service.orders()).toEqual(legacyOpen);
    expect(service.triggerOrders()).toEqual(initialTriggerOrders);
  });

  it('starts trading off and updates it only after the backend accepts the toggle', () => {
    const { service, controller } = createService();
    bootstrapRequest(controller);

    expect(service.tradingStatus()).toEqual({ available: true, enabled: false, network: 'testnet' });
    let observed = false;
    service.setTradingEnabled(true).subscribe((status) => {
      observed = status.enabled;
    });
    expect(service.tradingStatus().enabled).toBe(false);
    controller.expectOne({ method: 'PUT', url: '/api/v1/trading' }).flush({
      available: true,
      enabled: true,
      network: 'testnet',
    });
    expect(observed).toBe(true);
    expect(service.tradingStatus().enabled).toBe(true);

    service.destroy();
    controller.verify();
  });

  it('never simulates a successful order in demo mode', () => {
    const { service, controller } = createService();
    controller.expectOne((request) => request.url.startsWith('/api/v1/bootstrap')).flush(
      { detail: 'offline' },
      { status: 503, statusText: 'Unavailable' },
    );
    controller.expectOne('/api/v1/trading').flush({ available: false, enabled: false, network: 'testnet' });

    let resultStatus = '';
    service.submitOrder({
      symbol: 'BTC',
      side: 'buy',
      kind: 'market',
      size: '0.01',
      reduceOnly: false,
    }).subscribe((result) => {
      resultStatus = result.status;
    });
    expect(resultStatus).toBe('error');
    expect(controller.match('/api/v1/orders')).toHaveLength(0);

    service.destroy();
    controller.verify();
  });
});
