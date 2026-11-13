# Operations are split along permission lines, and a form has at most one step-up point

<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

Two decisions used together:

1. Operations are split or merged along **permission** lines, not by how many endpoints the backend has: what the same
   people can change under the same gate (permission code + step-up or not) is merged into one form submitted once;
   only what needs different permissions is split.
2. **A form has at most one step-up point.**

## Why split and merge by permission

The criterion is "is it the same UX action", not "how many endpoints the backend has", nor "are the permission codes
exactly the same". Split by endpoint, and the user opens three dialogs and enters three codes for what is one action
in their head; split by permission, and the number of dialogs equals "how many different things you are allowed to
do". Three ready-made examples:

- **Role management = merge.** Basic info / permissions / assignable roles / manageable departments / data scope have
  exactly the same gate (`RootOnly + RequireOTP + Audit`); splitting them into several dialogs has no basis in
  permissions, so they are merged into one `UpdateRole`: one form, one code, one submission.
- **User management "Edit" = merge.** Nickname / avatar / department / roles / IP allowlist are one action, "edit this
  user", just as "create user" is one form filled in once. So they are merged into one `UpdateUser`, and **assigning a
  department / assigning roles / setting IPs get no permission codes of their own**: whoever has `update` can change
  them, and how far is enforced by ARBAC and validation in biz (the target is manageable, the department is within
  the manageable range, the roles are within the assignable range, the IPs are valid), the same as setting them at
  creation. Each field is written only if it really differs from the current value.
- **User management "Actions" = split.** Reset password / reset OTP / enable-disable / force logout are each one-off
  actions, not "editing the profile", and stay in the "Actions" dropdown as separate operations. Granting / revoking
  super administrator has a different gate from editing (root only); it is likewise an item under "Actions" and does
  not go into the edit form.

Every write in user management requires step-up: create / edit / disable / unlock / force logout / reset password /
reset OTP / set root all change someone else's account, and any of them can affect another person's login and
permissions, so all are bound to `RequireOTP`.

## Why a form can have only one step-up point

A step-up code has a **use limit** (`sys.otp.max_uses`, default 3). "Collect one
code + call N step-up endpoints in a row" hits `OTP_ALREADY_USED` once over the limit.

Do not read the limit as "then three points are fine", and certainly do not raise it to fit more:

- those extra uses are **bought with replay risk**: the same code can be submitted again within the 90s window,
  including by an attacker who intercepted it. They are slack for real hiccups like "doing two operations in a row
  before the authenticator rolls over", not a budget for form design.
- A deployment can set it back to 1, and a form with several points breaks on the spot. **Designing the frontend
  around the limit stakes frontend correctness on a config value.**

How the two examples satisfy this: `UpdateRole` by "merging into one endpoint, one step-up"; `UpdateUser` likewise
uses one step-up for the whole form, while "is root", which has a different gate, stays out of the edit form and is a
separate item under "Actions" that goes through `SetUserRoot` and collects its own code. Putting it in the same form
would make one form with two points.

The step-up component and calling convention in the frontend are in `docs/admin/permission.md`.
