import { computed, Injectable, signal } from '@angular/core';

export type PanelId =
  | 'order-form'
  | 'chart'
  | 'depth'
  | 'book'
  | 'trades'
  | 'positions'
  | 'orders'
  | 'triggers'
  | 'fills'
  | 'history'
  | 'funding'
  | 'assets'
  | 'leverage'
  | 'settings';

export type PanelDensity = 'compact' | 'normal' | 'spacious';
export type LayoutPreset = 'basic' | 'advanced' | 'charting' | 'custom';

export interface GridLayoutItem {
  x: number;
  y: number;
  w: number;
  h: number;
  id: PanelId;
}

export const defaultPanels: PanelId[] = [
  'order-form',
  'chart',
  'depth',
  'book',
  'trades',
  'positions',
  'orders',
  'triggers',
  'fills',
  'history',
  'funding',
  'assets',
  'leverage',
  'settings',
];

const PRESET_LAYOUTS: Record<Exclude<LayoutPreset, 'custom'>, GridLayoutItem[]> = {
  basic: [
    { id: 'order-form', x: 0, y: 0, w: 3, h: 8 },
    { id: 'chart', x: 3, y: 0, w: 6, h: 12 },
    { id: 'book', x: 9, y: 0, w: 3, h: 7 },
    { id: 'depth', x: 9, y: 7, w: 3, h: 5 },
    { id: 'trades', x: 0, y: 8, w: 3, h: 6 },
    { id: 'positions', x: 0, y: 14, w: 5, h: 4 },
    { id: 'orders', x: 5, y: 14, w: 5, h: 4 },
    { id: 'assets', x: 10, y: 12, w: 2, h: 6 },
  ],
  advanced: [
    { id: 'chart', x: 0, y: 0, w: 8, h: 12 },
    { id: 'order-form', x: 8, y: 0, w: 4, h: 12 },
    { id: 'book', x: 0, y: 12, w: 4, h: 6 },
    { id: 'depth', x: 4, y: 12, w: 4, h: 6 },
    { id: 'trades', x: 8, y: 12, w: 4, h: 6 },
    { id: 'positions', x: 0, y: 18, w: 4, h: 6 },
    { id: 'orders', x: 4, y: 18, w: 4, h: 6 },
    { id: 'triggers', x: 8, y: 18, w: 4, h: 6 },
    { id: 'fills', x: 0, y: 24, w: 4, h: 6 },
    { id: 'history', x: 4, y: 24, w: 4, h: 6 },
    { id: 'funding', x: 8, y: 24, w: 4, h: 6 },
    { id: 'assets', x: 0, y: 30, w: 4, h: 6 },
    { id: 'leverage', x: 4, y: 30, w: 4, h: 3 },
    { id: 'settings', x: 8, y: 30, w: 4, h: 3 },
  ],
  charting: [
    { id: 'chart', x: 0, y: 0, w: 12, h: 15 },
    { id: 'depth', x: 0, y: 15, w: 6, h: 8 },
    { id: 'trades', x: 6, y: 15, w: 6, h: 8 },
    { id: 'order-form', x: 0, y: 23, w: 4, h: 8 },
    { id: 'book', x: 4, y: 23, w: 4, h: 8 },
    { id: 'positions', x: 8, y: 23, w: 2, h: 8 },
    { id: 'orders', x: 10, y: 23, w: 2, h: 8 },
  ],
};

@Injectable({ providedIn: 'root' })
export class LayoutService {
  readonly panelIds = defaultPanels;
  readonly preset = signal<LayoutPreset>('advanced');
  readonly density = signal<PanelDensity>('normal');
  readonly titleBarCollapsed = signal(false);
  readonly sidebarCompact = computed(() => this.density() === 'compact');

  private readonly customLayout = signal<GridLayoutItem[]>(PRESET_LAYOUTS.advanced);

  readonly activeLayout = computed<GridLayoutItem[]>(() => {
    if (this.preset() === 'custom') {
      return this.customLayout();
    }
    if (this.preset() === 'basic') return PRESET_LAYOUTS.basic;
    if (this.preset() === 'advanced') return PRESET_LAYOUTS.advanced;
    return PRESET_LAYOUTS.charting;
  });

  setPreset(preset: LayoutPreset) {
    this.preset.set(preset);
    if (preset !== 'custom') {
      this.customLayout.set(PRESET_LAYOUTS[preset]);
    }
    this.persist();
  }

  setDensity(density: PanelDensity) {
    this.density.set(density);
    this.persist();
  }

  setCustomLayout(layout: GridLayoutItem[]) {
    this.customLayout.set(layout);
    this.preset.set('custom');
    this.persist();
  }

  loadPersisted() {
    if (typeof localStorage === 'undefined') return;
    const persistedDensity = localStorage.getItem('fakemex-density');
    const persistedPreset = localStorage.getItem('fakemex-preset') as LayoutPreset | null;
    const persistedLayout = localStorage.getItem('fakemex-layout-custom');
    if (persistedDensity === 'compact' || persistedDensity === 'normal' || persistedDensity === 'spacious') {
      this.density.set(persistedDensity);
    }
    if (persistedPreset === 'basic' || persistedPreset === 'advanced' || persistedPreset === 'charting' || persistedPreset === 'custom') {
      this.preset.set(persistedPreset);
    }
    if (persistedLayout) {
      try {
        const parsed = JSON.parse(persistedLayout) as GridLayoutItem[];
        if (Array.isArray(parsed) && parsed.length > 0) {
          this.customLayout.set(parsed);
        }
      } catch {
        // ignore corrupted payload
      }
    }
  }

  private persist() {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem('fakemex-density', this.density());
    localStorage.setItem('fakemex-preset', this.preset());
    localStorage.setItem('fakemex-layout-custom', JSON.stringify(this.customLayout()));
  }

  ensurePersisted() {
    this.loadPersisted();
  }
}
