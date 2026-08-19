import { computed, Injectable, Signal, WritableSignal, signal } from '@angular/core';

export interface KittyColorRoles {
  background: string;
  foreground: string;
  selectionBackground: string;
  selectionForeground: string;
  cursor: string;
  cursorTextColor: string;
  color0: string;
  color1: string;
  color2: string;
  color3: string;
  color4: string;
  color5: string;
  color6: string;
  color7: string;
  color8: string;
  color9: string;
  color10: string;
  color11: string;
  color12: string;
  color13: string;
  color14: string;
  color15: string;
}

export type ThemeMode = 'dark' | 'light';

export interface KittyTheme {
  id: string;
  label: string;
  variant: ThemeMode;
  source: string;
  roles: KittyColorRoles;
}

export interface ThemeCssVarMap {
  [key: string]: string;
}

const LOCAL_STORAGE_THEME_KEY = 'fakemex-kitty-theme';
const LOCAL_STORAGE_OLD_THEME_KEY = 'fakemex-theme';

const THEME_SOURCE = 'https://github.com/kovidgoyal/kitty-themes';
const FUZZY_SEARCH_MAX_DISTANCE = 3;
const MIN_FUZZY_SEARCH_QUERY_LENGTH = 6;

function normalizeThemeQuery(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9]/g, '');
}

function levenshteinDistance(a: string, b: string, limit = FUZZY_SEARCH_MAX_DISTANCE + 1) {
  const source = normalizeThemeQuery(a);
  const target = normalizeThemeQuery(b);
  const lenA = source.length;
  const lenB = target.length;
  if (lenA === 0) return lenB;
  if (lenB === 0) return lenA;
  if (Math.abs(lenA - lenB) > limit) return limit + 1;

  const row: number[] = Array.from({ length: lenB + 1 }, (_, column) => column);

  for (let rowIndex = 1; rowIndex <= lenA; rowIndex++) {
    let previous = rowIndex - 1;
    row[0] = rowIndex;
    let rowMin = Number.MAX_SAFE_INTEGER;
    const rowChar = source[rowIndex - 1];

    for (let column = 1; column <= lenB; column++) {
      const temp = row[column];
      const cost = rowChar === target[column - 1] ? 0 : 1;
      const insertion = row[column - 1] + 1;
      const deletion = row[column] + 1;
      const substitution = previous + cost;
      row[column] = Math.min(insertion, deletion, substitution);
      rowMin = Math.min(rowMin, row[column]);
      previous = temp;
    }

    if (rowMin > limit) {
      return limit + 1;
    }
  }

  return row[lenB];
}

function matchesThemeQuery(theme: KittyTheme, query: string) {
  const normalizedLabel = normalizeThemeQuery(theme.label);
  const normalizedId = normalizeThemeQuery(theme.id);
  const normalizedQuery = normalizeThemeQuery(query);
  if (!normalizedQuery) return true;

  const queryTokens = [
    normalizedLabel,
    normalizedId,
    ...theme.label
      .toLowerCase()
      .split(/[^a-z0-9]+/)
      .filter(Boolean)
      .map((token) => normalizeThemeQuery(token)),
    ...theme.id.split('-'),
  ];

  for (const token of queryTokens) {
    if (!token) continue;
    const normalizedToken = normalizeThemeQuery(token);
    if (normalizedToken.includes(normalizedQuery)) return true;
    if (normalizedQuery.includes(normalizedToken)) return true;

    if (
      normalizedQuery.length >= MIN_FUZZY_SEARCH_QUERY_LENGTH &&
      normalizedToken.length >= MIN_FUZZY_SEARCH_QUERY_LENGTH &&
      levenshteinDistance(normalizedToken, normalizedQuery) <= FUZZY_SEARCH_MAX_DISTANCE
    ) {
      return true;
    }
  }
  return false;
}

function themeSearchTokens(theme: KittyTheme) {
  const labelTokens = theme.label
    .toLowerCase()
    .split(/[^a-z0-9]+/)
    .filter(Boolean)
    .map(normalizeThemeQuery);
  return [normalizeThemeQuery(theme.label), normalizeThemeQuery(theme.id), ...labelTokens, ...theme.id.split('-').map(normalizeThemeQuery)];
}

function themeSearchRank(theme: KittyTheme, query: string) {
  const normalizedQuery = normalizeThemeQuery(query);
  if (!normalizedQuery) return 0;
  const tokens = themeSearchTokens(theme);
  let best = Number.POSITIVE_INFINITY;
  for (const token of tokens) {
    if (!token) continue;
    if (token.includes(normalizedQuery) || normalizedQuery.includes(token)) {
      return 0;
    }
    if (
      normalizedQuery.length >= MIN_FUZZY_SEARCH_QUERY_LENGTH &&
      token.length >= MIN_FUZZY_SEARCH_QUERY_LENGTH
    ) {
      const distance = levenshteinDistance(token, normalizedQuery);
      best = Math.min(best, distance);
    }
  }
  return best;
}

