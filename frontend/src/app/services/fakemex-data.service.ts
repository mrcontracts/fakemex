import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Injectable, PLATFORM_ID, computed, inject, signal } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';
import { catchError, Observable, interval, of, Subject, switchMap, tap } from 'rxjs';
import { takeUntil } from 'rxjs/operators';
import {
  Asset,
  BookLevel,
  Candle,
  ClosePositionRequest,
  ConnectionState,
  Fill,
  Funding,
  Health,
  LeverageRequest,
  MarketMeta,
  MarketSnapshot,
  ModifyOrderRequest,
  NetworkName,
  NetworkStatus,
  Order,
  OrderWriteResult,
  OrderRequest,
  Position,
  TriggerOrder,
  StreamEnvelope,
  Trade,
  TradingStatus,
} from '../models';
import { demoHealth, demoSnapshot } from '../mock-data';

const DEFAULT_REFRESH_INTERVAL_MS = 1000;
const MIN_REFRESH_INTERVAL_MS = 500;
const MAX_REFRESH_INTERVAL_MS = 5000;
const REFRESH_INTERVAL_STEP_MS = 250;
const MAX_RECONNECT_DELAY_MS = 8000;
const LOCAL_STORAGE_REFRESH_INTERVAL_KEY = 'fakemex-refresh-interval-ms';
const LOCAL_STORAGE_REFRESH_INTERVAL_FALLBACK = `${DEFAULT_REFRESH_INTERVAL_MS}`;

@Injectable({ providedIn: 'root' })
export class FakeMexDataService {
  private readonly http = inject(HttpClient);
  private readonly platformId = inject(PLATFORM_ID);

  readonly markets = signal<MarketMeta[]>([]);
  readonly selectedSymbol = signal('BTC');
  readonly interval = signal('15m');
  readonly market = signal<MarketMeta | null>(null);
  readonly candles = signal<Candle[]>([]);
  readonly book = signal<BookLevel[]>([]);
  readonly trades = signal<Trade[]>([]);
  readonly positions = signal<Position[]>([]);
  readonly orders = signal<Order[]>([]);
  readonly triggerOrders = signal<TriggerOrder[]>([]);
  readonly fills = signal<Fill[]>([]);
  readonly funding = signal<Funding[]>([]);
  readonly assets = signal<Asset[]>([]);
  readonly margin = signal({
    crossBalance: '0',
    availableBalance: '0',
    totalMarginUsed: '0',
  });
  readonly leverageMode = signal<'cross' | 'isolated'>('cross');
  readonly connection = signal<ConnectionState>({ phase: 'offline', detail: 'Disconnected' });
  readonly loading = signal(true);
  readonly error = signal<string | null>(null);
  readonly isDemo = signal(false);
  readonly health = signal<Health>(demoHealth);
  readonly tradingStatus = signal<TradingStatus>({
    available: false,
    enabled: false,
    network: 'testnet',
  });
  readonly networkStatus = signal<NetworkStatus>({
    network: 'testnet',
    availableNetworks: ['testnet', 'mainnet'],
    tradingAvailable: false,
    tradingEnabled: false,
  });
  readonly refreshIntervalMs = signal(DEFAULT_REFRESH_INTERVAL_MS);
  readonly hasConnection = computed(() => this.connection().phase !== 'offline');

  readonly selectedMarket = computed(
    () => this.markets().find((item) => item.symbol === this.selectedSymbol()) ?? this.market(),
  );
  readonly selectedLeverage = computed(() => this.market()?.leverage.currentLeverage ?? 1);
  readonly selectedMode = computed(() => this.leverageMode());

  private readonly apiBase = '/api/v1';
  private readonly wsBase = this.resolveWsBase();
  private socket: WebSocket | null = null;
  private destroy$ = new Subject<void>();
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private readonly intentionalCloseSockets = new WeakSet<WebSocket>();
  private readonly sequenceByType = new Map<string, number>();

  constructor() {
    this.refreshIntervalMs.set(this.loadRefreshInterval());
    this.bootstrap(this.selectedSymbol(), this.interval());
    this.loadTradingStatus();
    this.startHealthWatch();
  }

  setSymbol(symbol: string) {
    if (this.selectedSymbol() === symbol) return;
    this.selectedSymbol.set(symbol);
    this.bootstrap(symbol, this.interval());
  }

  setInterval(intervalMs: string) {
    if (this.interval() === intervalMs) return;
    this.interval.set(intervalMs);
    this.bootstrap(this.selectedSymbol(), intervalMs);
  }

