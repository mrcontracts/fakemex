import { describe, expect, it, beforeEach, vi } from 'vitest';
import { LayoutService } from './layout.service';

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

describe('LayoutService', () => {
  beforeEach(() => {
    ensureLocalStorage();
    localStorage.clear();
  });

  it('defaults to the advanced preset', () => {
    const service = new LayoutService();
    expect(service.preset()).toBe('advanced');
    expect(service.activeLayout().length).toBe(14);
    expect(service.activeLayout()[0]?.id).toBe('chart');
  });

  it('persists and restores custom layout state', () => {
    const service = new LayoutService();
    service.setCustomLayout([
      { id: 'chart', x: 0, y: 0, w: 12, h: 10 },
      { id: 'trades', x: 0, y: 10, w: 12, h: 6 },
    ]);

    expect(service.preset()).toBe('custom');
    expect(service.activeLayout()).toHaveLength(2);

    const restored = new LayoutService();
    restored.ensurePersisted();
    expect(restored.preset()).toBe('custom');
    expect(restored.activeLayout()).toEqual(service.activeLayout());
  });

  it('stores density in storage', () => {
    const service = new LayoutService();

    service.setDensity('compact');

    expect(service.density()).toBe('compact');
    expect(localStorage.getItem('fakemex-density')).toBe('compact');
  });
});