const THEME_DEFINITIONS: readonly KittyTheme[] = [
  {
    id: 'default',
    label: 'Default',
    variant: 'dark',
    source: THEME_SOURCE,
    roles: {
      background: '#000000',
      foreground: '#f8f8f2',
      selectionBackground: '#44475a',
      selectionForeground: '#f8f8f2',
      cursor: '#f8f8f2',
      cursorTextColor: '#000000',
      color0: '#21222c',
      color1: '#ff5555',
      color2: '#50fa7b',
      color3: '#f1fa8c',
      color4: '#bd93f9',
      color5: '#ff79c6',
      color6: '#8be9fd',
      color7: '#f8f8f2',
      color8: '#6272a4',
      color9: '#ff6e6e',
      color10: '#69ff94',
      color11: '#ffffa5',
      color12: '#d6acff',
      color13: '#ff92df',
      color14: '#a4ffff',
      color15: '#ffffff',
    },
  },
  {
    id: 'dracula',
    label: 'Dracula',
    variant: 'dark',
    source: THEME_SOURCE,
    roles: {
      background: '#282a36',
      foreground: '#f8f8f2',
      selectionBackground: '#44475a',
      selectionForeground: '#ffffff',
      cursor: '#f8f8f2',
      cursorTextColor: '#282a36',
      color0: '#21222c',
      color1: '#ff5555',
      color2: '#50fa7b',
      color3: '#f1fa8c',
      color4: '#bd93f9',
      color5: '#ff79c6',
      color6: '#8be9fd',
      color7: '#f8f8f2',
      color8: '#6272a4',
      color9: '#ff6e6e',
      color10: '#69ff94',
      color11: '#ffffa5',
      color12: '#d6acff',
      color13: '#ff92df',
      color14: '#a4ffff',
      color15: '#ffffff',
    },
  },
  {
    id: 'nord',
    label: 'Nord',
    variant: 'dark',
    source: THEME_SOURCE,
    roles: {
      background: '#2e3440',
      foreground: '#d8dee9',
      selectionBackground: '#434c5e',
      selectionForeground: '#d8dee9',
      cursor: '#d8dee9',
      cursorTextColor: '#3b4252',
      color0: '#3b4252',
      color1: '#bf616a',
      color2: '#a3be8c',
      color3: '#ebcb8b',
      color4: '#81a1c1',
      color5: '#b48ead',
      color6: '#88c0d0',
      color7: '#e5e9f0',
      color8: '#4c566a',
      color9: '#bf616a',
      color10: '#a3be8c',
      color11: '#d08770',
      color12: '#5e81ac',
      color13: '#b48ead',
      color14: '#8fbcbb',
      color15: '#eceff4',
    },
  },
  {
    id: 'gruvbox-dark',
    label: 'Gruvbox Dark',
    variant: 'dark',
    source: THEME_SOURCE,
    roles: {
      background: '#282828',
      foreground: '#ebdbb2',
      selectionBackground: '#d65d0e',
      selectionForeground: '#ebdbb2',
      cursor: '#bdae93',
      cursorTextColor: '#665c54',
      color0: '#3c3836',
      color1: '#cc241d',
      color2: '#98971a',
      color3: '#d79921',
      color4: '#458588',
      color5: '#b16286',
      color6: '#689d6a',
      color7: '#a89984',
      color8: '#928374',
      color9: '#fb4934',
      color10: '#b8bb26',
      color11: '#fabd2f',
      color12: '#83a598',
      color13: '#d3869b',
      color14: '#8ec07c',
      color15: '#fbf1c7',
    },
  },
  {
    id: 'tokyo-night',
    label: 'Tokyo Night',
    variant: 'dark',
    source: THEME_SOURCE,
    roles: {
      background: '#1a1b26',
      foreground: '#c0caf5',
      selectionBackground: '#283457',
      selectionForeground: '#c0caf5',
      cursor: '#c0caf5',
      cursorTextColor: '#1a1b26',
      color0: '#15161e',
      color1: '#f7768e',
      color2: '#9ece6a',
      color3: '#e0af68',
      color4: '#7aa2f7',
      color5: '#bb9af7',
      color6: '#7dcfff',
      color7: '#a9b1d6',
      color8: '#414868',
      color9: '#ff899d',
      color10: '#9fe044',
      color11: '#faba4a',
      color12: '#8db0ff',
      color13: '#c7a9ff',
      color14: '#a4daff',
      color15: '#c0caf5',
    },
  },
  {
    id: 'catppuccin-mocha',
    label: 'Catppuccin Mocha',
    variant: 'dark',
    source: THEME_SOURCE,
    roles: {
      background: '#1e1e2e',
      foreground: '#cdd6f4',
      selectionBackground: '#f5e0dc',
      selectionForeground: '#1e1e2e',
      cursor: '#f5e0dc',
      cursorTextColor: '#1e1e2e',
      color0: '#45475a',
      color1: '#f38ba8',
      color2: '#a6e3a1',
      color3: '#f9e2af',
      color4: '#89b4fa',
      color5: '#f5c2e7',
      color6: '#94e2d5',
      color7: '#bac2de',
      color8: '#585b70',
      color9: '#f38ba8',
      color10: '#a6e3a1',
      color11: '#f9e2af',
      color12: '#89b4fa',
      color13: '#f5c2e7',
      color14: '#94e2d5',
      color15: '#a6adc8',
    },
  },
  {
    id: 'solarized-dark',
    label: 'Solarized Dark',
    variant: 'dark',
    source: THEME_SOURCE,
    roles: {
      background: '#002b36',
      foreground: '#839496',
      selectionBackground: '#073642',
      selectionForeground: '#93a1a1',
      cursor: '#708183',
      cursorTextColor: '#002b36',
      color0: '#002731',
      color1: '#d01b24',
      color2: '#728905',
      color3: '#a57705',
      color4: '#2075c7',
      color5: '#c61b6e',
      color6: '#259185',
      color7: '#e9e2cb',
      color8: '#001e26',
      color9: '#bd3612',
      color10: '#465a61',
      color11: '#52676f',
      color12: '#708183',
      color13: '#5856b9',
      color14: '#81908f',
      color15: '#fcf4dc',
    },
  },
  {
    id: 'solarized-light',
    label: 'Solarized Light',
    variant: 'light',
    source: THEME_SOURCE,
    roles: {
      background: '#fdf6e3',
      foreground: '#657b83',
      selectionBackground: '#eee8d5',
      selectionForeground: '#586e75',
      cursor: '#657b83',
      cursorTextColor: '#fdf6e3',
      color0: '#073642',
      color1: '#dc322f',
      color2: '#859900',
      color3: '#b58900',
      color4: '#268bd2',
      color5: '#d33682',
      color6: '#2aa198',
      color7: '#eee8d5',
      color8: '#93a1a1',
      color9: '#cb4b16',
      color10: '#586e75',
      color11: '#657b83',
      color12: '#839496',
      color13: '#6c71c4',
      color14: '#93a1a1',
      color15: '#fdf6e3',
    },
  },
  {
    id: 'catppuccin-latte',
    label: 'Catppuccin Latte',
    variant: 'light',
    source: THEME_SOURCE,
    roles: {
      background: '#eff1f5',
      foreground: '#4c4f69',
      selectionBackground: '#dc8a78',
      selectionForeground: '#eff1f5',
      cursor: '#dc8a78',
      cursorTextColor: '#eff1f5',
      color0: '#5c5f77',
      color1: '#d20f39',
      color2: '#40a02b',
      color3: '#df8e1d',
      color4: '#1e66f5',
      color5: '#ea76cb',
      color6: '#179299',
      color7: '#acb0be',
      color8: '#6c6f85',
      color9: '#d20f39',
      color10: '#40a02b',
      color11: '#df8e1d',
      color12: '#1e66f5',
      color13: '#ea76cb',
      color14: '#179299',
      color15: '#bcc0cc',
    },
  },
  {
    id: 'gruvbox-light',
    label: 'Gruvbox Light',
    variant: 'light',
    source: THEME_SOURCE,
    roles: {
      background: '#fbf1c7',
      foreground: '#3c3836',
      selectionBackground: '#d65d0e',
      selectionForeground: '#3c3836',
      cursor: '#665c54',
      cursorTextColor: '#665c54',
      color0: '#ebdbb2',
      color1: '#cc241d',
      color2: '#98971a',
      color3: '#d79921',
      color4: '#458588',
      color5: '#b16286',
      color6: '#689d6a',
      color7: '#7c6f64',
      color8: '#928374',
      color9: '#9d0006',
      color10: '#79740e',
      color11: '#b57614',
      color12: '#076678',
      color13: '#8f3f71',
      color14: '#427b58',
      color15: '#282828',
    },
  },
  {
    id: 'github-light',
    label: 'GitHub Light',
    variant: 'light',
    source: THEME_SOURCE,
    roles: {
      background: '#ffffff',
      foreground: '#24292f',
      selectionBackground: '#0969da',
      selectionForeground: '#ffffff',
      cursor: '#0969da',
      cursorTextColor: '#ffffff',
      color0: '#24292f',
      color1: '#cf222e',
      color2: '#116329',
      color3: '#4d2d00',
      color4: '#0969da',
      color5: '#8250df',
      color6: '#1b7c83',
      color7: '#6e7781',
      color8: '#57606a',
      color9: '#a40e26',
      color10: '#1a7f37',
      color11: '#633c01',
      color12: '#218bff',
      color13: '#a475f9',
      color14: '#3192aa',
      color15: '#8c959f',
    },
  },
];

