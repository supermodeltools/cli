import type { CartItem } from './types.js';

export function calculateSubtotal(items: CartItem[]): number {
  return items.reduce((sum, item) => sum + item.unitPrice * item.quantity, 0);
}

export function applyDiscount(total: number, percentage: number): number {
  return total * (1 - percentage / 100);
}
