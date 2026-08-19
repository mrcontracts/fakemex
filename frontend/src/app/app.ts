import {
  AfterViewInit,
  Component,
  HostListener,
  OnDestroy,
  OnInit,
  computed,
  effect,
  ElementRef,
  inject,
  signal,
  ViewChild,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormControl, FormsModule, ReactiveFormsModule } from '@angular/forms';
import { GridStack, GridStackWidget } from 'gridstack';
import { MatAutocompleteModule, MatAutocompleteSelectedEvent } from '@angular/material/autocomplete';
import { MatButtonModule } from '@angular/material/button';
import { MatCheckboxModule } from '@angular/material/checkbox';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSelectChange, MatSelectModule } from '@angular/material/select';
import { MatTabsModule } from '@angular/material/tabs';
import { MatSliderModule } from '@angular/material/slider';
import { MatSlideToggleModule } from '@angular/material/slide-toggle';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';
import { MatToolbarModule } from '@angular/material/toolbar';
import { Subject, finalize } from 'rxjs';
import { takeUntil } from 'rxjs/operators';
import { FakeMexDataService } from './services/fakemex-data.service';
import { LayoutPreset, LayoutService, PanelId, GridLayoutItem } from './services/layout.service';
import { ThemeService } from './services/theme.service';
import { Position, TriggerOrder, Fill, Funding, OrderWriteResult } from './models';
import { PanelShellComponent } from './components/panel-shell.component';
import { MarketChartComponent } from './components/market-chart.component';
import { DepthChartComponent } from './components/depth-chart.component';
import { OrderBookComponent } from './components/order-book.component';
import { RecentTradesComponent } from './components/recent-trades.component';
import { OrderFormComponent } from './components/order-form.component';
import { SimpleTableComponent } from './components/simple-table.component';
import { OrderRequest } from './models';

const panelTitles: Record<PanelId, string> = {
  'chart': 'Chart',
  'order-form': 'Order entry',
  'depth': 'Depth',
  'book': 'Order book',
  'trades': 'Recent trades',
  'positions': 'Positions',
  'orders': 'Open orders',
  'triggers': 'Triggers',
  'fills': 'Fills',
  'history': 'History',
  'funding': 'Funding',
  'assets': 'Assets',
  'leverage': 'Leverage',
  'settings': 'Settings',
};

const panelGeometryDefaults: Record<
  PanelId,
  {
    minW: number;
    minH: number;
    maxH?: number;
  }
> = {
  'order-form': { minW: 2, minH: 6 },
  'chart': { minW: 5, minH: 8 },
  'depth': { minW: 2, minH: 6 },
  'book': { minW: 2, minH: 4 },
  'trades': { minW: 2, minH: 4 },
  'positions': { minW: 2, minH: 3 },
  'orders': { minW: 2, minH: 3 },
  'triggers': { minW: 2, minH: 3 },
  'fills': { minW: 2, minH: 3 },
  'history': { minW: 2, minH: 3 },
  'funding': { minW: 2, minH: 3 },
  'assets': { minW: 2, minH: 3 },
  'leverage': { minW: 2, minH: 2 },
  'settings': { minW: 2, minH: 2 },
};

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    ReactiveFormsModule,
    PanelShellComponent,
    MarketChartComponent,
    DepthChartComponent,
    OrderBookComponent,
    RecentTradesComponent,
    OrderFormComponent,
    SimpleTableComponent,
    MatToolbarModule,
    MatFormFieldModule,
    MatSelectModule,
    MatInputModule,
    MatAutocompleteModule,
    MatButtonModule,
    MatCheckboxModule,
    MatTabsModule,
    MatSliderModule,
    MatSlideToggleModule,
    MatSnackBarModule,
  ],
  templateUrl: './app.html',
  styleUrls: ['./app.scss'],
})
export class App implements OnInit, OnDestroy, AfterViewInit {
  private readonly api = inject(FakeMexDataService);
  private readonly snackBar = inject(MatSnackBar);
  readonly layout = inject(LayoutService);
  readonly theme = inject(ThemeService);
  private readonly destroy$ = new Subject<void>();
  private grid?: GridStack | null;
  @ViewChild('gridRoot') private gridRoot?: ElementRef<HTMLElement>;

