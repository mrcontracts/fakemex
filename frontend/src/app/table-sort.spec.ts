import { describe, expect, it } from 'vitest';
import { sortTableRows } from './table-sort';

describe('sortTableRows', () => {
  it('sorts decorated numeric values numerically in either direction', () => {
    const rows = [['$1,200.50'], ['$90.25'], ['300']];

    expect(sortTableRows(rows, 0, 'asc').map((row) => row[0])).toEqual(['$90.25', '300', '$1,200.50']);
    expect(sortTableRows(rows, 0, 'desc').map((row) => row[0])).toEqual(['$1,200.50', '300', '$90.25']);
  });

  it('sorts text naturally and keeps equal values stable', () => {
    const rows = [
      ['order10', 'first'],
      ['order2', 'second'],
      ['order2', 'third'],
    ];

    expect(sortTableRows(rows, 0, 'asc')).toEqual([
      ['order2', 'second'],
      ['order2', 'third'],
      ['order10', 'first'],
    ]);
  });
});
