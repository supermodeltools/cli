import assert from 'node:assert/strict';
import test from 'node:test';
import { renderQuote } from '../dist/src/api.js';
import { buildSalesReport } from '../dist/src/reports.js';

const items = [
  { sku: 'a', unitPrice: 10, quantity: 2 },
  { sku: 'b', unitPrice: 5, quantity: 1 },
];

test('renderQuote includes subtotal and tax-adjusted total', () => {
  assert.equal(renderQuote(items), '{"subtotal":25,"total":26.75}');
});

test('buildSalesReport includes item count and subtotal', () => {
  assert.equal(buildSalesReport(items), '3 items: $25.00');
});
