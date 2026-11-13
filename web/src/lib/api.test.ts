import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  applyRequestHeaders,
  dispatchError,
  performErrorAction,
  stepUp,
} from './api';
import { clearPreview, setPreview } from './preview';

const { toastError } = vi.hoisted(() => ({ toastError: vi.fn() }));
vi.mock('sonner', () => ({ toast: { error: toastError } }));

afterEach(() => {
  localStorage.clear();
  toastError.mockClear();
});

describe('dispatchError: only two rules', () => {
  it('the three SESSION_KILLED reasons go to login without a toast', () => {
    for (const reason of [
      'UNAUTHORIZED',
      'UNAUTHENTICATED',
      'SESSION_EXPIRED',
    ]) {
      expect(dispatchError({ reason, message: 'Session expired' })).toEqual({
        kind: 'login',
      });
    }
  });

  it('401s from the login flow and step-up are not session kills: decided by reason, not status code', () => {
    for (const reason of [
      'INVALID_CREDENTIAL',
      'CAPTCHA_INVALID',
      'OTP_STEPUP',
    ]) {
      expect(dispatchError({ reason, message: 'x' })).toEqual({
        kind: 'toast',
        text: 'x',
      });
    }
  });

  it('every other non-2xx toasts the server-localized message', () => {
    expect(
      dispatchError({
        reason: 'DEPT_NOT_EMPTY',
        message: 'Department is not empty',
      }),
    ).toEqual({ kind: 'toast', text: 'Department is not empty' });
  });

  it('falls back to the fallback text the caller gives when there is no body', () => {
    expect(dispatchError(undefined, 'Bad Gateway')).toEqual({
      kind: 'toast',
      text: 'Bad Gateway',
    });
  });

  it('does nothing without even a fallback text, never an empty toast', () => {
    expect(dispatchError(undefined)).toEqual({ kind: 'none' });
  });
});

describe('header injection', () => {
  it('always sends Accept-Language', () => {
    const headers = new Headers();
    applyRequestHeaders(headers);
    expect(headers.get('Accept-Language')).toBeTruthy();
  });

  it('sends X-Preview-Roles while previewing, not after leaving', () => {
    setPreview({ ids: '3,7', names: ['Operations'] });
    const on = new Headers();
    applyRequestHeaders(on);
    expect(on.get('X-Preview-Roles')).toBe('3,7');

    clearPreview();
    const off = new Headers();
    applyRequestHeaders(off);
    expect(off.get('X-Preview-Roles')).toBeNull();
  });

  it('never injects Authorization: the session token is in an httpOnly cookie', () => {
    const headers = new Headers();
    applyRequestHeaders(headers);
    expect(headers.get('Authorization')).toBeNull();
  });

  it('stepUp is the only entry point for X-OTP-Code', () => {
    expect(stepUp('123456')).toEqual({ headers: { 'X-OTP-Code': '123456' } });
  });
});

describe('performErrorAction: the same message does not stack', () => {
  it('the toast ID is the text itself: when page prefetch and component fetch both fail, one remains', () => {
    performErrorAction({ kind: 'toast', text: 'Invalid request' });
    performErrorAction({ kind: 'toast', text: 'Invalid request' });
    expect(toastError).toHaveBeenCalledTimes(2);
    for (const call of toastError.mock.calls) {
      expect(call).toEqual(['Invalid request', { id: 'Invalid request' }]);
    }
  });
});