  readonly data = this.api;
  readonly isNarrow = signal(false);
  readonly activeTab = signal<PanelId>('chart');
  readonly layoutItems = computed(() => this.layout.activeLayout());
  readonly layoutPresets: LayoutPreset[] = ['basic', 'advanced', 'charting', 'custom'];
  readonly panelTitles = panelTitles;
  readonly connectionMode = computed(() => this.data.connection().phase);
  readonly tradingStatus = computed(() => this.data.tradingStatus());
  readonly tradingToggleBusy = signal(false);
  readonly panelTabs: readonly PanelId[] = this.layout.panelIds;
  readonly activeTabIndex = computed(() => {
    const index = this.panelTabs.indexOf(this.activeTab());
    return index >= 0 ? index : 0;
  });
  readonly themePickerControl = new FormControl(this.theme.activeTheme().label, { nonNullable: true });

  readonly statusText = computed(() => {
    if (this.data.loading()) return 'LOADING';
    if (this.connectionMode() === 'reconnecting') return 'RECONNECTING';
    if (this.data.isDemo()) return 'DEMO';
    if (this.connectionMode() === 'online') return 'LIVE';
    return 'OFFLINE';
  });

  readonly chartStatus = computed(() => {
    if (this.data.isDemo()) return 'Demo';
    return this.connectionMode() === 'online' ? 'Live' : 'Offline';
  });

  readonly connectionTone = computed(() => {
    if (this.data.isDemo()) return 'demo';
    if (this.data.hasConnection() && !this.data.loading() && this.connectionMode() === 'online') return 'online';
    if (this.connectionMode() === 'reconnecting') return 'reconnecting';
    return 'offline';
  });

  readonly connectionBannerText = computed(() => {
    if (this.data.loading()) return 'Loading market state…';
    if (this.data.isDemo()) {
      if (this.connectionMode() === 'reconnecting') return 'Demo stream reconnecting';
      if (this.connectionMode() === 'online') return 'Demo connection active';
      return 'Demo / offline';
    }
    if (this.connectionMode() === 'online') return 'Live stream connected';
    if (this.connectionMode() === 'reconnecting') return this.data.connection().detail ?? 'Reconnecting';
    if (this.data.error()) return this.data.error();
    return 'Backend unavailable';
  });

  readonly statusToneClass = computed(() => {
    if (this.data.isDemo()) return 'status-demo';
    if (this.connectionMode() === 'online' && !this.data.loading()) return 'status-live';
    if (this.connectionMode() === 'reconnecting') return 'status-reconnect';
    return 'status-offline';
  });

  readonly marginMode = computed(() => {
    const explicit = this.data.selectedMode();
    return explicit === 'isolated' ? 'ISOLATED' : 'CROSS';
  });

  readonly marginModeToneClass = computed(() => {
    return this.data.selectedMode() === 'isolated' ? 'margin-mode-isolated' : 'margin-mode-cross';
  });

  readonly selectedMarket = computed(() => this.data.market());
  readonly marketMark = computed(() => this.selectedMarket()?.markPx ?? '--');
  readonly marketFunding = computed(() => this.selectedMarket()?.funding ?? '--');
  readonly marketLeverage = computed(() => this.selectedMarket()?.leverage.currentLeverage ?? '--');
  readonly marketMarkStatus = computed(() => {
    const candles = this.data.candles();
    if (candles.length < 2) return '—';
    const latest = Number(candles[candles.length - 1].c);
    const prior = Number(candles[candles.length - 2].c);
    if (!Number.isFinite(latest) || !Number.isFinite(prior) || prior === 0) return '—';
    const delta = ((latest - prior) / prior) * 100;
    return `${delta >= 0 ? '+' : ''}${delta.toFixed(2)}%`;
  });

