import type { DeepPartial } from '../types';
import type sysEnUS from './en-US';

/**
 * Example file: delete the whole file after forking. Shows "the kernel translated in part, the rest falls back to
 * English": only the login page, the most common action buttons and punctuation are translated; other keys (`nav`,
 * `table`, `users`, ...) are left out on purpose. After switching to Japanese most system pages stay English, which is
 * the intended effect of this example, covered by i18next's `fallbackLng: 'en-US'`, not a missing translation. To
 * translate more, add keys below; `DeepPartial` checks spelling only, not completeness.
 */
const jaJP: DeepPartial<typeof sysEnUS> = {
  format: {
    listSeparator: '、',
    withDetail: '{{main}}（{{detail}}）',
    labeled: '{{label}}：',
  },
  login: {
    username: 'ユーザー名',
    usernameRequired: 'ユーザー名を入力してください',
    password: 'パスワード',
    passwordRequired: 'パスワードを入力してください',
    otp: 'ワンタイムコード',
    otpHint: '認証アプリを登録していない場合は空欄のままで構いません',
    remember: 'ログイン状態を保持する',
    submit: 'ログイン',
    submitting: 'ログイン中…',
    captcha: '画像認証',
    captchaRefresh: '画像を更新',
    captchaRequired: '先に認証を完了してください',
  },
  form: {
    showPassword: 'パスワードを表示',
    hidePassword: 'パスワードを非表示',
  },
  action: {
    cancel: 'キャンセル',
    confirm: '確定',
    save: '保存',
    edit: '編集',
    detail: '詳細',
    delete: '削除',
  },
  error: {
    retry: '再試行',
    loadFailed: 'データを読み込めませんでした。',
  },
};

export default jaJP;
