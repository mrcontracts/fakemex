import {
  AfterViewInit,
  ChangeDetectionStrategy,
  Component,
  Input,
  ElementRef,
  effect,
  NgZone,
  OnChanges,
  OnDestroy,
  SimpleChanges,
  inject,
  ViewChild,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { createChart, IChartApi, LineData, LineSeries, LineSeriesOptions, UTCTimestamp } from 'lightweight-charts';
import { BookLevel } from '../models';
import { ThemeService } from '../services/theme.service';

@Component({
  selector: 'app-depth-chart',
  standalone: true,
  imports: [CommonModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="depth-root" #chartRoot></div>
    @if (book.length <= 1) {
      <div class="depth-empty">Waiting for depth snapshot…</div>
    }
  `,
  styles: [
    `
      :host {
        display: block;
        height: 100%;
        min-height: 0;
      }

      .depth-root {
        width: 100%;
        height: 100%;
        min-height: 0;
        position: relative;
      }

      .depth-empty {
        border-radius: var(--fakemex-radius);
        border: 1px dashed var(--fakemex-border);
        color: var(--fakemex-muted);
        display: flex;
        align-items: center;
        justify-content: center;
        min-height: 4rem;
        font-size: 0.75rem;
      }
    `,
  ],
})
export class DepthChartComponent implements AfterViewInit, OnChanges, OnDestroy {
  @Input() book: BookLevel[] = [];

  @ViewChild('chartRoot') chartRef?: ElementRef<HTMLDivElement>;
  private chart?: IChartApi;
  private series?: ReturnType<IChartApi['addSeries']>;
  private resizeObserver?: ResizeObserver;
  private retryHandle = 0;
  private readonly theme = inject(ThemeService);
  private themeDisposer = effect(() => {
    void this.theme.activeTheme();
    this.zone.runOutsideAngular(() => this.applyTheme());
  });

  constructor(private readonly zone: NgZone) {}

  ngAfterViewInit() {
    this.zone.runOutsideAngular(() => {
      requestAnimationFrame(() => this.renderDepth());
      this.resizeObserver = new ResizeObserver(() => requestAnimationFrame(() => this.renderDepth()));
      if (this.chartRef?.nativeElement) {
        this.resizeObserver.observe(this.chartRef.nativeElement);
      }
    });
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (!changes['book']) return;
    this.zone.runOutsideAngular(() => this.renderDepth());
  }

  ngOnDestroy() {
    this.themeDisposer.destroy();
    this.resizeObserver?.disconnect();
    if (this.retryHandle) cancelAnimationFrame(this.retryHandle);
    this.chart?.remove();
  }

  private renderDepth(attempt = 0) {
    if (!this.chartRef?.nativeElement) return;
    const root = this.chartRef.nativeElement;
    const width = root.clientWidth;
    const height = root.clientHeight;
    if (width < 20 || height < 20) {
      if (attempt >= 12) return;
      if (this.retryHandle) cancelAnimationFrame(this.retryHandle);
      this.retryHandle = requestAnimationFrame(() => this.renderDepth(attempt + 1));
      return;
    }

    if (!this.ensureChart(width, height)) return;
    if (!this.series) return;

    const data = this.buildSeriesData();
    this.series.setData(data);
    this.applyTheme();
    this.retryHandle = 0;
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
      rightPriceScale: { borderColor: theme.chartAxis },
      timeScale: { visible: false },
      grid: {
        horzLines: { color: theme.chartGrid },
        vertLines: { color: theme.chartGrid },
      },
    });
    this.series = this.chart.addSeries(LineSeries, {
      color: theme.fg,
      lineWidth: 2,
    } as LineSeriesOptions);
    return true;
  }

  private applyTheme() {
    if (!this.chart || !this.series) return;
    const theme = this.readThemeVars();
    this.chart.applyOptions({
      layout: {
        background: { color: theme.chartBg },
        textColor: theme.chartText,
      },
      rightPriceScale: { borderColor: theme.chartAxis },
      grid: {
        horzLines: { color: theme.chartGrid },
        vertLines: { color: theme.chartGrid },
      },
      timeScale: {
        visible: false,
      },
    });
    this.series.applyOptions({
      color: theme.fg,
      lineWidth: 2,
    });
  }

  private buildSeriesData() {
    return this.book
      .filter((entry) => entry.price !== undefined && entry.total !== undefined)
      .map((entry, index): LineData | null => {
        const value = Number(String(entry.total).replace(/,/g, ''));
        if (!Number.isFinite(value)) return null;
        return {
          time: (Math.floor(Date.now() / 1000) - (this.book.length - index) * 20) as UTCTimestamp,
          value,
        };
      })
      .filter((point): point is LineData => point !== null);
  }

  private readThemeVars() {
    const style = getComputedStyle(document.documentElement);
    return {
      chartBg: style.getPropertyValue('--fakemex-chart-bg').trim() || '#0e1420',
      chartText: style.getPropertyValue('--fakemex-chart-text').trim() || '#aeb8d2',
      chartGrid: style.getPropertyValue('--fakemex-chart-grid').trim() || '#20304f',
      chartAxis: style.getPropertyValue('--fakemex-chart-axis').trim() || '#213255',
      fg: style.getPropertyValue('--fakemex-accent').trim() || '#6f9ef8',
    };
  }
}