  setRefreshInterval(valueMs: number) {
    const next = clampRefreshInterval(valueMs);
    if (next === this.refreshIntervalMs()) return;
    this.refreshIntervalMs.set(next);
    if (isPlatformBrowser(this.platformId)) {
      localStorage.setItem(LOCAL_STORAGE_REFRESH_INTERVAL_KEY, `${next}`);
    }
    this.rescheduleRecovery();
  }

  getRefreshIntervalBounds() {
    return {
      min: MIN_REFRESH_INTERVAL_MS,
      max: MAX_REFRESH_INTERVAL_MS,
      step: REFRESH_INTERVAL_STEP_MS,
    };
  }

  bootstrap(symbol = this.selectedSymbol(), intervalValue = this.interval()): void {
    this.loading.set(true);
    this.error.set(null);
    this.http
      .get<MarketSnapshot>(
        `${this.apiBase}/bootstrap?symbol=${encodeURIComponent(symbol)}&interval=${encodeURIComponent(intervalValue)}`,
      )
      .pipe(
        catchError((error) => {
          const backendError = this.readMessage(error);
          this.error.set(`Backend unavailable (${backendError}). Using demo fallback.`);
          this.applySnapshot({ ...demoSnapshot, market: this.findMarket(symbol) }, true, 'offline');
          return of(null);
        }),
      )
      .subscribe((payload) => {
        if (!payload) return;
        this.applySnapshot(payload, false);
      });
    this.openStream(symbol, intervalValue);
  }

  updateLeverage(payload: LeverageRequest): Observable<OrderWriteResult> {
    if (this.isDemo()) {
      return of(this.disabledWriteResponse());
    }
    return this.http
      .put<OrderWriteResult>(
        `${this.apiBase}/positions/${encodeURIComponent(this.selectedSymbol())}/leverage`,
        payload,
      )
      .pipe(
        tap((result) => {
          if (result.status === 'ok') {
            this.leverageMode.set(payload.mode);
          }
        }),
        catchError((error) => of(this.errorResponse(error))),
      );
  }

  closePosition(
    payload: ClosePositionRequest,
    symbol = this.selectedSymbol(),
  ): Observable<OrderWriteResult> {
    if (this.isDemo()) {
      return of(this.disabledWriteResponse());
    }
    return this.http
      .post<OrderWriteResult>(
        `${this.apiBase}/positions/${encodeURIComponent(symbol)}/close`,
        payload,
      )
      .pipe(catchError((error) => of(this.errorResponse(error))));
  }

  submitOrder(order: OrderRequest): Observable<OrderWriteResult> {
    const payload = {
      ...order,
      timeInForce: order.timeInForce ?? 'gtc',
      reduceOnly: !!order.reduceOnly,
    };
    if (this.isDemo()) {
      return of(this.disabledWriteResponse());
    }
    return this.http
      .post<OrderWriteResult>(`${this.apiBase}/orders`, payload)
      .pipe(catchError((error) => of(this.errorResponse(error))));
  }

  modifyOrder(oid: string, request: ModifyOrderRequest): Observable<OrderWriteResult> {
    if (this.isDemo()) {
      return of(this.disabledWriteResponse());
    }
    return this.http
      .patch<OrderWriteResult>(`${this.apiBase}/orders/${encodeURIComponent(oid)}`, request)
      .pipe(catchError((error) => of(this.errorResponse(error))));
  }

  cancelOrder(oid: string): Observable<OrderWriteResult> {
    if (this.isDemo()) {
      return of(this.disabledWriteResponse());
    }
    return this.http
      .delete<OrderWriteResult>(
        `${this.apiBase}/orders/${encodeURIComponent(oid)}?symbol=${encodeURIComponent(this.selectedSymbol())}`,
      )
      .pipe(catchError((error) => of(this.errorResponse(error))));
  }

  cancelAll(symbol = this.selectedSymbol()): Observable<OrderWriteResult> {
    if (this.isDemo()) {
      return of(this.disabledWriteResponse());
    }
    return this.http
      .delete<OrderWriteResult>(`${this.apiBase}/orders?symbol=${encodeURIComponent(symbol)}`)
      .pipe(catchError((error) => of(this.errorResponse(error))));
  }

  loadTradingStatus(): void {
    this.http
      .get<TradingStatus>(`${this.apiBase}/trading`)
      .pipe(catchError(() => of(this.tradingStatus())))
      .subscribe((status) => this.applyTradingStatus(status));
  }

  setTradingEnabled(enabled: boolean): Observable<TradingStatus> {
    return this.http
      .put<TradingStatus>(`${this.apiBase}/trading`, { enabled })
      .pipe(tap((status) => this.applyTradingStatus(status)));
  }