type ThemeColorMap = {
  '--fakemex-radius': string;
  '--fakemex-shell-bg': string;
  '--fakemex-shell-surface': string;
  '--fakemex-shell-surface-alt': string;
  '--fakemex-panel-bg': string;
  '--fakemex-panel-header': string;
  '--fakemex-panel-body': string;
  '--fakemex-border': string;
  '--fakemex-text': string;
  '--fakemex-muted': string;
  '--fakemex-accent': string;
  '--fakemex-buy': string;
  '--fakemex-sell': string;
  '--fakemex-warning': string;
  '--fakemex-chart-bg': string;
  '--fakemex-chart-grid': string;
  '--fakemex-chart-text': string;
  '--fakemex-chart-axis': string;
  '--fakemex-up-candle': string;
  '--fakemex-down-candle': string;
  '--fakemex-input-bg': string;
  '--fakemex-input-text': string;
  '--fakemex-input-border': string;
};

function parseHexColor(value: string): { r: number; g: number; b: number } {
  const normalized = value.trim().replace('#', '').toLowerCase();
  if (!/^[0-9a-f]{3}$|^[0-9a-f]{6}$/.test(normalized)) {
    return { r: 0, g: 0, b: 0 };
  }
  const hex = normalized.length === 3
    ? normalized.split('').flatMap((char) => [char, char]).join('')
    : normalized;
  const full = Number.parseInt(hex, 16);
  return {
    r: (full >> 16) & 255,
    g: (full >> 8) & 255,
    b: full & 255,
  };
}

