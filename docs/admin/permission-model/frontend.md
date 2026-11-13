<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Frontend kernel responsibilities

Read this before gating frontend pages or controls by permission codes.

## 14. Frontend kernel responsibilities

This section fixes the mechanisms; UI details are in the other pages of `docs/admin/`.

The template's web core takes on at least: the login page (including OTP admission / the binding bootstrap flow), menus
and route guards filtered by permission codes, the standard OTP input component (`otp.md` §7.2), unified handling of
401/403/OTP_STEPUP errors, the role / account / department / audit management pages, and the favorites group in the
sidebar.

Favorites follow the account (`sys_user.settings`, delivered with `Profile`, written per item:
`POST /v1/sys/auth/favorites/{add,remove}`, available to anyone signed in, not audited, no step-up), and are filtered by
the same permission codes before rendering: favorites whose permission was lost disappear together with the menu,
leaving no dead links. What may go in and why writes are per item: the "User settings" section of `docs/admin/auth.md`.

**Binding permissions to the UI** (how to write it: `docs/admin/permission.md` §9):

- **Where the codes come from**: the profile (`/v1/sys/auth/profile`) carries the current user's effective permission
  code set (the backend has already expanded the dependency closure) + `isRoot`; the frontend refetches it when the tab
  regains focus, so a permission change takes effect then or on the next load (there is no live push).
- **One place per code**: the route's `staticData` declares it, and menu visibility and the route guard both read it
  there; a sub-view's code stacks on top of the page's. Elements inside a page check the same codes. `has`
  short-circuits to true on `isRoot`, so root sees business entries; root-only entries check `isRoot` itself.
- **Principle**: frontend permission control is only a **usability measure** (hiding entries), not a security boundary:
  the authz middleware is always the fallback. Frontend codes are typed `PermCode`, generated from the backend
  registration, so a code the backend does not register fails `tsc`.

The standard UI and kernel-level UI features are specified in the other pages of `docs/admin/` (`pages.md`,
`forms.md`, `tables.md`); this page fixes only the mechanism boundaries above.