  setNetwork(network: NetworkName): Observable<NetworkStatus> {
    return this.http.put<NetworkStatus>(`${this.apiBase}/network`, { network }).pipe(
      tap((status) => {
        this.networkStatus.set(status);
        this.tradingStatus.set({
          available: status.tradingAvailable,
          enabled: status.tradingEnabled,
          network: status.network,
        });
        this.sequenceByType.clear();
        this.bootstrap(this.selectedSymbol(), this.interval());
      }),
    );
  }

  errorMessage(error: unknown): string {
    return this.readMessage(error);
  }

  destroy() {
    this.closeSocket(true);
    this.destroy$.next();
    this.destroy$.complete();
  }

  private openStream(symbol: string, intervalValue: string): void {
    this.closeSocket(true);
    this.connection.set({ phase: 'reconnecting', detail: `Connecting ${symbol}/${intervalValue}` });
    const url = `${this.wsBase}/stream?symbol=${encodeURIComponent(symbol)}&interval=${encodeURIComponent(intervalValue)}`;
    const socket = new WebSocket(url);
    this.socket = socket;
    socket.addEventListener('open', () => {
      if (this.isDemo()) {
        this.isDemo.set(false);
      }
      this.connection.set({ phase: 'online', detail: `Streaming ${symbol}` });
      this.error.set(null);
      this.reconnectAttempt = 0;
    });
    socket.addEventListener('message', (event) => {
      try {
        const payload = JSON.parse(event.data) as StreamEnvelope;
        this.applyStream(payload);
      } catch {
        // ignore malformed payload
      }
    });
    socket.addEventListener('error', () => {
      this.connection.set({ phase: 'offline', detail: 'Socket error' });
    });
    socket.addEventListener('close', () => {
      if (this.intentionalCloseSockets.has(socket)) {
        this.intentionalCloseSockets.delete(socket);
        return;
      }
      if (this.destroy$.isStopped) return;
      this.scheduleReconnect(symbol, intervalValue);
    });
  }

