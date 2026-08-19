import '@angular/compiler';
import { describe, expect, it, vi } from 'vitest';
import { FormBuilder } from '@angular/forms';
import { OrderFormComponent } from './order-form.component';

describe('OrderFormComponent', () => {
  it('keeps size in base units and updates dynamic labels', () => {
    const component = new OrderFormComponent(new FormBuilder());
    component.base = 'ETH';
    component.quote = 'USDC';
    expect(component.baseSymbol).toBe('ETH');
    expect(component.quoteSymbol).toBe('USDC');
    expect(component.estimatedNotionalText).toBeNull();
    component.form.patchValue({
      side: 'buy',
      kind: 'market',
      size: '2',
    });
    component.markPrice = '1800';
    expect(component.estimatedNotionalText).toBe('Est. notional 3,600 USDC');
  });

  it('uses mark price for market notional and explicit fields for limit/trigger-limit notional', () => {
    const component = new OrderFormComponent(new FormBuilder());
    component.base = 'BTC';
    component.quote = 'USD';
    component.markPrice = '30000';

    component.form.patchValue({
      side: 'buy',
      kind: 'market',
      size: '1',
    });
    expect(component.estimatedNotionalText).toBe('Est. notional 30,000 USD');

    component.form.patchValue({
      side: 'buy',
      kind: 'limit',
      size: '0.2',
      price: '40000',
    });
    expect(component.estimatedNotionalText).toBe('Est. notional 8,000 USD');
  });

  it('emits selected symbol without changing size semantics', () => {
    const component = new OrderFormComponent(new FormBuilder());
    component.symbol = 'BTC';
    component.base = 'BTC';
    component.quote = 'USD';
    component.form.patchValue({
      side: 'buy',
      kind: 'market',
      size: '2',
    });

    const emitSpy = vi.spyOn(component.sendOrder, 'emit');
    component.submit();

    expect(emitSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        symbol: 'BTC',
        size: '2',
      }),
    );

    component.symbol = 'ETH';
    emitSpy.mockClear();
    component.submit();
    expect(emitSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        symbol: 'ETH',
        size: '2',
      }),
    );
  });

  it('blocks limit orders without a price', () => {
    const component = new OrderFormComponent(new FormBuilder());
    component.form.patchValue({
      side: 'buy',
      kind: 'limit',
      size: '1.25',
      price: '',
    });

    const emitSpy = vi.spyOn(component.sendOrder, 'emit');
    component.submit();

    expect(component.formError).toBe('Price is required for limit variants');
    expect(emitSpy).not.toHaveBeenCalled();
  });

  it('requires trigger price for take-profit and stop variants', () => {
    const component = new OrderFormComponent(new FormBuilder());
    component.form.patchValue({
      side: 'sell',
      kind: 'takeProfitMarket',
      size: '0.5',
      price: '',
      triggerPrice: '',
    });

    const emitSpy = vi.spyOn(component.sendOrder, 'emit');
    component.submit();

    expect(component.formError).toBe('Trigger price is required for stop/take-profit');
    expect(emitSpy).not.toHaveBeenCalled();
  });

  it('emits valid order payload for market orders', () => {
    const component = new OrderFormComponent(new FormBuilder());
    component.form.patchValue({
      side: 'buy',
      kind: 'market',
      size: '2',
      attachedTp: '50000',
      attachedSl: '42000',
      slippagePercent: '0.5',
      reduceOnly: true,
    });

    const emitSpy = vi.spyOn(component.sendOrder, 'emit');
    component.submit();

    expect(component.formError).toBeNull();
    expect(emitSpy).toHaveBeenCalledOnce();
    expect(component.form.valid).toBe(true);
    expect(emitSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        side: 'buy',
        kind: 'market',
        size: '2',
        timeInForce: 'gtc',
        slippagePercent: '0.5',
        reduceOnly: true,
        attachedTakeProfit: { triggerPrice: '50000' },
        attachedStopLoss: { triggerPrice: '42000' },
      }),
    );
  });
});
