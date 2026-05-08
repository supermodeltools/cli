import { calculateSubtotal } from './pricing.js';
import type { CartItem } from './types.js';

export function createCheckoutQuote(items: CartItem[], taxRate: number): { subtotal: number; total: number } {
  const subtotal = calculateSubtotal(items);
  return {
    subtotal,
    total: subtotal * (1 + taxRate),
  };
}