  private closeSocket(suppressReconnect = false): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    const socket = this.socket;
    if (!socket) {
      return;
    }
    if (suppressReconnect) {
      this.intentionalCloseSockets.add(socket);
    }
    socket.close();
    this.socket = null;
  }

  private scheduleReconnect(symbol: string, intervalValue: string): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    const wait = this.computeReconnectDelay(this.reconnectAttempt);
    const scheduledAttempt = this.reconnectAttempt;
    this.connection.set({
      phase: 'reconnecting',
      detail: `Reconnecting in ${wait}ms`,
    });
    this.reconnectAttempt = Math.max(scheduledAttempt + 1, this.reconnectAttempt);
    this.reconnectTimer = setTimeout(() => {
      this.openStream(symbol, intervalValue);
    }, wait);
  }

  private rescheduleRecovery(): void {
    if (this.connection().phase === 'online') {
      return;
    }
    const nextAttempt = Math.max(0, this.reconnectAttempt - 1);
    this.reconnectAttempt = nextAttempt;
    this.scheduleReconnect(this.selectedSymbol(), this.interval());
  }

  private computeReconnectDelay(attempt: number): number {
    const safeAttempt = Math.max(0, attempt);
    const delay = this.refreshIntervalMs() * Math.pow(2, safeAttempt);
    return Math.max(MIN_REFRESH_INTERVAL_MS, Math.min(MAX_RECONNECT_DELAY_MS, delay));
  }

  private loadRefreshInterval() {
    if (!isPlatformBrowser(this.platformId)) return DEFAULT_REFRESH_INTERVAL_MS;
    const raw =
      localStorage.getItem(LOCAL_STORAGE_REFRESH_INTERVAL_KEY) ??
      LOCAL_STORAGE_REFRESH_INTERVAL_FALLBACK;
    const parsed = raw ? Number.parseInt(raw, 10) : DEFAULT_REFRESH_INTERVAL_MS;
    return clampRefreshInterval(parsed);
  }

  private applySnapshot(
    snapshot: MarketSnapshot,
    demo = false,
    connectionPhase?: 'offline' | 'reconnecting' | 'online',
  ): void {
    this.loading.set(false);
    this.markets.set(snapshot.markets);
    this.market.set(snapshot.market);
    this.selectedSymbol.set(snapshot.market.symbol);
    this.candles.set(snapshot.candles);
    this.book.set(snapshot.book);
    this.trades.set(snapshot.trades);
    this.positions.set(snapshot.account.positions);
    this.orders.set(snapshot.orders.open);
    this.triggerOrders.set(snapshot.orders.trigger as unknown as TriggerOrder[]);
    this.fills.set(snapshot.fills);
    this.funding.set(snapshot.funding);
    this.assets.set(snapshot.assets);
    this.margin.set(snapshot.account.margin);
    this.leverageMode.set(snapshot.account.leverageMode);
    this.isDemo.set(demo);
    this.sequenceByType.clear();
    const phase = connectionPhase ?? (demo ? 'offline' : 'online');
    const detail = demo
      ? 'Demo mode'
      : phase === 'online'
        ? 'Live snapshot synced'
        : 'Live fallback synced';
    this.connection.set({ phase, detail });
  }

  private applyStream(envelope: StreamEnvelope<unknown>): void {
    if (envelope.symbol && envelope.symbol !== this.selectedSymbol()) return;
    if (!this.shouldApply(envelope.type, envelope.sequence) && envelope.type !== 'snapshot') return;

    switch (envelope.type) {
      case 'snapshot':
        this.applySnapshot(envelope.data as MarketSnapshot, false);
        break;
      case 'markets':
        this.markets.set(envelope.data as MarketMeta[]);
        break;
      case 'assetContext':
        this.market.update((current) => {
          const base = current ?? this.findMarket(envelope.symbol ?? this.selectedSymbol());
          return mergeMarketContext(base, envelope.data as Partial<MarketMeta>);
        });
        this.markets.update((markets) =>
          markets.map((item) => {
            if (item.symbol !== (envelope.symbol ?? this.selectedSymbol())) {
              return item;
            }
            return mergeMarketContext(item, envelope.data as Partial<MarketMeta>);
          }),
        );
        break;
      case 'book':
        this.book.set(envelope.data as BookLevel[]);
        break;
      case 'trades':
        this.trades.update((trades) => [...(envelope.data as Trade[]), ...trades].slice(0, 80));
        break;
      case 'candle':
        this.candles.update((candles) => [...candles.slice(-240), envelope.data as Candle]);
        break;
      case 'account':
        const accountUpdate = envelope.data as AccountUpdate;
        if (accountUpdate.margin) {
          this.margin.set(accountUpdate.margin);
        }
        if (accountUpdate.positions) {
          this.positions.set(accountUpdate.positions);
        }
        if (accountUpdate.balances) {
          this.assets.set(accountUpdate.balances);
        }
        if (accountUpdate.assets) {
          this.assets.set(accountUpdate.assets);
        }
        if (accountUpdate.leverageMode) {
          this.leverageMode.set(accountUpdate.leverageMode);
        }
        break;
      case 'orders':
        const orderUpdate = envelope.data as { open?: Order[]; trigger?: TriggerOrder[] };
        if (orderUpdate.open) {
          this.orders.set(orderUpdate.open);
        }
        if (orderUpdate.trigger) {
          this.triggerOrders.set(orderUpdate.trigger);
        }
        break;
      case 'fills':
        this.fills.update((fills) => [...(envelope.data as Fill[]), ...fills].slice(0, 100));
        break;
      case 'funding':
        this.funding.update((funding) =>
          [...(envelope.data as Funding[]), ...funding].slice(0, 50),
        );
        break;
      case 'connection':
        this.connection.set({
          phase: (envelope.data as { online: boolean }).online ? 'online' : 'offline',
          detail: `Upstream ${(envelope.data as { online: boolean }).online ? 'healthy' : 'disconnected'}`,
        });
        break;
      case 'error':
        this.error.set(
          (envelope.data as { detail?: string; title?: string }).detail || 'Stream error',
        );
        this.connection.set({ phase: 'offline', detail: 'Stream error' });
        break;
    }
  }

  private shouldApply(type: string, sequence: number): boolean {
    const last = this.sequenceByType.get(type);
    if (last === undefined || sequence > last) {
      this.sequenceByType.set(type, sequence);
      return true;
    }
    return false;
  }

  private startHealthWatch(): void {
    if (!isPlatformBrowser(this.platformId)) return;
    interval(15000)
      .pipe(
        takeUntil(this.destroy$),
        switchMap(() =>
          this.http.get<Health>(`${this.apiBase}/health`).pipe(catchError(() => of(null))),
        ),
      )
      .subscribe((health) => {
        if (!health) return;
        const networkChanged = health.network !== this.networkStatus().network;
        this.health.set(health);
        this.applyTradingStatus({
          available: health.tradingAvailable,
          enabled: health.tradingEnabled,
          network: health.network,
        });
        if (networkChanged && (health.network === 'testnet' || health.network === 'mainnet')) {
          this.bootstrap(this.selectedSymbol(), this.interval());
        }
      });
  }

  private applyTradingStatus(status: TradingStatus): void {
    this.tradingStatus.set(status);
    this.networkStatus.update((current) => ({
      ...current,
      network: status.network,
      tradingAvailable: status.available,
      tradingEnabled: status.enabled,
    }));
  }

  private findMarket(symbol: string): MarketMeta {
    const market = this.markets().find((item) => item.symbol === symbol);
    return market ?? demoSnapshot.markets[0];
  }

  private readMessage(error: unknown): string {
    if (error instanceof HttpErrorResponse) {
      return error.error?.detail || error.message || `HTTP ${error.status}`;
    }
    return 'network issue';
  }

  private errorResponse(error: unknown): OrderWriteResult {
    return {
      requestId: createRequestId('err'),
      status: 'error',
      message: this.readMessage(error),
    };
  }

  private disabledWriteResponse(): OrderWriteResult {
    return {
      requestId: createRequestId('disabled'),
      status: 'error',
      message: 'Trading is unavailable in demo mode.',
    };
  }

  private resolveWsBase(): string {
    if (!isPlatformBrowser(this.platformId)) return 'ws://localhost/api/v1';
    return window.location.origin.replace(/^http/i, 'ws') + '/api/v1';
  }
}