  readonly positionsRows = computed(() =>
    this.api.positions().map((position: Position) => [
      position.symbol,
      position.side,
      position.size,
      position.entryPrice,
      position.markPrice,
      position.unrealizedPnl,
      `${position.leverage}x`,
      position.liquidation ?? '--',
    ]),
  );
  readonly positionCellClass = (cell: unknown, _row: readonly unknown[], columnIndex: number): string => {
    const value = String(cell).trim().toLowerCase();
    if (columnIndex === 1) {
      if (value === 'buy') return 'cell-buy';
      if (value === 'sell') return 'cell-sell';
    }
    if (columnIndex === 5) {
      const numericValue = Number(value.replace(/[,\s%$]/g, ''));
      if (Number.isFinite(numericValue) && numericValue > 0) return 'cell-positive';
      if (Number.isFinite(numericValue) && numericValue < 0) return 'cell-negative';
    }
    return '';
  };
  readonly ordersRows = computed(() =>
    this.api.orders().map((order) => [
      order.oid,
      order.symbol,
      order.side,
      order.kind,
      order.size,
      order.price ?? '--',
      order.reduceOnly ? 'RO' : 'No',
      order.status,
    ]),
  );
  readonly triggerRows = computed(() =>
    this.api.triggerOrders().map((order: TriggerOrder) => [
      order.oid,
      order.symbol,
      order.side,
      order.kind,
      order.triggerPrice,
      order.price ?? '--',
      order.status,
    ]),
  );
  readonly fillsRows = computed(() =>
    this.api.fills().map((fill: Fill) => [fill.fillId, fill.symbol, fill.side, fill.px, fill.size, fill.fee]),
  );
  readonly historyRows = computed(() =>
    this.api.fills().slice(0, 12).map((fill: Fill) => [
      new Date(fill.ts).toLocaleTimeString(),
      fill.symbol,
      fill.side,
      fill.px,
      fill.size,
    ]),
  );
  readonly fundingRows = computed(() =>
    this.api.funding().map((funding: Funding) => [
      funding.symbol,
      funding.rate,
      funding.payment,
      new Date(funding.ts).toLocaleTimeString(),
    ]),
  );
  readonly assetsRows = computed(() =>
    this.api.assets().map((asset) => [asset.coin, asset.wallet, asset.available, asset.crossMarginUsed]),
  );

  readonly leverageValue = signal(0);
  readonly refreshBounds = this.data.getRefreshIntervalBounds();
  readonly refreshFrequency = computed(() => `${this.data.refreshIntervalMs()}ms`);
  readonly refreshFrequencyLabel = computed(() => {
    const interval = this.data.refreshIntervalMs();
    if (interval <= 750) return 'Fast recovery';
    if (interval >= 4000) return 'Slow recovery';
    return 'Recovery';
  });

  constructor() {
    this.layout.ensurePersisted();
    effect(() => {
      if (!this.grid || this.isNarrow()) {
        return;
      }
      this.grid.load(
        this.normalizeGridLayout(this.layoutItems()).map((item: GridLayoutItem) =>
          ({
            id: item.id,
            x: item.x,
            y: item.y,
            w: item.w,
            h: item.h,
            minW: this.panelGeometry(item.id).minW,
            minH: this.panelGeometry(item.id).minH,
            maxH: this.panelGeometry(item.id).maxH,
          }) as GridStackWidget,
        ),
      );
    });

    effect(() => {
      this.themePickerControl.setValue(this.theme.activeTheme().label, {
        emitEvent: false,
      });
      this.theme.setSearchQuery('');
    });
  }

  ngOnInit() {
    this.updateViewportState();
    this.leverageValue.set(this.api.selectedLeverage());
  }

  ngAfterViewInit() {
    if (!this.isNarrow()) {
      this.initGrid();
    }
  }

  ngOnDestroy() {
    this.grid?.destroy();
    this.destroy$.next();
    this.destroy$.complete();
    this.api.destroy();
  }

  @HostListener('window:resize')
  onResize() {
    this.updateViewportState();
    this.grid?.onResize();
  }

  selectSymbol(symbol: string) {
    this.api.setSymbol(symbol);
    this.leverageValue.set(this.api.selectedLeverage());
  }

  onSymbolSelection(event: MatSelectChange) {
    const symbol = event.value as string;
    if (symbol) {
      this.selectSymbol(symbol);
    }
  }

  selectInterval(interval: string) {
    this.api.setInterval(interval);
  }

