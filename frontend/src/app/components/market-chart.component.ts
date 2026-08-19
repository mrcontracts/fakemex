import {
  AfterViewInit,
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  effect,
  EventEmitter,
  Input,
  NgZone,
  OnChanges,
  OnDestroy,
  Output,
  SimpleChanges,
  inject,
  ViewChild,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { CandlestickData, CandlestickSeries, createChart, CrosshairMode, IChartApi, Time, UTCTimestamp } from 'lightweight-charts';
import { Candle } from '../models';
import { ThemeService } from '../services/theme.service';

@Component({
  selector: 'app-market-chart',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CommonModule],
  template: `
    <div class="chart-toolbar">
      <button
        type="button"
        class="interval"
        *ngFor="let duration of intervals"
        (click)="intervalRequested.emit(duration)"
        [class.active]="duration === selectedInterval"
      >
        {{ duration }}
      </button>
      <span class="toolbar-spacer"></span>
      <span class="status-pill" [class.offline]="statusTone() === 'offline'" [class.demo]="statusTone() === 'demo'">
        {{ connectionStatusLabel() }}
      </span>
    </div>
    <div class="chart-root" #chartRoot aria-live="polite"></div>
    @if (candles.length <= 1) {
      <div class="chart-empty">Waiting for candle stream…</div>
    }
  `,
  styles: [
    `
      :host {
        display: flex;
        flex-direction: column;
        height: 100%;
        min-height: 0;
      }

      .chart-root {
        flex: 1;
        min-height: 0;
        width: 100%;
        position: relative;
      }

      .chart-toolbar {
        margin-bottom: 0.65rem;
        display: flex;
        gap: 0.45rem;
        flex-wrap: wrap;
      }

      .interval {
        border: 1px solid var(--fakemex-border);
        background: color-mix(in srgb, var(--fakemex-panel-header) 72%, var(--fakemex-chart-bg) 28%);
        border-radius: var(--fakemex-radius);
        color: var(--fakemex-muted);
        font-size: 0.73rem;
        padding: 0.25rem 0.65rem;
      }

      .interval.active,
      .interval:hover {
        color: var(--fakemex-text);
        background: color-mix(in srgb, var(--fakemex-accent) 35%, var(--fakemex-panel-header) 65%);
      }

      .toolbar-spacer {
        flex: 1;
      }

      .status-pill {
        border-radius: var(--fakemex-radius);
        padding: 0.25rem 0.65rem;
        font-size: 0.72rem;
        border: 1px solid var(--fakemex-border);
        color: var(--fakemex-muted);
      }

      .status-pill.offline {
        border-color: color-mix(in srgb, var(--fakemex-sell) 54%, var(--fakemex-border) 46%);
        color: var(--fakemex-sell);
      }

      .status-pill.demo {
        border-color: color-mix(in srgb, var(--fakemex-accent) 58%, var(--fakemex-border) 42%);
        color: var(--fakemex-accent);
      }

      .chart-empty {
        border-radius: var(--fakemex-radius);
        border: 1px dashed var(--fakemex-border);
        color: var(--fakemex-muted);
        display: flex;
        align-items: center;
        justify-content: center;
        min-height: 5rem;
        font-size: 0.78rem;
      }
    `,
  ],
})
export class MarketChartComponent implements AfterViewInit, OnChanges, OnDestroy {
  @Input() symbol = 'BTC';
  @Input() candles: Candle[] = [];
  @Input() selectedInterval = '15m';
  @Input() connectionStatus: 'Live' | 'Demo' | 'Offline' = 'Offline';
  @Output() intervalRequested = new EventEmitter<string>();

  readonly intervals = ['1m', '5m', '15m', '1h', '4h', '1d'];
  private chart?: IChartApi;
  private candleSeries?: ReturnType<IChartApi['addSeries']>;
  private resizeObserver?: ResizeObserver;
  private readonly theme = inject(ThemeService);
  private themeDisposer = effect(() => {
    void this.theme.activeTheme();
    this.ngZone.runOutsideAngular(() => {
      this.applyChartTheme();
      const points = this.currentDataPoints();
      if (!this.chart || !points.length) {
        return;
      }
      this.candleSeries?.setData(points);
      this.chart.timeScale().fitContent();
    });
  });
  private requestedRaf = 0;

  @ViewChild('chartRoot') chartRef?: ElementRef<HTMLDivElement>;

  constructor(private readonly ngZone: NgZone) {}

  statusTone() {
    if (this.connectionStatus === 'Demo') return 'demo';
    if (this.connectionStatus === 'Live') return 'live';
    return 'offline';
  }

  connectionStatusLabel() {
    return this.connectionStatus;
  }

