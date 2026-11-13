import type { Translations } from '../types';
import type appEnUS from './en-US';

// Example file: delete the whole file after forking. Japanese is the sample language shipped upstream; it shows
// "business copy fully translated and self-checked": like `zh-CN.ts` it declares `Translations<typeof appEnUS>`, so a
// missing key fails tsc. The kernel does not check whether this file is fully translated; the constraint is this
// file's own.
const jaJP: Translations<typeof appEnUS> = {
  login: {
    title: 'ログイン',
    subtitle: '管理コンソールにログイン',
  },
  // Example: delete after forking (display name of the sample time zone in web/src/timezones.ts)
  timezone: {
    beijing: '北京時間',
  },
  nav: {
    // Example: delete after forking
    ops: '運用管理',
    notices: 'コミュニティのお知らせ',
  },
  // Example: delete after forking (the whole notices object)
  notices: {
    title: 'お知らせ管理',
    categories: 'カテゴリー管理',
    notice: 'タイトル',
    category: 'カテゴリー',
    content: '内容',
    keyword: 'キーワード',
    createdDate: '作成日',
    owner: '作成者',
    ownerDept: '所属部署',
    create: 'お知らせを作成',
    pickCategory: 'カテゴリーを選択',
    ownerHint: '作成者と所属部署はアカウント情報から自動的に入力されます。',
    delete: 'お知らせを削除',
    deleteHint: '「{{title}}」を削除しますか？',
    createCategory: 'カテゴリーを作成',
    deleteCategory: 'カテゴリーを削除',
    deleteCategoryHint:
      '「{{name}}」を削除しますか？使用中のカテゴリーは削除できません。',
    export: 'エクスポート',
    viewNotices: 'お知らせを表示',
    batchDelete: '選択項目を削除',
    batchDeleteN: '選択項目を削除（{{count}} 件）',
    batchDeleteHint:
      '選択した {{count}} 件のお知らせを削除しますか？まとめて削除され、1 件でも削除できない場合は、どれも削除されません。',
  },
  workbench: {
    title: 'ワークベンチ',
    greeting: 'こんにちは、{{name}} さん',
    shortcuts: 'よく使う機能',
  },
};

export default jaJP;
