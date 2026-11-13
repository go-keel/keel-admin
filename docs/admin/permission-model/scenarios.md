<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Permission scenarios

Read this when a permission need does not obviously fit one dimension: each scenario shows which dimensions combine.

## 11. Scenarios

Each scenario is split into three parts: "configuration (data plane, done by operators) / code (written by the
business) / kernel behaviour".

### S1 Integrating a new business module end to end

Need: add management of "articles", editor roles can view and publish, data isolated by department.

- **Code**: the steps of `docs/admin/permission.md` (proto, three codes and their bindings in
  `service/<domain>_perm.go`, biz/data/service, a table with `owner_user_id`/`owner_dept_id`, `ApplyScope` on every
  path in the repo, two lines in di.go).
- **Configuration**: create the role "Article editor" → tick `cms:article:view`/`cms:article:create` → configure scope:
  resource_type=`article`, scope_type=`DEPT` → assign users to the role.
- **Kernel behaviour**: registration checks pass; a `ListArticle` request → authz checks `cms:article:view` → biz
  resolves ScopeSpec{DeptIDs:[user's department]} → the repo query automatically carries `dept_id IN (...)`.

### S2 Plain function grants (read-only vs read-write)

Need: support staff can only view articles; supervisors can publish and delete.

- **Configuration**: the "Support" role ticks `cms:article:view`; the "Supervisor" role ticks
  `cms:article:create`+`cms:article:delete` (view takes effect through the dependency closure and need not be ticked).
- **Code/kernel**: nothing. The `Deps` closure expands in `Effective()`; without the delete code, `DeleteArticle` is
  403.

### S3 Tiered data scope (self / department / subtree)

Need: salespeople see only the customers they created; department managers see the whole department; regional directors
see the whole subtree.

- **Configuration**: under the same resource_type=`customer`, the three roles get scope_type=`SELF` / `DEPT` /
  `ALL_SUB` respectively.
- **Code**: the customer table carries `owner_user_id`/`owner_dept_id`, the repo uses `ApplyScope` (once).
- **Kernel behaviour**: SELF → `owner_user_id=uid`; DEPT → `owner_dept_id IN {my department}`; ALL_SUB → the subtree's
  department set expanded from ancestors. Several roles held together take the union (OR).

### S4 Explicit assignment (RESOURCE_GRANT): granting groups/categories to roles

Need: articles are organized by category; the "Operations specialist" role can see and submit only to the "Operations
news" category, and the "R&D weekly" category is completely invisible to it. (Brand groups, store groups and channel
groups have the same shape.)

- **Code** (once): register the category as a resource (`Anchors=0` + a `ListGrantable` callback); the article table
  carries `category_id`; the repo AND-s two Scopes (`ApplyScope(row level) + ApplyScopeWith(category spec,
  Cols{PK: "category_id"})`); after deleting a category, call `perm.GrantRevoker` to clean up the grant rows.
- **Configuration** (by root): role "Operations specialist" → Data scope → resource "Article category" →
  scope_type=`RESOURCE_GRANT` → tick "Operations news". When a new category goes live, one tick makes it effective; role
  members inherit it automatically, with no per-person work.
- **Kernel behaviour**: resolves ResourceIDs=[Operations news] → the category list shows only it, the article list is
  gated by `category_id IN (...)`, and submitting to an ungranted category returns 404 (existence does not leak).
- **Per-user point exceptions** ("Zhang San follows up this one customer") **are outside the kernel's semantics**:
  normal ownership uses owner columns + handover (`data-scope.md` §5.4b); a business with a real per-person grant
  dimension builds its own `ScopeResolver` (`data-scope.md` §5.6). Policies stay configured on roles; this model makes
  no exception for exceptions.

### S5 Tiered administrators (ARBAC)

Need: the East China branch administrator may manage only accounts in the East China subtree, may assign subordinates
only the two roles "Support" and "Sales", and cannot create an account more powerful than themselves.

- **Configuration** (by root): role "East China admin" → manage_dept=[East China] (subtree included) →
  assignable=[Support, Sales]; grant the `sys:account:*` function codes.
- **Code/kernel**: no business code needed. On CreateUser/UpdateUser the kernel guards: target department ∉ East China
  subtree → `DEPT_OUT_OF_RANGE`; assigning the "Supervisor" role (not in assignable) → `ROLE_OUT_OF_RANGE`; assigning a
  sensitive role → rejected; acting on root or oneself → rejected. Anti-escalation is guaranteed by the assignable allow
  list.

### S6 Step-up OTP for sensitive operations

Need: resetting someone else's OTP is a high-risk back door, so even a signed-in user must verify a code again on the
spot.

- **Configuration**: none (`RequireOTP: true` is fixed in the binding; the `sys:account:reset_otp` code is granted to
  the roles that need it).
- **Code**: the business's own high-risk operations work the same way: add `RequireOTP: true` to the binding; the
  frontend explicitly embeds the standard OTP input component in that operation's confirmation dialog (`otp.md` §7.2),
  and the request carries the `X-OTP-Code` header.
- **Kernel behaviour**: header missing → 401 OTP_STEPUP; wrong code → 401 OTP_INVALID; replay (`otp.md` §7.3) →
  401 OTP_ALREADY_USED; a failure of the verification itself (guard unavailable, secret cannot be decrypted, query
  failed) fails closed with 500 UNEXPECTED_ERROR and a warn log on the server. root is stopped too.

### S7 root-only operations

Need: role management and permission assignment are allowed only for the super administrator.

- **Configuration**: none. The binding `RootOnly: true` is fixed in `core/sys/service/perm.go`.
- **Code**: if the business has functions of the same sensitivity (such as a global switch), write `RootOnly: true` in
  the binding.
- **Kernel behaviour**: non-root always gets 403 NOT_ROOT; no combination of permission codes gets around it.

### S8 Anonymous endpoints

Need: login and health checks need no token.

- **Code**: add `option (keel.v1.auth) = { allow_anonymous: true };` in the proto and rerun `make proto`. No Go code, no
  change to bindings.
- **Kernel behaviour**: Keel auth and admin authz read the same proto annotation and allow it. Forgetting the annotation
  leaves the endpoint with neither a binding nor the annotation, so startup fails with `perm.operation-unowned`.

### S9 Roles that force OTP (login admission)

Need: the finance role must bind OTP before it can sign in.

- **Configuration**: role "Finance" with `force_otp=1`.
- **Kernel behaviour**: a user with this role signs in → if not bound, gets a bind token and goes through the binding
  bootstrap (`otp.md` §7.1); after binding, signing in again always verifies OTP; any error in an OTP-related query
  rejects the login (fail-closed).

### S10 Managing data in an external business system

Need: build management pages in the admin for the order system (an independent business service): query orders,
close orders by hand.

- **Code**: follow `docs/admin/permission.md` as usual; the difference is in the data layer: the repo does not use the
  admin database but a business gRPC/HTTP client (`docs/guides/grpc-client.md`): `ListOrder` calls the order service's
  query endpoint, `CloseOrder` calls its close endpoint. **Do not connect to the order database directly** (`README.md`
  §1).
- **Division of permissions**: dimension one as usual (`order:view`/`order:close` codes control the entry, step-up and
  audit as usual); **dimension two uses the pass-down mode** (the unified ownership model in `data-scope.md` §5.1):
  orders are registered as a derived resource (Via merchant), the admin resolves the set of visible merchant IDs and
  passes it down with the call, and the order service filters by `merchant_id`; the business may also ignore what it
  receives and decide on its own.
- **How far data access goes** (`README.md` §1): **writes go through the business API first** (invariants guarded on the
  business side); **read-only queries may connect to the business database directly** (a replica or read-only account,
  normal practice for reports and lists); writing the business database directly is neither recommended nor forbidden by
  the template, and after the fork the business team owns the invariants.
- **Kernel behaviour**: authz and audit apply on the admin side as usual; the business invariants (can this order be
  closed) are guarded by the order service: the admin relays the operation and does not decide for the business.

### S11 Cross-system instance grants and fan-out

Need: one management console manages N independently deployed downstream system instances ("brands", each with its own
database and domain name), brands grouped by how they are operated; support staff are granted by brand group, and
cross-brand queries (user by phone number, order by number) call the downstreams concurrently over the granted set.
Requirements: grants hang on roles; **the same role's visible range is independent in each business domain** (the order
domain sees the self-operated group, the user domain only 3 of its brands); after a group is granted, new brands joining
it are included automatically.

