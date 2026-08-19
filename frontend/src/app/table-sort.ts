export type TableSortDirection = 'asc' | 'desc';

const naturalCollator = new Intl.Collator(undefined, {
  numeric: true,
  sensitivity: 'base',
});

export function sortTableItems<T>(
  items: readonly T[],
  valueForItem: (item: T) => unknown,
  direction: TableSortDirection,
): T[] {
  const multiplier = direction === 'asc' ? 1 : -1;
  return items
    .map((item, originalIndex) => ({ item, originalIndex }))
    .sort((left, right) => {
      const comparison = compareTableValues(valueForItem(left.item), valueForItem(right.item));
      return comparison === 0
        ? left.originalIndex - right.originalIndex
        : comparison * multiplier;
    })
    .map(({ item }) => item);
}

export function sortTableRows<T extends readonly unknown[]>(
  rows: readonly T[],
  columnIndex: number,
  direction: TableSortDirection,
): T[] {
  return sortTableItems(rows, (row) => row[columnIndex], direction);
}

export function compareTableValues(left: unknown, right: unknown): number {
  const leftMissing = isMissing(left);
  const rightMissing = isMissing(right);
  if (leftMissing || rightMissing) {
    if (leftMissing && rightMissing) return 0;
    return leftMissing ? 1 : -1;
  }

  const leftNumber = tableNumber(left);
  const rightNumber = tableNumber(right);
  if (leftNumber !== null && rightNumber !== null) {
    return leftNumber - rightNumber;
  }

  return naturalCollator.compare(String(left).trim(), String(right).trim());
}

function isMissing(value: unknown): boolean {
  if (value === null || value === undefined) return true;
  const text = String(value).trim();
  return text === '' || text === '--' || text === '—';
}

function tableNumber(value: unknown): number | null {
  if (typeof value === 'number') {
    return Number.isFinite(value) ? value : null;
  }
  if (typeof value !== 'string') return null;

  const normalized = value
    .trim()
    .replace(/[\s,]/g, '')
    .replace(/^[€£$]/, '')
    .replace(/[%x]$/i, '');
  if (!/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)$/.test(normalized)) return null;
  const parsed = Number(normalized);
  return Number.isFinite(parsed) ? parsed : null;
}
