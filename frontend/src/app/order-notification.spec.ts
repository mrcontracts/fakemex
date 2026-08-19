import { describe, expect, it } from 'vitest';
import { OrderRequest, OrderWriteResult } from './models';
import { orderReferencePrice, orderSubmissionMessage } from './order-notification';

const marketBuy: OrderRequest = {
  symbol: 'BTC',
  side: 'buy',
  kind: 'market',
  size: '0.1',
  reduceOnly: false,
};

describe('order submission notification', () => {
  it('shows actual fill price and adverse buy slippage', () => {
    const result: OrderWriteResult = {
      requestId: 'request-1',
      status: 'ok',
      averagePrice: '50125',
      filled: '0.1',
    };

    expect(orderSubmissionMessage(result, marketBuy, '50000', 'USD')).toBe(
      'Trade submitted · Filled at 50,125 USD · Slippage +0.2500%',
    );
  });

  it('treats a higher sell fill as price improvement', () => {
    const result: OrderWriteResult = {
      requestId: 'request-2',
      status: 'ok',
      averagePrice: '101',
    };
    const sell = { ...marketBuy, side: 'sell' as const };

    expect(orderSubmissionMessage(result, sell, '100', 'USDC')).toContain('Slippage -1.0000%');
  });

  it('reports resting orders without inventing a fill', () => {
    const result: OrderWriteResult = { requestId: 'request-3', status: 'ok', orderId: '42' };

    expect(orderSubmissionMessage(result, marketBuy, '50000', 'USD')).toBe(
      'Order submitted · Not filled yet · Slippage available after fill',
    );
  });

  it('uses mark price for market orders and limit price for limit orders', () => {
    expect(orderReferencePrice(marketBuy, '50000')).toBe('50000');
    expect(orderReferencePrice({ ...marketBuy, kind: 'limit', price: '49000' }, '50000')).toBe('49000');
  });
});
