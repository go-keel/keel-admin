# Options endpoints are gated by purpose, and requests sent on page load are no stricter than the page's entry gate

<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

Two interlocking decisions:

1. The option sets of selectors / filters come from each domain's own `*Options` endpoint (`/v1/sys/depts/options`,
   `/v1/sys/roles/options`), not from the management endpoints; **the permission binding follows the purpose, not
   the module that owns the data**.
2. A request sent on page load **must not have a stricter gate than the page's own entry gate**.

## Why the binding follows the purpose

URLs are placed under the domain that owns the resource, but the gate should be decided by **who uses it and for
what**. On the user management page the department tree is a filter condition and the options for a user's
department; `sys:account:view` is enough for whoever needs it there. The `ListDept` group is department
**management**, and binding it to `sys:dept:view` is right.

The cost of mixing them: if the user management page called `/v1/sys/depts` directly, someone with only the account
view permission would get "You do not have permission to perform this operation" the moment they open the page, even
though that data is required for the page to render normally and is no overreach at all.

**This kind of 403 cannot be caught away in the frontend**: the global error handler always shows a toast, on purpose
(see the frontend's two rules in `docs/admin/errors.md`). So there are only two ways out: either the request should
not be sent at all, or the gate should be opened.

## Why two uses of the same data are split into two endpoints

The roles on the user management page are the ready-made example: the filter needs "which roles exist in the system"
(`ListRoleOptions`, bound to `account:view`); the assignment dropdown needs "which roles may I give to others"
(`ListAssignableRoleOptions`, bound to `account:update`, limited to the union of assignable roles).

Merging them necessarily breaks one side: use the assignable set as the filter, and a role clearly shown in a list row
cannot be filtered for; use the full set as the assignment dropdown, and a pick is rejected by the server's ARBAC.
**The correct sets for the two uses are simply not equal**; the endpoint saved is paid for with a bug that is certain
to appear.

## Why the gate must not be stricter than the entry

Being let into a page and then getting a 403 while it loads is the most damaging kind of mismatch: the user has done
nothing yet and is first told "You do not have permission to perform this operation". There are two fixes; pick one
by the nature of the data:

- **the data is required for the page to render normally** → open the gate (`listDepts` → `ListDeptOptions`: the
  department tree is a filter condition and the options for a user's department; without it the page is crippled);
- **the data serves only an element rendered under a condition** → the request follows that element's permission
  check (assignable roles are fetched only with `sys:account:update`: the role selector is rendered conditionally
  anyway, and fetching its data must follow it).

The steps (adding a new kind of option set, auditing page gates against each other) are in `docs/admin/permission.md`.
