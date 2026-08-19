import { Component, Input } from '@angular/core';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-panel',
  standalone: true,
  imports: [CommonModule],
  template: `
    <section class="fm-panel">
      <header class="fm-panel__header">
        <h2 class="fm-panel__title">{{ title }}</h2>
        <ng-content select="[actions]"></ng-content>
      </header>
      <div class="fm-panel__body">
        <ng-content></ng-content>
      </div>
    </section>
  `,
  styles: [
    `
      .fm-panel {
        background: linear-gradient(160deg, var(--fakemex-panel-header) 0%, var(--fakemex-panel-body) 100%);
        border: 1px solid var(--fakemex-border);
        border-radius: var(--fakemex-radius);
        height: 100%;
        min-height: 0;
        display: flex;
        flex-direction: column;
        overflow: hidden;
        box-shadow:
          0 10px 30px rgb(0 0 0 / 25%),
          inset 0 1px 0 rgb(255 255 255 / 4%);
      }

      .fm-panel__header {
        border-bottom: 1px solid var(--fakemex-border);
        display: flex;
        justify-content: space-between;
        gap: 0.75rem;
        align-items: center;
        padding: 0.8rem 0.9rem;
      }

      .fm-panel__title {
        color: var(--fakemex-text);
        font-size: 0.84rem;
        text-transform: uppercase;
        letter-spacing: 0.08em;
        margin: 0;
      }

      .fm-panel__body {
        flex: 1;
        padding: 0.85rem;
        min-height: 0;
        overflow: auto;
      }

      :host {
        display: block;
        height: 100%;
      }
    `,
  ],
})
export class PanelShellComponent {
  @Input()
  title = '';
}
