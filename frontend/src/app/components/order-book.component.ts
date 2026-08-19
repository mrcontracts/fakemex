import { CommonModule } from '@angular/common';
import { ChangeDetectionStrategy, Component, Input } from '@angular/core';
import { BookLevel } from '../models';
import { sortTableItems, TableSortDirection } from '../table-sort';

const bookColumns = [
  { label: 'Price', value: (row: BookLevel) => row.price },
  { label: 'Size', value: (row: BookLevel) => row.size },
  { label: 'Total', value: (row: BookLevel) => row.total },
] as const;

@Component({
  selector: 'app-order-book',
  standalone: true,
  imports: [CommonModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="book-grid">
      <table>
        <thead>
          <tr>
            @for (column of columns; track column.label; let columnIndex = $index) {
              <th [attr.aria-sort]="ariaSort(columnIndex)">
                <button
                  type="button"
                  class="sort-header"
                  (click)="sortBy(columnIndex)"
                  [attr.aria-label]="sortLabel(column.label, columnIndex)"
                >
                  <span>{{ column.label }}</span>
                  <span class="sort-indicator" aria-hidden="true">{{ sortIndicator(columnIndex) }}</span>
                </button>
              </th>
            }
          </tr>
        </thead>
        <tbody>
          @for (row of displayedRows; track row.side + ':' + row.price) {
            <tr [class.bid]="row.side === 'buy'" [class.ask]="row.side === 'sell'">
              <td>{{ row.price }}</td>
              <td>{{ row.size }}</td>
              <td>{{ row.total }}</td>
            </tr>
          }
        </tbody>
      </table>
    </div>
  `,
  styles: [
    `
      .book-grid {
        color: var(--fakemex-text);
      }

      table {
        width: 100%;
        border-collapse: collapse;
        font-size: 0.76rem;
      }

      th {
        text-transform: uppercase;
        text-align: right;
        font-size: 0.68rem;
        color: var(--fakemex-muted);
        letter-spacing: 0.06rem;
        border-bottom: 1px solid var(--fakemex-border);
        padding-bottom: 0.3rem;
      }

      .sort-header {
        display: inline-flex;
        align-items: center;
        justify-content: flex-end;
        gap: 0.25rem;
        width: 100%;
        padding: 0;
        border: 0;
        color: inherit;
        background: transparent;
        font: inherit;
        font-weight: 700;
        text-align: inherit;
        text-transform: inherit;
        cursor: pointer;
      }

      .sort-header:focus-visible {
        outline: 2px solid var(--fakemex-accent);
        outline-offset: 2px;
      }

      .sort-indicator {
        display: inline-block;
        min-width: 0.7rem;
        color: var(--fakemex-accent);
      }

      th:first-child,
      td:first-child {
        text-align: left;
      }

      th:first-child .sort-header {
        justify-content: flex-start;
      }

      td {
        text-align: right;
        padding: 0.2rem 0;
        font-weight: 700;
      }

      tr {
        transition: background 120ms ease;
      }

      tr.bid {
        background: linear-gradient(90deg, color-mix(in srgb, var(--fakemex-buy) 15%, transparent 85%), transparent);
      }

      tr.ask {
        background: linear-gradient(90deg, color-mix(in srgb, var(--fakemex-sell) 15%, transparent 85%), transparent);
      }
    `,
  ],
})
export class OrderBookComponent {
  @Input() book: BookLevel[] = [];
  readonly columns = bookColumns;
  sortColumnIndex: number | null = null;
  sortDirection: TableSortDirection = 'asc';

  get displayedRows(): BookLevel[] {
    const defaultRows = [...this.bids, ...this.asks];
    if (this.sortColumnIndex === null) return defaultRows;
    const column = this.columns[this.sortColumnIndex];
    return column
      ? sortTableItems(defaultRows, column.value, this.sortDirection)
      : defaultRows;
  }

  sortBy(columnIndex: number): void {
    if (this.sortColumnIndex === columnIndex) {
      this.sortDirection = this.sortDirection === 'asc' ? 'desc' : 'asc';
      return;
    }
    this.sortColumnIndex = columnIndex;
    this.sortDirection = 'asc';
  }

  ariaSort(columnIndex: number): 'ascending' | 'descending' | null {
    if (this.sortColumnIndex !== columnIndex) return null;
    return this.sortDirection === 'asc' ? 'ascending' : 'descending';
  }

  sortIndicator(columnIndex: number): string {
    if (this.sortColumnIndex !== columnIndex) return '↕';
    return this.sortDirection === 'asc' ? '↑' : '↓';
  }

  sortLabel(header: string, columnIndex: number): string {
    if (this.sortColumnIndex !== columnIndex) return `Sort ${header} ascending`;
    return `Sort ${header} ${this.sortDirection === 'asc' ? 'descending' : 'ascending'}`;
  }

  get bids(): BookLevel[] {
    return this.book
      .filter((entry) => entry.side === 'buy')
      .sort((a, b) => Number(b.price) - Number(a.price));
  }

  get asks(): BookLevel[] {
    return this.book
      .filter((entry) => entry.side === 'sell')
      .sort((a, b) => Number(a.price) - Number(b.price));
  }
}
