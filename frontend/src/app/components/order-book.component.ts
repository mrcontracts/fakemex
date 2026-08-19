import { CommonModule } from '@angular/common';
import { ChangeDetectionStrategy, Component, Input } from '@angular/core';
import { BookLevel } from '../models';

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
            <th>Price</th>
            <th>Size</th>
            <th>Total</th>
          </tr>
        </thead>
        <tbody>
          @for (row of bids; track row.price) {
            <tr class="bid">
              <td>{{ row.price }}</td>
              <td>{{ row.size }}</td>
              <td>{{ row.total }}</td>
            </tr>
          } @for (row of asks; track row.price) {
            <tr class="ask">
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

      th:first-child,
      td:first-child {
        text-align: left;
      }

      td {
        text-align: right;
        padding: 0.2rem 0;
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
