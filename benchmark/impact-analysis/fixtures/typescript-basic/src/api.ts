import { createCheckoutQuote } from './checkout.js';
import type { CartItem } from './types.js';

export function renderQuote(items: CartItem[]): string {
  const quote = createCheckoutQuote(items, 0.07);
  return JSON.stringify(quote);
}