  submitOrder(payload: OrderRequest) {
    if (!this.guardTrading()) return;
    this.api.submitOrder(payload)
      .pipe(takeUntil(this.destroy$))
      .subscribe((result) => this.handleWriteResult(result));
  }

  sendLeverage(mode: 'cross' | 'isolated') {
    if (!this.guardTrading()) return;
    this.api.updateLeverage({ mode, leverage: this.leverageValue() })
      .pipe(takeUntil(this.destroy$))
      .subscribe((result) => this.handleWriteResult(result));
  }

  setLeverage(value: number) {
    const safeMax = Math.max(1, this.data.selectedMarket()?.leverage.maxLeverage ?? 100);
    const safeValue = Number.isFinite(value) ? Math.max(1, Math.min(Math.trunc(value), safeMax)) : 1;
    this.leverageValue.set(safeValue);
  }

  closePositions(percent: 25 | 50 | 75 | 100) {
    if (!this.guardTrading()) return;
    this.api.closePosition({ percent, kind: 'market' })
      .pipe(takeUntil(this.destroy$))
      .subscribe((result) => this.handleWriteResult(result));
  }

  onTradingToggle(enabled: boolean) {
    if (this.tradingToggleBusy()) return;
    if (enabled && !this.tradingStatus().available) {
      this.showTradingWarning('Trading is unavailable. Enable signed testnet trading in config/local.env and restart the backend.');
      return;
    }
    this.tradingToggleBusy.set(true);
    this.api.setTradingEnabled(enabled)
      .pipe(
        takeUntil(this.destroy$),
        finalize(() => this.tradingToggleBusy.set(false)),
      )
      .subscribe({
        next: (status) => {
          if (!status.enabled) {
            this.showTradingWarning('Trading is disabled. Orders and position changes will be blocked.');
          }
        },
        error: () => {
          this.api.loadTradingStatus();
          this.showTradingWarning('The backend refused the trading toggle. Trading remains disabled.');
        },
      });
  }

  setPreset(preset: LayoutPreset) {
    this.layout.setPreset(preset);
    if (!this.isNarrow()) {
      this.initGrid();
    }
  }

  setDensity(value: 'compact' | 'normal' | 'spacious') {
    this.layout.setDensity(value);
    if (!this.isNarrow()) {
      this.initGrid();
    }
  }

  onPresetSelection(event: MatSelectChange) {
    this.setPreset(event.value as LayoutPreset);
  }

  onDensitySelection(event: MatSelectChange) {
    this.setDensity(event.value as 'compact' | 'normal' | 'spacious');
  }

  onThemeSearch(event: Event) {
    this.theme.setSearchQuery((event.target as HTMLInputElement).value);
  }

  onThemeOptionSelected(event: MatAutocompleteSelectedEvent) {
    this.theme.setThemeByLabel(event.option.value);
  }

  onThemeApply() {
    const query = this.themePickerControl.value.trim();
    const didApply = this.theme.setThemeByLabel(query);
    if (!didApply) {
      this.themePickerControl.setValue(this.theme.activeTheme().label, { emitEvent: false });
      return;
    }
    this.themePickerControl.setValue(this.theme.activeTheme().label, { emitEvent: false });
    this.theme.setSearchQuery('');
  }

  onRefreshFrequency(valueOrEvent: number | Event) {
    const value = typeof valueOrEvent === 'number'
      ? valueOrEvent
      : Number((valueOrEvent.target as HTMLInputElement).value);
    if (!Number.isFinite(value)) return;
    this.data.setRefreshInterval(value);
  }

  onMobileTabChange(index: number) {
    const nextTab = this.panelTabs[index];
    if (nextTab) {
      this.activeTab.set(nextTab);
    }
  }

