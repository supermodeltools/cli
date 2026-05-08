import { summarizeCart } from './cart.js';
import type { CartItem } from './types.js';

export function buildSalesReport(items: CartItem[]): string {
  const summary = summarizeCart(items);
  return `${summary.count} items: $${summary.subtotal.toFixed(2)}`;
}
