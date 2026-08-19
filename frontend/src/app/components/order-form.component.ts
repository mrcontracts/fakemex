import { CommonModule } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  EventEmitter,
  Input,
  Output,
} from '@angular/core';
import { FormBuilder, FormGroup, ReactiveFormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatCheckboxModule } from '@angular/material/checkbox';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';
import { OrderKind, OrderRequest, OrderSide, TimeInForce } from '../models';

@Component({
  selector: 'app-order-form',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, MatFormFieldModule, MatSelectModule, MatInputModule, MatCheckboxModule, MatButtonModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <form class="form" [formGroup]="form" (ngSubmit)="submit()">
      <div class="form-row">
        <mat-form-field appearance="outline" class="field">
          <mat-label>Side</mat-label>
          <mat-select formControlName="side">
            <mat-option value="buy">Buy</mat-option>
            <mat-option value="sell">Sell</mat-option>
          </mat-select>
        </mat-form-field>

        <mat-form-field appearance="outline" class="field">
          <mat-label>Order type</mat-label>
          <mat-select formControlName="kind">
            <mat-option value="limit">Limit</mat-option>
            <mat-option value="market">Market</mat-option>
            <mat-option value="stopMarket">Stop Market</mat-option>
            <mat-option value="stopLimit">Stop Limit</mat-option>
            <mat-option value="takeProfitMarket">Take-Profit Market</mat-option>
            <mat-option value="takeProfitLimit">Take-Profit Limit</mat-option>
          </mat-select>
        </mat-form-field>
      </div>

      <mat-form-field appearance="outline" class="field size-field">
        <mat-label>Size ({{ baseSymbol }})</mat-label>
        <input matInput type="text" inputmode="decimal" formControlName="size" placeholder="0.00" />
      </mat-form-field>
      @if (estimatedNotionalText) {
        <p class="order-notional" aria-live="polite">{{ estimatedNotionalText }}</p>
      }
      @if (requiresPrice()) {
        <mat-form-field appearance="outline" class="field price-field">
          <mat-label>Limit price ({{ quoteSymbol }})</mat-label>
          <input matInput type="text" inputmode="decimal" formControlName="price" placeholder="0.00" />
        </mat-form-field>
      }
      @if (requiresTrigger()) {
        <mat-form-field appearance="outline" class="field trigger-price-field">
          <mat-label>Trigger price ({{ quoteSymbol }})</mat-label>
          <input matInput type="text" inputmode="decimal" formControlName="triggerPrice" placeholder="Stop / TP trigger" />
        </mat-form-field>
      }
      <mat-checkbox formControlName="reduceOnly">Reduce Only</mat-checkbox>
      @if (requiresTriggerLimit()) {
        <mat-form-field appearance="outline" class="field trigger-limit-field">
          <mat-label>Trigger limit price ({{ quoteSymbol }})</mat-label>
          <input matInput type="text" inputmode="decimal" formControlName="triggerLimitPrice" placeholder="For stop / tp limits" />
        </mat-form-field>
      }
      @if (isMarketType()) {
        <mat-form-field appearance="outline" class="field">
          <mat-label>Market slippage %</mat-label>
          <input matInput type="text" inputmode="decimal" formControlName="slippagePercent" placeholder="0.10" />
        </mat-form-field>
      }
      @if (!isMarketType()) {
        <mat-form-field appearance="outline" class="field">
          <mat-label>Max slippage %</mat-label>
          <input matInput type="text" inputmode="decimal" formControlName="slippagePercent" placeholder="0.10" />
        </mat-form-field>
      }

      <div class="split-line">
        <mat-form-field appearance="outline" class="field">
          <mat-label>TIF</mat-label>
          <mat-select formControlName="timeInForce">
            <mat-option value="gtc">GTC</mat-option>
            <mat-option value="ioc">IOC</mat-option>
            <mat-option value="alo">ALO</mat-option>
          </mat-select>
        </mat-form-field>
        <mat-form-field appearance="outline" class="field">
          <mat-label>Attached TP price</mat-label>
          <input matInput type="text" inputmode="decimal" formControlName="attachedTp" />
        </mat-form-field>
        <mat-form-field appearance="outline" class="field">
          <mat-label>Attached SL price</mat-label>
          <input matInput type="text" inputmode="decimal" formControlName="attachedSl" />
        </mat-form-field>
      </div>
      @if (formError) {
        <p class="error" role="status">{{ formError }}</p>
      }
      <button mat-raised-button type="submit" color="primary" [disabled]="form.invalid">{{ submitLabel }}</button>
    </form>
  `,
  styles: [
    `
      .form {
        display: grid;
        gap: 0.45rem;
        color: var(--fakemex-text);
      }

      .form-row {
        display: flex;
        gap: 0.45rem;
        flex-wrap: wrap;
      }

      .field {
        width: 100%;
      }

      .form-row .field {
        flex: 1 1 min(18rem, 50%);
      }

      .split-line {
        display: flex;
        gap: 0.55rem;
        flex-wrap: wrap;
      }

      .split-line .field {
        flex: 1 1 9.5rem;
      }

      mat-checkbox {
        color: var(--fakemex-text);
      }

      .form ::ng-deep .mat-mdc-form-field-subscript-wrapper {
        min-height: 0;
        height: 0;
      }

.error {
        color: #ff95a4;
        margin: 0;
      }

      button {
        align-items: center;
        justify-content: center;
      }

      button::ng-deep {
        margin-top: 0.2rem;
      }

      .order-notional {
        margin: 0;
        padding: 0.1rem 0.45rem 0.25rem;
        font-size: 0.77rem;
        color: var(--fakemex-muted);
      }
    `,
  ],
})
export class OrderFormComponent {
  private _symbol = 'BTC';
  private _base = 'BTC';
  private _quote = 'USD';
  private _markPrice = '';

  @Input() submitLabel = 'Submit Order';
  @Input() processing = false;
  @Output() sendOrder = new EventEmitter<OrderRequest>();
  @Input()
  set symbol(value: string) {
    const next = value.trim() || 'BTC';
    this._symbol = next;
    this.form.controls['symbol'].setValue(next, { emitEvent: false });
  }
  @Input()
  set base(value: string) {
    this._base = (value || 'BTC').trim() || 'BTC';
  }
  @Input()
  set quote(value: string) {
    this._quote = (value || 'USD').trim() || 'USD';
  }
  @Input()
  set markPrice(value: string) {
    this._markPrice = value ?? '';
  }

  form: FormGroup;

  formError: string | null = null;

  get estimatedNotionalText() {
    const notional = this.estimatedNotional();
    if (!notional) return null;
    return `Est. notional ${this.formatNotional(notional)} ${this.quoteSymbol}`;
  }

  get baseSymbol() {
    return this._base;
  }

  get quoteSymbol() {
    return this._quote;
  }

  requiresPrice() {
    return ['limit', 'stopLimit', 'takeProfitLimit'].includes(this.form.controls['kind']?.value);
  }

  requiresTrigger() {
    const kind = this.form.controls['kind']?.value as OrderKind;
    return kind === 'stopMarket' || kind === 'stopLimit' || kind === 'takeProfitMarket' || kind === 'takeProfitLimit';
  }

  requiresTriggerLimit() {
    const kind = this.form.controls['kind']?.value as OrderKind;
    return kind === 'stopLimit' || kind === 'takeProfitLimit';
  }

  isMarketType() {
    return this.form.controls['kind']?.value === 'market';
  }

  constructor(private readonly fb: FormBuilder) {
    this.form = this.fb.group({
      symbol: 'BTC',
      side: 'buy' as OrderSide,
      kind: 'limit' as OrderKind,
      size: '',
      price: '',
      triggerPrice: '',
      triggerLimitPrice: '',
      timeInForce: 'gtc' as TimeInForce,
      reduceOnly: false,
      slippagePercent: '',
      attachedTp: '',
      attachedSl: '',
    });
  }

  submit() {
    this.formError = null;
    if (this.processing || this.form.invalid) {
      this.formError = 'Please complete the required fields.';
      return;
    }

    const { 
      symbol,
      side,
      kind,
      size,
      price,
      triggerPrice,
      triggerLimitPrice,
      timeInForce,
      reduceOnly,
      slippagePercent,
      attachedTp,
      attachedSl,
    } = this.form.getRawValue();

    if (side !== 'buy' && side !== 'sell') {
      this.formError = 'Invalid side';
      return;
    }
    if (
      kind !== 'limit' &&
      kind !== 'market' &&
      kind !== 'stopMarket' &&
      kind !== 'stopLimit' &&
      kind !== 'takeProfitMarket' &&
      kind !== 'takeProfitLimit'
    ) {
      this.formError = 'Invalid order type';
      return;
    }
    if (timeInForce !== 'gtc' && timeInForce !== 'ioc' && timeInForce !== 'alo') {
      this.formError = 'Invalid time in force';
      return;
    }

    const attachedTakeProfit =
      attachedTp
        ? {
            triggerPrice: attachedTp,
          }
        : undefined;
    const attachedStopLoss =
      attachedSl
        ? {
            triggerPrice: attachedSl,
          }
        : undefined;

    if (!size) {
      this.formError = 'Size is required';
      return;
    }
    if ((kind === 'limit' || kind === 'stopLimit' || kind === 'takeProfitLimit') && !price) {
      this.formError = 'Price is required for limit variants';
      return;
    }
    if ((kind.startsWith('stop') || kind.startsWith('takeProfit')) && !triggerPrice) {
      this.formError = 'Trigger price is required for stop/take-profit';
      return;
    }

    this.sendOrder.emit({
      symbol,
      side,
      kind,
      size: size || '0',
      price: price || undefined,
      triggerPrice: triggerPrice || undefined,
      triggerLimitPrice: triggerLimitPrice || undefined,
      timeInForce,
      reduceOnly,
      slippagePercent: slippagePercent || undefined,
      attachedTakeProfit,
      attachedStopLoss,
    });
  }

  private estimatedNotional(): number | null {
    const kind = this.form.controls['kind']?.value as OrderKind;
    const size = this.parseNumber(this.form.controls['size']?.value);
    if (!size || size <= 0) return null;

    let executionPrice: number | null = null;
    if (kind === 'market') {
      executionPrice = this.parseNumber(this._markPrice);
    } else if (this.requiresPrice()) {
      executionPrice = this.parseNumber(this.form.controls['price']?.value);
      if (executionPrice === null && this.requiresTriggerLimit()) {
        executionPrice = this.parseNumber(this.form.controls['triggerLimitPrice']?.value);
      }
    } else if (this.requiresTrigger()) {
      executionPrice = this.parseNumber(this.form.controls['triggerPrice']?.value);
    }

    if (executionPrice === null) {
      executionPrice = this.parseNumber(this._markPrice);
    }
    if (executionPrice === null) return null;

    const notional = size * executionPrice;
    if (!Number.isFinite(notional)) return null;
    return notional;
  }

  private formatNotional(value: number): string {
    const abs = Math.abs(value);
    const maximumFractionDigits = abs >= 100 ? 2 : abs >= 1 ? 4 : 8;
    return new Intl.NumberFormat('en-US', {
      minimumFractionDigits: 0,
      maximumFractionDigits,
    }).format(value);
  }

  private parseNumber(value: string | null | undefined): number | null {
    if (!value) return null;
    const parsed = Number(value);
    if (!Number.isFinite(parsed)) return null;
    return parsed;
  }
}