function rgbToHex(r: number, g: number, b: number): string {
  const toHex = (value: number) => Math.max(0, Math.min(255, Math.round(value))).toString(16).padStart(2, '0');
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`;
}

function blendColor(base: string, target: string, amount: number): string {
  const source = parseHexColor(base);
  const mix = parseHexColor(target);
  const t = Math.max(0, Math.min(1, amount));
  return rgbToHex(
    source.r + (mix.r - source.r) * t,
    source.g + (mix.g - source.g) * t,
    source.b + (mix.b - source.b) * t,
  );
}

function shiftColor(color: string, amount: number): string {
  if (amount >= 0) {
    return blendColor(color, '#ffffff', amount);
  }
  return blendColor(color, '#000000', -amount);
}

function srgbToLinear(value: number): number {
  const v = value / 255;
  return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
}

function luminance(color: string): number {
  const rgb = parseHexColor(color);
  const r = srgbToLinear(rgb.r);
  const g = srgbToLinear(rgb.g);
  const b = srgbToLinear(rgb.b);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrastRatio(foreground: string, background: string): number {
  const l1 = luminance(foreground);
  const l2 = luminance(background);
  const lighter = Math.max(l1, l2);
  const darker = Math.min(l1, l2);
  return (lighter + 0.05) / (darker + 0.05);
}

function ensureContrast(
  color: string,
  against: string,
  target = 4.5,
  variant: ThemeMode,
): string {
  let candidate = color;
  const direction = variant === 'light' ? -0.06 : 0.08;
  for (let step = 0; step < 24; step += 1) {
    if (contrastRatio(candidate, against) >= target) {
      return candidate;
    }
    candidate = shiftColor(candidate, direction);
  }
  return contrastRatio(color, against) >= target
    ? color
    : contrastRatio('#000000', against) >= contrastRatio('#ffffff', against)
      ? '#000000'
      : '#ffffff';
}

function ensureContrastAgainstBoth(
  color: string,
  bgA: string,
  bgB: string,
  target = 4.5,
  variant: ThemeMode,
): string {
  let candidate = color;
  const direction = variant === 'light' ? -0.07 : 0.07;
  for (let step = 0; step < 30; step += 1) {
    if (contrastRatio(candidate, bgA) >= target && contrastRatio(candidate, bgB) >= target) {
      return candidate;
    }
    candidate = shiftColor(candidate, direction);
  }
  const options = ['#000000', '#ffffff', color] as const;
  const readable = options
    .filter((value) => contrastRatio(value, bgA) >= target && contrastRatio(value, bgB) >= target)
    .map((value) => ({
      value,
      score: contrastRatio(value, bgA) + contrastRatio(value, bgB),
    }));
  if (readable.length > 0) {
    readable.sort((a, b) => b.score - a.score);
    return readable[0].value;
  }
  return color;
}

function buildThemeMap(theme: KittyTheme): ThemeColorMap {
  const radius = '8px';
  const { roles, variant } = theme;
  const isLight = variant === 'light';

  const shellBg = roles.background;
  const panelBg = isLight ? shiftColor(shellBg, -0.08) : shiftColor(shellBg, 0.06);
  const shellSurface = isLight ? shiftColor(shellBg, -0.12) : shiftColor(panelBg, -0.02);
  const shellSurfaceAlt = isLight ? shiftColor(shellBg, -0.18) : shiftColor(panelBg, -0.04);
  const panelHeader = isLight ? shiftColor(panelBg, -0.1) : shiftColor(panelBg, 0.05);
  const panelBody = isLight ? shiftColor(panelBg, -0.02) : shiftColor(panelBg, 0.05);

  const text = ensureContrastAgainstBoth(roles.foreground, shellBg, panelBody, 4.8, variant);
  const muted = ensureContrastAgainstBoth(
    shiftColor(text, isLight ? -0.17 : 0.14),
    shellBg,
    panelBody,
    3.3,
    variant,
  );
  const accent = ensureContrastAgainstBoth(roles.color4, panelBody, shellBg, 3.2, variant);
  const buy = ensureContrastAgainstBoth(roles.color2, panelBody, shellBg, 2.95, variant);
  const sell = ensureContrastAgainstBoth(roles.color1, panelBody, shellBg, 2.95, variant);
  const warning = ensureContrastAgainstBoth(roles.color3, panelBody, shellBg, 2.85, variant);

  const chartBg = isLight ? shiftColor(panelBg, -0.04) : shiftColor(panelBg, -0.02);
  const chartAxis = isLight ? shiftColor(chartBg, -0.2) : shiftColor(chartBg, 0.14);
  const chartGrid = isLight ? shiftColor(chartAxis, 0.04) : shiftColor(chartAxis, -0.18);
  const chartText = ensureContrast(roles.foreground, chartBg, isLight ? 4.5 : 4.2, variant);
  const inputBg = isLight
    ? '#ffffff'
    : shiftColor(panelBg, 0.06);
  const inputText = ensureContrastAgainstBoth(text, inputBg, panelBody, 4.3, variant);
  const inputBorder = ensureContrast(shiftColor(roles.color8, isLight ? -0.2 : -0.05), panelBody, 3.2, variant);

  return {
    '--fakemex-radius': radius,
    '--fakemex-shell-bg': shellBg,
    '--fakemex-shell-surface': shellSurface,
    '--fakemex-shell-surface-alt': shellSurfaceAlt,
    '--fakemex-panel-bg': panelBg,
    '--fakemex-panel-header': panelHeader,
    '--fakemex-panel-body': panelBody,
    '--fakemex-border': ensureContrast(inputBorder, panelBody, 2.9, variant),
    '--fakemex-text': text,
    '--fakemex-muted': muted,
    '--fakemex-accent': accent,
    '--fakemex-buy': buy,
    '--fakemex-sell': sell,
    '--fakemex-warning': warning,
    '--fakemex-chart-bg': chartBg,
    '--fakemex-chart-grid': chartGrid,
    '--fakemex-chart-text': chartText,
    '--fakemex-chart-axis': chartAxis,
    '--fakemex-up-candle': buy,
    '--fakemex-down-candle': sell,
    '--fakemex-input-bg': inputBg,
    '--fakemex-input-text': inputText,
    '--fakemex-input-border': inputBorder,
  };
}

function computeThemeMap(theme: KittyTheme): ThemeColorMap {
  return buildThemeMap(theme);
}

@Injectable({
  providedIn: 'root',
})
export class ThemeService {
  readonly themes: Signal<readonly KittyTheme[]> = signal(THEME_DEFINITIONS);
  readonly activeThemeId: WritableSignal<string> = signal(THEME_DEFINITIONS[0]?.id ?? 'default');
  readonly searchQuery = signal('');
  private readonly isBrowser = typeof window !== 'undefined' && typeof document !== 'undefined';

  readonly activeTheme = computed(() => {
    const themeId = this.activeThemeId();
    return this.findThemeById(themeId) ?? THEME_DEFINITIONS[0];
  });

  readonly filteredThemes = computed(() => {
    const query = this.searchQuery().trim().toLowerCase();
    if (!query) return this.themes();
    return this.themes().filter((theme) => matchesThemeQuery(theme, query));
  });

  readonly isLightMode = computed(() => this.activeTheme().variant === 'light');

  constructor() {
    this.loadPersistedTheme();
    this.applyTheme(this.activeTheme());
  }

  setTheme(themeId: string) {
    const match = this.findThemeById(themeId);
    if (!match) return;
    this.activeThemeId.set(themeId);
    this.applyTheme(match);
    if (this.isBrowser) {
      localStorage.setItem(LOCAL_STORAGE_THEME_KEY, themeId);
      localStorage.setItem(LOCAL_STORAGE_OLD_THEME_KEY, match.variant);
    }
  }

  setThemeByLabel(label: string): boolean {
    const match = this.findThemeByLabel(label) ?? this.findThemeBySearch(label);
    if (!match) return false;
    this.setTheme(match.id);
    this.searchQuery.set('');
    return true;
  }

  setSearchQuery(query: string) {
    this.searchQuery.set(query);
  }

  searchThemes(query: string): readonly KittyTheme[] {
    const normalized = query.trim().toLowerCase();
    if (!normalized) return this.themes();
    return this.themes().filter((theme) => matchesThemeQuery(theme, normalized));
  }

  getThemesByVariant(variant: ThemeMode): readonly KittyTheme[] {
    return this.themes().filter((theme) => theme.variant === variant);
  }

  buildThemeCssMap(theme: KittyTheme): ThemeCssVarMap {
    return {
      ...computeThemeMap(theme),
    };
  }

  private findThemeById(themeId: string): KittyTheme | undefined {
    const normalized = themeId.trim().toLowerCase();
    return this.themes().find((theme) => theme.id === normalized);
  }

  private findThemeByLabel(label: string): KittyTheme | undefined {
    const normalized = label.trim().toLowerCase();
    return this.themes().find((theme) => theme.label.toLowerCase() === normalized);
  }

  private findThemeBySearch(query: string): KittyTheme | undefined {
    const normalized = query.trim().toLowerCase();
    if (normalized.length < MIN_FUZZY_SEARCH_QUERY_LENGTH) return undefined;

    let bestTheme: KittyTheme | undefined;
    let bestScore = Number.POSITIVE_INFINITY;

    for (const theme of this.themes()) {
      const score = themeSearchRank(theme, normalized);
      if (score === 0) {
        return theme;
      }
      if (score < bestScore) {
        bestScore = score;
        bestTheme = theme;
      }
    }
    return bestScore <= FUZZY_SEARCH_MAX_DISTANCE ? bestTheme : undefined;
  }

  private loadPersistedTheme() {
    if (!this.isBrowser) return;

    const persistedTheme = localStorage.getItem(LOCAL_STORAGE_THEME_KEY);
    if (persistedTheme && this.findThemeById(persistedTheme)) {
      this.activeThemeId.set(persistedTheme);
      return;
    }

    const legacyTheme = localStorage.getItem(LOCAL_STORAGE_OLD_THEME_KEY);
    if (legacyTheme === 'light') {
      const firstLight = this.themes().find((theme) => theme.variant === 'light');
      if (firstLight) {
        this.activeThemeId.set(firstLight.id);
      }
      return;
    }

    if (legacyTheme === 'dark') {
      const firstDark = this.themes().find((theme) => theme.variant === 'dark');
      if (firstDark) {
        this.activeThemeId.set(firstDark.id);
      }
    }
  }

  private applyTheme(theme: KittyTheme) {
    if (!this.isBrowser) return;

    const root = document.documentElement;
    const tokens = this.buildThemeCssMap(theme);
    const isLight = theme.variant === 'light';
    for (const [key, value] of Object.entries(tokens)) {
      root.style.setProperty(key, value);
    }
    const surfaceContrast = tokens['--fakemex-shell-surface'];
    const panelSurface = tokens['--fakemex-panel-body'];
    const border = tokens['--fakemex-border'];
    const accent = tokens['--fakemex-accent'];
    const text = tokens['--fakemex-text'];
    const muted = tokens['--fakemex-muted'];
    const inputBg = tokens['--fakemex-input-bg'];
    const inputText = tokens['--fakemex-input-text'];
    const inputBorder = tokens['--fakemex-input-border'];
    const shellBg = tokens['--fakemex-shell-bg'];
    const buy = tokens['--fakemex-buy'];
    const sell = tokens['--fakemex-sell'];
    const warning = tokens['--fakemex-warning'];
    const chartBg = tokens['--fakemex-chart-bg'];
    const chartAxis = tokens['--fakemex-chart-axis'];
    const chartGrid = tokens['--fakemex-chart-grid'];
    const chartText = tokens['--fakemex-chart-text'];

    const materialOverrides: Array<[string, string]> = [
      ['--mat-app-background-color', surfaceContrast],
      ['--mat-option-label-text-color', text],
      ['--mat-menu-container-color', panelSurface],
      ['--mat-menu-item-label-text-color', text],
      ['--mat-tree-text-color', text],
      ['--mat-tooltip-text-color', text],
      ['--mat-sys-primary', accent],
      ['--mat-sys-on-primary', isLight ? '#ffffff' : panelSurface],
      ['--mat-sys-primary-container', panelSurface],
      ['--mat-sys-on-primary-container', text],
      ['--mat-sys-secondary', buy],
      ['--mat-sys-on-secondary', panelSurface],
      ['--mat-sys-secondary-container', panelSurface],
      ['--mat-sys-on-secondary-container', text],
      ['--mat-sys-surface', surfaceContrast],
      ['--mat-sys-surface-variant', panelSurface],
      ['--mat-sys-surface-container-low', surfaceContrast],
      ['--mat-sys-surface-container', panelSurface],
      ['--mat-sys-surface-container-high', panelSurface],
      ['--mat-sys-surface-container-highest', panelSurface],
      ['--mat-sys-surface-bright', panelSurface],
      ['--mat-sys-surface-dim', shellBg],
      ['--mat-sys-outline', border],
      ['--mat-sys-outline-variant', border],
      ['--mat-sys-on-surface', text],
      ['--mat-sys-on-surface-variant', muted],
      ['--mat-sys-error', warning],
      ['--mat-sys-on-error', panelSurface],
      ['--mat-sys-inverse-surface', chartBg],
      ['--mat-sys-inverse-on-surface', chartText],
      ['--mat-sys-tertiary', warning],
      ['--mat-sys-on-tertiary', panelSurface],
      ['--mat-form-field-filled-container-color', inputBg],
      ['--mat-form-field-filled-label-text-color', muted],
      ['--mat-form-field-filled-input-text-color', inputText],
      ['--mat-form-field-outlined-label-text-color', muted],
      ['--mat-form-field-outlined-outline-color', inputBorder],
      ['--mat-form-field-outlined-input-text-color', inputText],
      ['--mat-form-field-outlined-input-text-placeholder-color', muted],
      ['--mat-form-field-outlined-hover-label-text-color', text],
      ['--mat-form-field-outlined-hover-outline-color', accent],
      ['--mat-form-field-outlined-focus-label-text-color', accent],
      ['--mat-form-field-outlined-focus-outline-color', accent],
      ['--mat-form-field-outlined-caret-color', inputText],
      ['--mat-form-field-container-shape', 'var(--fakemex-radius)'],
      ['--mat-form-field-filled-container-shape', 'var(--fakemex-radius)'],
      ['--mat-form-field-outlined-container-shape', 'var(--fakemex-radius)'],
      ['--mat-form-field-enabled-select-arrow-color', muted],
      ['--mat-form-field-disabled-select-arrow-color', muted],
      ['--mat-select-trigger-text-color', text],
      ['--mat-select-value-text-color', text],
      ['--mat-select-enabled-trigger-text-color', inputText],
      ['--mat-select-placeholder-text-color', muted],
      ['--mat-select-enabled-arrow-color', muted],
      ['--mat-select-disabled-arrow-color', muted],
      ['--mat-select-disabled-trigger-text-color', muted],
      ['--mat-autocomplete-background-color', panelSurface],
      ['--mat-autocomplete-container-shape', 'var(--fakemex-radius)'],
      ['--mat-list-list-item-label-text-color', text],
      ['--mat-select-panel-background-color', surfaceContrast],
      ['--mat-select-panel-background-color-hover', chartBg],
      ['--mat-button-outlined-label-text-color', text],
      ['--mat-button-outlined-outline-color', border],
      ['--mat-button-outlined-state-layer-color', accent],
      ['--mat-button-filled-container-shape', 'var(--fakemex-radius)'],
      ['--mat-button-outlined-container-shape', 'var(--fakemex-radius)'],
      ['--mat-button-text-container-shape', 'var(--fakemex-radius)'],
      ['--mat-button-protected-container-color', accent],
      ['--mat-button-protected-label-text-color', isLight ? '#ffffff' : shellBg],
      ['--mat-button-protected-state-layer-color', text],
      ['--mat-checkbox-label-text-color', text],
      ['--mat-checkbox-selected-icon-color', buy],
      ['--mat-checkbox-selected-focus-icon-color', buy],
      ['--mat-checkbox-selected-hover-icon-color', buy],
      ['--mat-checkbox-unselected-icon-color', muted],
      ['--mat-checkbox-unselected-focus-icon-color', text],
      ['--mat-checkbox-unselected-hover-icon-color', text],
      ['--mdc-theme-primary', accent],
      ['--mdc-theme-on-primary', isLight ? panelSurface : '#ffffff'],
      ['--mdc-theme-secondary', buy],
      ['--mdc-theme-on-secondary', '#ffffff'],
      ['--mdc-filled-text-field-container-color', inputBg],
      ['--mdc-filled-text-field-input-text-color', inputText],
      ['--mdc-filled-text-field-label-text-color', muted],
      ['--mdc-filled-text-field-active-indicator-color', accent],
      ['--mdc-filled-text-field-hover-label-text-color', inputText],
      ['--mdc-outlined-text-field-outline-color', inputBorder],
      ['--mdc-outlined-text-field-focus-outline-color', accent],
      ['--mdc-outlined-text-field-hover-outline-color', accent],
      ['--mdc-outlined-text-field-input-text-color', inputText],
      ['--mdc-outlined-text-field-label-text-color', muted],
      ['--mdc-outlined-text-field-hover-label-text-color', text],
      ['--mdc-outlined-text-field-caret-color', text],
      ['--mdc-outlined-text-field-container-shape', 'var(--fakemex-radius)'],
      ['--mdc-filled-text-field-container-shape', 'var(--fakemex-radius)'],
      ['--mdc-outlined-text-field-hover-state-layer-color', accent],
      ['--mdc-select-outline-color', inputBorder],
      ['--mdc-select-disabled-arrow-color', muted],
      ['--mdc-select-hover-state-layer-color', accent],
      ['--mdc-select-focus-state-layer-color', accent],
      ['--mdc-select-pressed-state-layer-color', border],
      ['--mdc-select-enabled-arrow-color', muted],
      ['--mdc-select-container-shape', 'var(--fakemex-radius)'],
      ['--mdc-filled-button-container-color', accent],
      ['--mdc-filled-button-label-text-color', isLight ? '#ffffff' : panelSurface],
      ['--mdc-filled-button-container-shape', 'var(--fakemex-radius)'],
      ['--mdc-filled-button-hover-state-layer-color', chartBg],
      ['--mdc-filled-button-focus-state-layer-color', chartBg],
      ['--mdc-outlined-button-outline-color', accent],
      ['--mdc-outlined-button-label-text-color', text],
      ['--mdc-outlined-button-container-shape', 'var(--fakemex-radius)'],
      ['--mdc-checkbox-selected-icon-color', buy],
      ['--mdc-checkbox-unselected-icon-color', muted],
      ['--mdc-checkbox-selected-focus-icon-color', buy],
      ['--mdc-checkbox-selected-hover-icon-color', buy],
      ['--mdc-checkbox-selected-hover-state-layer-color', buy],
      ['--mdc-checkbox-unselected-hover-state-layer-color', border],
      ['--mdc-slider-active-track-color', buy],
      ['--mdc-slider-inactive-track-color', isLight ? border : border],
      ['--mdc-slider-handle-color', buy],
      ['--mdc-slider-focus-handle-color', buy],
      ['--mdc-slider-disabled-active-track-color', border],
      ['--mdc-slider-disabled-handle-color', muted],
      ['--mdc-tab-indicator-active-indicator-color', accent],
      ['--mdc-tab-text-label-text-color', text],
      ['--mdc-tab-text-label-active-text-color', text],
      ['--mdc-tab-outline-color', border],
      ['--mat-table-row-item-outline-color', border],
      ['--mat-table-background-color', panelSurface],
      ['--mat-sys-outline-variant', border],
    ];
    for (const [key, value] of materialOverrides) {
      root.style.setProperty(key, value);
    }

    root.style.setProperty('--fakemex-theme-variant', theme.variant);
    root.style.setProperty('--fakemex-cursor', theme.roles.cursor);
    root.style.setProperty('--fakemex-selection', theme.roles.selectionBackground);
    root.style.setProperty('color-scheme', theme.variant);
    root.style.setProperty('--fakemex-chart-bg', chartBg);
    root.style.setProperty('--fakemex-chart-axis', chartAxis);
    root.style.setProperty('--fakemex-chart-grid', chartGrid);
    root.style.setProperty('--fakemex-chart-text', chartText);
  }
}
