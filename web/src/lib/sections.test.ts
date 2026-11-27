import { describe, expect, it } from 'vitest';
import { sectionAfterInvalidSubmit } from './sections';

describe('sectionAfterInvalidSubmit', () => {
  it('stays when the current section has an error', () => {
    expect(sectionAfterInvalidSubmit('b', ['a', 'b'])).toBe('b');
  });

  it('moves to the first section with an error', () => {
    expect(sectionAfterInvalidSubmit('a', ['c', 'd'])).toBe('c');
  });

  it('stays when no section has an error', () => {
    expect(sectionAfterInvalidSubmit('a', [])).toBe('a');
  });
});
