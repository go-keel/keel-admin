import { afterEach, describe, expect, it } from 'vitest';
import { clearPreview, getPreview, setPreview } from './preview';

afterEach(() => {
  localStorage.clear();
});

describe('preview state read and write', () => {
  it('reads back what was written', () => {
    setPreview({ ids: '1,2', names: ['Auditor'] });
    expect(getPreview()).toEqual({ ids: '1,2', names: ['Auditor'] });
  });

  it('is undefined when never written', () => {
    expect(getPreview()).toBeUndefined();
  });

  it('is undefined after clearing', () => {
    setPreview({ ids: '1', names: [] });
    clearPreview();
    expect(getPreview()).toBeUndefined();
  });

  it('treats a corrupted stored value as absent without throwing', () => {
    localStorage.setItem('keel:preview', '{ not json');
    expect(getPreview()).toBeUndefined();
  });

  it('empty ids count as off: an empty CSV in the header would reduce nothing', () => {
    localStorage.setItem(
      'keel:preview',
      JSON.stringify({ ids: '', names: [] }),
    );
    expect(getPreview()).toBeUndefined();
  });
});
