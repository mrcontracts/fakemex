import { CommonModule } from '@angular/common';
import { ChangeDetectionStrategy, Component, Input } from '@angular/core';

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
            @for (header of headers; track header) {
              <th>{{ header }}</th>
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
            @for (row of rows; track $index) {
              <tr>
                @for (cell of row; track $index) {
                  <td>{{ cell }}</td>
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

      th:first-child,
      td:first-child {
        text-align: left;
      }

      .empty {
        text-align: center;
        color: var(--fakemex-muted);
      }
    `,
  ],
})
export class SimpleTableComponent {
  @Input() headers: string[] = [];
  @Input() rows: unknown[][] = [];
}
