import { describe, expect, it, beforeEach } from 'vitest';
import { ThemeService } from './theme.service';
import type { KittyTheme } from './theme.service';

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

describe('ThemeService', () => {
  beforeEach(() => {
    ensureLocalStorage();
    localStorage.clear();
  });

  const create = () => {
    return new ThemeService();
  };

  it('loads a valid persisted theme', () => {
    localStorage.setItem('fakemex-kitty-theme', 'gruvbox-light');
    const service = create();
    expect(service.activeTheme().id).toBe('gruvbox-light');
    expect(service.isLightMode()).toBe(true);
  });

  it('falls back to the first light theme for legacy light migration', () => {
    localStorage.setItem('fakemex-theme', 'light');
    const service = create();
    expect(service.activeTheme().variant).toBe('light');
  });

  it('persists theme selection', () => {
    const service = create();
    service.setTheme('tokyo-night');
    expect(service.activeTheme().id).toBe('tokyo-night');
    expect(localStorage.getItem('fakemex-kitty-theme')).toBe('tokyo-night');
  });

  it('searches themes by label prefix', () => {
    const service = create();
    const matches = service.searchThemes('cat');
    expect(matches.map((theme) => theme.label)).toEqual(
      expect.arrayContaining(['Catppuccin Mocha', 'Catppuccin Latte']),
    );
  });

  it('resolves Catppuccin typo search via fuzzy match', () => {
    const service = create();
    const matches = service.searchThemes('catpuccino');
    expect(matches.map((theme) => theme.label)).toContain('Catppuccin Mocha');
    const applied = service.setThemeByLabel('catpuccino');
    expect(applied).toBe(true);
    expect(service.activeTheme().id).toBe('catppuccin-mocha');
    expect(service.activeTheme().label).toBe('Catppuccin Mocha');
    expect(localStorage.getItem('fakemex-kitty-theme')).toBe('catppuccin-mocha');
  });

  it('maps theme semantics with readable palette roles', () => {
    const service = create();

    const hexToRgb = (value: string) => {
      const hex = value.replace('#', '');
      const full = Number.parseInt(hex.length === 3 ? `${hex[0]}${hex[0]}${hex[1]}${hex[1]}${hex[2]}${hex[2]}` : hex, 16);
      return {
        r: (full >> 16) & 0xff,
        g: (full >> 8) & 0xff,
        b: full & 0xff,
      };
    };

    const channel = (value: number) => {
      const ratio = value / 255;
      return ratio <= 0.03928 ? ratio / 12.92 : Math.pow((ratio + 0.055) / 1.055, 2.4);
    };

    const luminance = (color: string) => {
      const rgb = hexToRgb(color);
      return 0.2126 * channel(rgb.r) + 0.7152 * channel(rgb.g) + 0.0722 * channel(rgb.b);
    };

    const contrast = (foreground: string, background: string) => {
      const light = Math.max(luminance(foreground), luminance(background));
      const dark = Math.min(luminance(foreground), luminance(background));
      return (light + 0.05) / (dark + 0.05);
    };

    const themeThemes: readonly KittyTheme[] = service.themes();
    for (const theme of themeThemes) {
      const map = service.buildThemeCssMap(theme);
      expect(map['--fakemex-shell-bg']).not.toBe(map['--fakemex-text']);
      expect(map['--fakemex-shell-surface']).not.toBe(map['--fakemex-text']);
      expect(map['--fakemex-panel-body']).not.toBe(map['--fakemex-shell-bg']);
      expect(map['--fakemex-text']).not.toBe(map['--fakemex-muted']);
      expect(map['--fakemex-input-text']).not.toBe(map['--fakemex-input-bg']);
      expect(map['--fakemex-chart-text']).not.toBe(map['--fakemex-chart-bg']);
      const textContrast = contrast(map['--fakemex-text'], map['--fakemex-shell-bg']);
      expect(textContrast).toBeGreaterThan(4);
      expect(contrast(map['--fakemex-text'], map['--fakemex-panel-body'])).toBeGreaterThan(4);
      expect(contrast(map['--fakemex-muted'], map['--fakemex-panel-body'])).toBeGreaterThan(2.6);
      expect(contrast(map['--fakemex-input-text'], map['--fakemex-input-bg'])).toBeGreaterThan(4);
    }
  });
});