  ngAfterViewInit() {
    this.ngZone.runOutsideAngular(() => {
      requestAnimationFrame(() => this.renderCandles());
      this.resizeObserver = new ResizeObserver(() => {
        requestAnimationFrame(() => this.onResize());
      });
      if (this.chartRef?.nativeElement) {
        this.resizeObserver.observe(this.chartRef.nativeElement);
      }
    });
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (!changes['candles']) return;
    this.ngZone.runOutsideAngular(() => this.renderCandles());
  }

  ngOnDestroy() {
    this.themeDisposer.destroy();
    this.resizeObserver?.disconnect();
    if (this.requestedRaf) {
      cancelAnimationFrame(this.requestedRaf);
    }
    this.chart?.remove();
  }

  private onResize() {
    this.renderCandles();
  }

  private renderCandles(attempt = 0) {
    if (!this.chartRef?.nativeElement) return;

    const root = this.chartRef.nativeElement;
    const width = root.clientWidth;
    const height = root.clientHeight;
    if (width < 20 || height < 20) {
      if (attempt >= 12) return;
      if (this.requestedRaf) cancelAnimationFrame(this.requestedRaf);
      this.requestedRaf = requestAnimationFrame(() => this.renderCandles(attempt + 1));
      return;
    }

    if (!this.ensureChart(width, height)) {
      return;
    }
    if (!this.candleSeries || !this.chart) return;

    const points = this.currentDataPoints();
    this.candleSeries.setData(points);
    if (points.length > 0) {
      this.chart.timeScale().fitContent();
    }
    this.applyChartTheme();
    this.requestedRaf = 0;
  }

  private currentDataPoints() {
    return this.candles
      .filter(
        (item) =>
          Number.isFinite(item.t) &&
          Number.isFinite(Number(item.o)) &&
          Number.isFinite(Number(item.h)) &&
          Number.isFinite(Number(item.l)) &&
          Number.isFinite(Number(item.c)),
      )
      .map<CandlestickData>((item) => ({
        time: (Math.floor(item.t / 1000) as UTCTimestamp) as Time,
        open: Number(item.o),
        high: Number(item.h),
        low: Number(item.l),
        close: Number(item.c),
      }))
      .slice(-250);
  }

  private ensureChart(width: number, height: number) {
    if (this.chart) {
      this.chart.resize(width, height);
      return true;
    }

    const root = this.chartRef?.nativeElement;
    if (!root) return false;
    const theme = this.readThemeVars();
    this.chart = createChart(root, {
      width,
      height,
      layout: {
        background: { color: theme.chartBg },
        textColor: theme.chartText,
      },
      grid: {
        vertLines: { color: theme.chartGrid },
        horzLines: { color: theme.chartGrid },
      },
      crosshair: {
        mode: CrosshairMode.Normal,
      },
      rightPriceScale: {
        borderColor: theme.chartAxis,
        autoScale: true,
      },
      timeScale: {
        borderColor: theme.chartAxis,
      },
    });

    this.candleSeries = this.chart.addSeries(CandlestickSeries, {
      upColor: theme.upCandle,
      downColor: theme.downCandle,
      wickUpColor: theme.upCandle,
      wickDownColor: theme.downCandle,
      borderVisible: false,
    });
    return true;
  }

  private applyChartTheme() {
    if (!this.chart) return;
    const theme = this.readThemeVars();
    this.chart.applyOptions({
      layout: {
        background: { color: theme.chartBg },
        textColor: theme.chartText,
      },
      grid: {
        vertLines: { color: theme.chartGrid },
        horzLines: { color: theme.chartGrid },
      },
      rightPriceScale: {
        borderColor: theme.chartAxis,
      },
      timeScale: {
        borderColor: theme.chartAxis,
      },
    });
    this.candleSeries?.applyOptions({
      upColor: theme.upCandle,
      downColor: theme.downCandle,
      wickUpColor: theme.upCandle,
      wickDownColor: theme.downCandle,
      borderVisible: false,
    });
  }

  private readThemeVars() {
    const style = getComputedStyle(document.documentElement);
    return {
      chartBg: style.getPropertyValue('--fakemex-chart-bg').trim() || '#0e1420',
      chartText: style.getPropertyValue('--fakemex-chart-text').trim() || '#b2bbd5',
      chartGrid: style.getPropertyValue('--fakemex-chart-grid').trim() || '#20304f',
      chartAxis: style.getPropertyValue('--fakemex-chart-axis').trim() || '#213255',
      upCandle: style.getPropertyValue('--fakemex-up-candle').trim() || '#31d0aa',
      downCandle: style.getPropertyValue('--fakemex-down-candle').trim() || '#f6465d',
    };
  }
}