type AccountUpdate = {
  margin?: {
    crossBalance: string;
    availableBalance: string;
    totalMarginUsed: string;
  };
  positions?: Position[];
  balances?: Asset[];
  assets?: Asset[];
  leverageMode?: 'cross' | 'isolated';
};

function mergeMarketContext(base: MarketMeta, partial: Partial<MarketMeta>): MarketMeta {
  const incoming = partial as Partial<
    MarketMeta & { active?: boolean; leverage?: Partial<MarketMeta['leverage']> }
  >;
  const text = (value: unknown): string | undefined =>
    typeof value === 'string' && value.trim().length > 0 ? value : undefined;
  const positiveNumber = (value: unknown): value is number =>
    typeof value === 'number' && Number.isFinite(value) && value > 0;

  const nextLeverage = { ...base.leverage };
  if (incoming.leverage && typeof incoming.leverage === 'object') {
    const incomingLeverage = incoming.leverage;
    if (typeof incomingLeverage.maxLeverage === 'number' && incomingLeverage.maxLeverage > 1) {
      nextLeverage.maxLeverage = incomingLeverage.maxLeverage;
    }
    if (positiveNumber(incomingLeverage.currentLeverage)) {
      nextLeverage.currentLeverage = incomingLeverage.currentLeverage;
    }
    if (incomingLeverage.currentMode === 'cross' || incomingLeverage.currentMode === 'isolated') {
      nextLeverage.currentMode = incomingLeverage.currentMode;
    }
  }

  const currentActive = (base as { active?: boolean }).active;
  const incomingActive = (incoming as { active?: boolean }).active;
  const nextActive =
    incomingActive === undefined ? currentActive : incomingActive === true ? true : currentActive;

  return {
    ...base,
    symbol: text(incoming.symbol) ?? base.symbol,
    base: text(incoming.base) ?? base.base,
    quote: text(incoming.quote) ?? base.quote,
    indexName: text(incoming.indexName) ?? base.indexName,
    markPx: text(incoming.markPx) ?? base.markPx,
    funding: text(incoming.funding) ?? base.funding,
    baseDecimals: positiveNumber(incoming.baseDecimals) ? incoming.baseDecimals : base.baseDecimals,
    quoteDecimals: positiveNumber(incoming.quoteDecimals)
      ? incoming.quoteDecimals
      : base.quoteDecimals,
    leverage: nextLeverage,
    ...(currentActive === undefined && incomingActive === undefined
      ? {}
      : {
          active: nextActive,
        }),
  };
}

function createRequestId(prefix: string) {
  const fallback = Math.floor(Math.random() * 2_147_483_647);
  const random = globalThis.crypto?.getRandomValues(new Uint32Array(1))[0] ?? fallback;
  return `${prefix}-${random}`;
}

function clampRefreshInterval(ms: number): number {
  if (!Number.isFinite(ms)) {
    return DEFAULT_REFRESH_INTERVAL_MS;
  }
  const safeStep = Math.max(
    1,
    Math.round(ms / REFRESH_INTERVAL_STEP_MS) * REFRESH_INTERVAL_STEP_MS,
  );
  return Math.max(MIN_REFRESH_INTERVAL_MS, Math.min(MAX_REFRESH_INTERVAL_MS, safeStep));
}