  private initGrid() {
    if (!this.gridRoot?.nativeElement) return;
    if (this.grid) {
      this.grid.destroy(false);
    }
    this.grid = GridStack.init(
      {
        column: 12,
        float: true,
        cellHeight: this.densityCellHeight(),
        margin: this.densityMargin(),
        sizeToContent: false,
        alwaysShowResizeHandle: true,
        resizable: {
          handles: 's,e,se',
          autoHide: false,
        },
      },
      this.gridRoot.nativeElement,
    );
    if (!this.grid) return;
    this.grid.load(
      this.normalizeGridLayout(this.layoutItems()).map(
        (item: GridLayoutItem) =>
          ({
            id: item.id,
            x: item.x,
            y: item.y,
            w: item.w,
            h: item.h,
            minW: this.panelGeometry(item.id).minW,
            minH: this.panelGeometry(item.id).minH,
            maxH: this.panelGeometry(item.id).maxH,
          }) as GridStackWidget,
      ),
    );
    this.grid.on('change', () => {
      const current = this.grid?.save(false) as GridStackWidget[] | null | undefined;
      if (!current || !Array.isArray(current)) {
        return;
      }
      const custom = current
        .map((node) => ({
          id: String(node.id),
          x: node.x ?? 0,
          y: node.y ?? 0,
          w: node.w ?? 1,
          h: node.h ?? 1,
        }))
        .filter((item): item is { id: string; x: number; y: number; w: number; h: number } => this.isPanelId(item.id))
        .map((item) => this.normalizeGridLayout([{ ...item, id: item.id as PanelId }])[0])
        .filter((item): item is GridLayoutItem => Boolean(item));
      this.layout.setCustomLayout(custom);
    });
  }

  private isPanelId(value: string): value is PanelId {
    return (
      value === 'order-form' ||
      value === 'chart' ||
      value === 'depth' ||
      value === 'book' ||
      value === 'trades' ||
      value === 'positions' ||
      value === 'orders' ||
      value === 'triggers' ||
      value === 'fills' ||
      value === 'history' ||
      value === 'funding' ||
      value === 'assets' ||
      value === 'leverage' ||
      value === 'settings'
    );
  }

  private normalizeGridLayout(items: GridLayoutItem[]): GridLayoutItem[] {
    return items
      .filter((item): item is GridLayoutItem => Boolean(item?.id))
      .map((item) => {
        const geometry = this.panelGeometry(item.id);
        const safeX = Math.min(11, Math.max(0, item.x));
        const safeW = Math.min(12 - safeX, Math.max(1, item.w));
        const safeY = Math.max(0, item.y);
        const maxH = geometry.maxH ?? Number.MAX_SAFE_INTEGER;
        const safeH = Math.max(geometry.minH, Math.min(maxH, item.h));
        return { ...item, x: safeX, w: safeW, y: safeY, h: safeH };
      });
  }

  private panelGeometry(id: PanelId) {
    return panelGeometryDefaults[id];
  }

  private densityCellHeight() {
    switch (this.layout.density()) {
      case 'compact':
        return 44;
      case 'spacious':
        return 70;
      case 'normal':
      default:
        return 58;
    }
  }

  private densityMargin() {
    switch (this.layout.density()) {
      case 'compact':
        return 6;
      case 'spacious':
        return 10;
      case 'normal':
      default:
        return 8;
    }
  }

  private updateViewportState() {
    if (typeof window === 'undefined') return;
    const nextNarrow = window.innerWidth <= 1160;
    if (nextNarrow !== this.isNarrow()) {
      this.isNarrow.set(nextNarrow);
      if (!nextNarrow) {
        queueMicrotask(() => this.initGrid());
      }
      if (nextNarrow && this.grid) {
        this.grid.destroy();
        this.grid = undefined;
      }
    }
  }

  private guardTrading(): boolean {
    const status = this.tradingStatus();
    if (!status.available) {
      this.showTradingWarning('Trading is unavailable. Signed testnet trading is not configured in the backend.');
      return false;
    }
    if (!status.enabled) {
      this.showTradingWarning('Trading is disabled. Turn on the Trading toggle before sending an order.');
      return false;
    }
    return true;
  }

  private handleWriteResult(result: OrderWriteResult): void {
    if (result.status === 'error') {
      this.showTradingWarning(result.message || 'The exchange rejected the trading action.');
    }
  }

  private showTradingWarning(message: string): void {
    this.snackBar.open(message, 'Dismiss', {
      duration: 7000,
      horizontalPosition: 'center',
      verticalPosition: 'top',
      panelClass: ['trading-warning'],
    });
  }
}
