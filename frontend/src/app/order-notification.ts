import { OrderRequest, OrderWriteResult } from './models';

export function orderSubmissionMessage(
  result: OrderWriteResult,
  order: OrderRequest,
  referencePrice: string | undefined,
  quote: string,
): string {
  const fillPrice = positiveNumber(result.averagePrice);
  if (fillPrice === null) {
    return 'Order submitted · Not filled yet · Slippage available after fill';
  }

  const reference = positiveNumber(referencePrice);
  const filledAt = formatPrice(fillPrice);
  if (reference === null) {
    return `Trade submitted · Filled at ${filledAt} ${quote} · Slippage unavailable`;
  }

  const direction = order.side === 'buy' ? 1 : -1;
  const slippage = ((fillPrice - reference) / reference) * 100 * direction;
  const normalizedSlippage = Math.abs(slippage) < 0.00005 ? 0 : slippage;
  const slippageText = `${normalizedSlippage > 0 ? '+' : ''}${normalizedSlippage.toFixed(4)}%`;
  return `Trade submitted · Filled at ${filledAt} ${quote} · Slippage ${slippageText}`;
}

export function orderReferencePrice(order: OrderRequest, markPrice: string | undefined): string | undefined {
  if (order.kind === 'market') return markPrice;
  return order.price || order.triggerLimitPrice || markPrice;
}

function positiveNumber(value: string | undefined): number | null {
  if (!value) return null;
  const parsed = Number(value.replace(/,/g, '').trim());
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
}

function formatPrice(value: number): string {
  return new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 8,
  }).format(value);
}
