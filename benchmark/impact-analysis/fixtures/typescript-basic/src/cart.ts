import { calculateSubtotal } from './pricing.js';
import type { CartItem } from './types.js';

export function summarizeCart(items: CartItem[]): { subtotal: number; count: number } {
  return {
    subtotal: calculateSubtotal(items),
    count: items.reduce((sum, item) => sum + item.quantity, 0),
  };
}
