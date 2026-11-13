import { describe, expect, it, vi } from 'vitest';
import { loadState, type QueryLike, retryFailed } from './load-state';

const q = (over: Partial<QueryLike>): QueryLike => ({
  data: undefined,
  isError: false,
  isPending: true,
  fetchStatus: 'fetching',
  refetch: vi.fn(),
  ...over,
});

const ready = q({ data: { items: [] }, isPending: false, fetchStatus: 'idle' });
const loading = q({});
const failed = q({ isError: true, isPending: false, fetchStatus: 'idle' });
const disabled = q({ fetchStatus: 'idle' });

describe('loadState', () => {
  it('ready only when all are ready; an empty list counts as data', () => {
    expect(loadState([ready, ready])).toBe('ready');
  });

  it('an empty list being refetched in the background stays ready, not back to the skeleton', () => {
    const refetching = q({
      data: [],
      isPending: false,
      fetchStatus: 'fetching',
    });
    expect(loadState([refetching])).toBe('ready');
  });

  it('any failure means failed, overriding loading', () => {
    expect(loadState([ready, loading, failed])).toBe('failed');
  });

  it('any in flight means loading', () => {
    expect(loadState([ready, loading])).toBe('loading');
  });

  it('a failed background refetch with data in hand stays ready: what is shown is not replaced by failure', () => {
    expect(loadState([q({ data: {}, isError: true, isPending: false })])).toBe(
      'ready',
    );
  });

  it('a disabled query does not hold the block back', () => {
    expect(loadState([ready, disabled])).toBe('ready');
  });
});

describe('retryFailed', () => {
  it('refetches only the failed pieces', () => {
    const a = q({ isError: true, isPending: false, fetchStatus: 'idle' });
    const b = q({ data: {}, isPending: false, fetchStatus: 'idle' });
    retryFailed([a, b]);
    expect(a.refetch).toHaveBeenCalledOnce();
    expect(b.refetch).not.toHaveBeenCalled();
  });
});
