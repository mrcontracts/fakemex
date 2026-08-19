import { CommonModule } from '@angular/common';
import { ChangeDetectionStrategy, Component, Input } from '@angular/core';
import { sortTableRows, TableSortDirection } from '../table-sort';

export type SimpleTableCellClass = (
  cell: unknown,
  row: readonly unknown[],
  columnIndex: number,
) => string;

@Component({
  selector: 'app-simple-table',
  standalone: true,
  imports: [CommonModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            @for (header of headers; track $index; let columnIndex = $index) {
              <th [attr.aria-sort]="ariaSort(columnIndex)">
                <button
                  type="button"
                  class="sort-header"
                  (click)="sortBy(columnIndex)"
                  [attr.aria-label]="sortLabel(header, columnIndex)"
                >
                  <span>{{ header }}</span>
                  <span class="sort-indicator" aria-hidden="true">{{ sortIndicator(columnIndex) }}</span>
                </button>
              </th>
            }
          </tr>
        </thead>
        <tbody>
          @if (rows.length === 0) {
            <tr>
              <td [attr.colspan]="headers.length" class="empty">
                No data yet
              </td>
            </tr>
          } @else {
            @for (row of sortedRows; track row) {
              <tr>
                @for (cell of row; track $index; let columnIndex = $index) {
                  <td [ngClass]="resolveCellClass(cell, row, columnIndex)">{{ cell }}</td>
                }
              </tr>
            }
          }
        </tbody>
      </table>
    </div>
  `,
  styles: [
    `
      :host(.strong-first-column) td:first-child {
        font-weight: 700;
      }

      .table-wrap {
        overflow: auto;
        min-height: 0;
      }

      table {
        width: 100%;
        border-collapse: collapse;
        font-size: 0.74rem;
      }

      th,
      td {
        text-align: right;
        padding: 0.38rem 0.42rem;
        border-bottom: 1px dashed var(--fakemex-border);
        color: var(--fakemex-text);
      }

      th {
        position: sticky;
        top: 0;
        background: color-mix(in srgb, var(--fakemex-panel-header) 70%, var(--fakemex-shell-bg) 30%);
        z-index: 1;
        color: var(--fakemex-muted);
        text-transform: uppercase;
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

      tbody td {
        font-weight: 700;
      }

      .empty {
        text-align: center;
        color: var(--fakemex-muted);
        font-weight: 400;
      }

      td.cell-buy,
      td.cell-positive {
        color: var(--fakemex-buy);
      }

      td.cell-sell,
      td.cell-negative {
        color: var(--fakemex-sell);
      }
    `,
  ],
})
export class SimpleTableComponent {
  @Input() headers: string[] = [];
  @Input() rows: unknown[][] = [];
  @Input() cellClass?: SimpleTableCellClass;
  sortColumnIndex: number | null = null;
  sortDirection: TableSortDirection = 'asc';

  get sortedRows(): unknown[][] {
    if (this.sortColumnIndex === null) return this.rows;
    return sortTableRows(this.rows, this.sortColumnIndex, this.sortDirection);
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

  resolveCellClass(cell: unknown, row: readonly unknown[], columnIndex: number): string {
    return this.cellClass?.(cell, row, columnIndex) ?? '';
  }
}
