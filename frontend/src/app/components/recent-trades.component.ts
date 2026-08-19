import { CommonModule } from '@angular/common';
import { ChangeDetectionStrategy, Component, Input } from '@angular/core';
import { Trade } from '../models';

@Component({
  selector: 'app-recent-trades',
  standalone: true,
  imports: [CommonModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <ul class="trade-list">
      @for (trade of trades; track trade.ts + ':' + trade.px + ':' + trade.size + ':' + $index) {
        <li>
          <span>{{ trade.ts | date:'HH:mm:ss' }}</span>
          <span [class.buy]="trade.side === 'buy'" [class.sell]="trade.side === 'sell'">
            {{ trade.side.toUpperCase() }}
          </span>
          <span>{{ trade.size }}</span>
          <span>{{ trade.px }}</span>
        </li>
      }
    </ul>
  `,
  styles: [
    `
      .trade-list {
        list-style: none;
        margin: 0;
        padding: 0;
        font-size: 0.78rem;
        color: var(--fakemex-text);
      }

      li {
        display: grid;
        grid-template-columns: 2fr 1fr 1fr 1fr;
        gap: 0.4rem;
        padding: 0.24rem 0;
        border-bottom: 1px solid var(--fakemex-border);
        font-weight: 700;
      }

      .buy {
        color: var(--fakemex-buy);
      }

      .sell {
        color: var(--fakemex-sell);
      }
    `,
  ],
})
export class RecentTradesComponent {
  @Input() trades: Trade[] = [];
}
