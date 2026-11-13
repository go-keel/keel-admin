import { describe, expect, it } from 'vitest';
import { matchLanguage, resolveSupportedLocale } from './locale';

describe('language resolution: manual choice → browser language family → English fallback', () => {
  it('manual choice beats the browser (full code match)', () => {
    expect(resolveSupportedLocale('en-US', ['zh-CN'])).toBe('en-US');
    expect(resolveSupportedLocale('zh-CN', ['en-US'])).toBe('zh-CN');
  });

  it('falls back when the manual choice is a code no longer in the registry', () => {
    expect(resolveSupportedLocale('fr-FR', ['zh-TW'])).toBe('zh-CN');
  });

  it('matches by language family, not requiring the same region', () => {
    expect(resolveSupportedLocale(null, ['zh-HK'])).toBe('zh-CN');
    expect(resolveSupportedLocale(null, ['en-GB'])).toBe('en-US');
    expect(resolveSupportedLocale(null, ['zh'])).toBe('zh-CN');
  });

  it('takes the first supported one in order', () => {
    expect(resolveSupportedLocale(null, ['fr-FR', 'zh-CN', 'en-US'])).toBe(
      'zh-CN',
    );
  });

  it('falls back to English when none is supported', () => {
    expect(resolveSupportedLocale(null, ['fr-FR', 'ko-KR'])).toBe('en-US');
    expect(resolveSupportedLocale(null, [])).toBe('en-US');
  });
});

describe('matchLanguage: matching the language part against the registry', () => {
  // A registry built here checks "language part matching" without depending on which languages the real registry
  // currently holds.
  const registered = ['en-US', 'zh-CN', 'ja-JP'] as const;

  it('a full code match wins', () => {
    expect(matchLanguage('ja-JP', registered)).toBe('ja-JP');
  });

  it('matches on the language part before `-` (ja matches the registered ja-JP)', () => {
    expect(matchLanguage('ja', registered)).toBe('ja-JP');
    expect(matchLanguage('ja-JP-x-custom', registered)).toBe('ja-JP');
  });

  it('returns undefined when nothing matches', () => {
    expect(matchLanguage('ko-KR', registered)).toBeUndefined();
  });
});