- **Modelling**: a "business domain" maps to a **resource_type**: each domain that needs its own range registers one
  explicit assignment resource (such as `order_brands`, `user_brands`), each declaring `ListGrantable`. The same role
  gets one policy per resource and the ranges do not affect each other: that is the whole implementation of the "domain
  dimension". The candidates return brand groups; when "groups + single brands" must be mixed, the business unifies the
  candidate id space (such as one grant target table where groups and single brands are each a row): instance ids are
  opaque to the kernel, and how they are encoded is up to the business.
- **Configuration** (root): role "Support - self-operated" → `order_brands` ticks "Self-operated group"; `user_brands`
  ticks three brand groups. A new brand going live and joining a group needs **zero grant operations** (expansion
  happens at query time).
- **Consumption** (the three fan-out rules, `data-scope.md` §5.6): `Resolve` → `spec`; group→brand expansion is in the
  business (query `brand.group_id`, so group membership changes take effect automatically); `All=true` is
  **materialized** as all brands; **an empty set = call no downstream** (the fan-out side has no `WHERE 1=0` fallback).
  The expanded brand set is the target list for the concurrent calls; `spec.ResourceIDs` is itself a list, with no need
  to go back through SQL.
- **Local row filtering reuses the same grants**: the platform's own brand configuration page filters the brand table
  with `ApplyScope` (gated by group with `Cols{PK: "group_id"}`, or with brands registered as a separate resource).
- **Candidate limit** (`data-scope.md` §5.2): the total of groups + frequently granted single brands is bound by
  `MaxGrantOptions` (100): exceeding it means the resource type should be split or a custom resolver used, not that the
  limit should be raised.
- **Per-user point exceptions** (a support person temporarily helping with brand X), if really needed → layer them on
  with a custom resolver (`data-scope.md` §5.6), or have the business create a single-brand group.
