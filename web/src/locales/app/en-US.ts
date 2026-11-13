/** Business copy. Owned by the derived repo, not overwritten on upgrade. */
export default {
  // Title above the login form (used by web/src/login-layout.tsx).
  login: {
    title: 'Sign in',
    subtitle: 'Sign in to the management console',
  },
  // Example: delete after forking (the display name of the example time zone in web/src/timezones.ts)
  timezone: {
    beijing: 'Beijing Time',
  },
  nav: {
    // Example: delete after forking (ops and notices)
    ops: 'Operations',
    notices: 'Community announcements',
  },
  // Example: delete after forking (the whole notices object)
  notices: {
    title: 'Manage announcements',
    categories: 'Categories',
    notice: 'Title',
    category: 'Category',
    content: 'Content',
    keyword: 'Keyword',
    createdDate: 'Created on',
    owner: 'Author',
    ownerDept: 'Owning department',
    create: 'New announcement',
    pickCategory: 'Pick a category',
    ownerHint:
      'Author and owning department are filled in by the server from your account.',
    delete: 'Delete announcement',
    deleteHint: 'Delete “{{title}}”?',
    createCategory: 'New category',
    deleteCategory: 'Delete category',
    deleteCategoryHint:
      'Delete “{{name}}”? Categories still in use cannot be deleted.',
    export: 'Export',
    viewNotices: 'View announcements',
    batchDelete: 'Delete selected',
    batchDeleteN: 'Delete selected ({{count}})',
    batchDeleteHint:
      'Delete the {{count}} selected announcement(s)? They are deleted together: if any one cannot be deleted, none of them are.',
  },
  workbench: {
    title: 'Workbench',
    greeting: 'Hello, {{name}}',
    shortcuts: 'Shortcuts',
  },
} as const;
