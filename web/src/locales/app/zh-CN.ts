import type { Translations } from '../types';
import type appEnUS from './en-US';

const zhCN: Translations<typeof appEnUS> = {
  login: {
    title: '登录',
    subtitle: '登录管理控制台',
  },
  // Example: delete after forking (display name of the sample time zone in web/src/timezones.ts)
  timezone: {
    beijing: '北京时间',
  },
  nav: {
    // Example: delete after forking (ops and notices)
    ops: '运营管理',
    notices: '社区公告',
  },
  // Example: delete after forking (the whole notices object)
  notices: {
    title: '公告管理',
    categories: '栏目管理',
    notice: '标题',
    category: '栏目',
    content: '内容',
    keyword: '关键词',
    createdDate: '创建日期',
    owner: '创建人',
    ownerDept: '归属部门',
    create: '新建公告',
    pickCategory: '请选择栏目',
    ownerHint: '归属部门与创建人由服务端按你的账号自动填写。',
    delete: '删除公告',
    deleteHint: '确定删除「{{title}}」？',
    createCategory: '新建栏目',
    deleteCategory: '删除栏目',
    deleteCategoryHint: '确定删除「{{name}}」？还被公告占用的栏目删不掉。',
    export: '导出',
    viewNotices: '查看公告',
    batchDelete: '批量删除',
    batchDeleteN: '批量删除 ({{count}})',
    batchDeleteHint:
      '确定删除选中的 {{count}} 条公告？这些公告会一起删除：只要有一条删不掉，就都不会删除。',
  },
  workbench: {
    title: '工作台',
    greeting: '你好，{{name}}',
    shortcuts: '常用功能',
  },
};

export default zhCN;
