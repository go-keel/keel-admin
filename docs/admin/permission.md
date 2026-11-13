<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Business integration guide: adding a business module

Read this before adding a business module to the admin: permission codes, data scope, registration, options endpoints,
frontend gating.

This is the playbook for adding a management module (permission codes + data scope + CRUD) to the admin. It supplements
`docs/guides/api-endpoint.md`: that guide covers the layers and the endpoint itself; this page adds only the permission
layer. The reasons behind the mechanism are in `docs/admin/permission-model/README.md`; this page only says how.

The code below uses a fictional "article" module: the resource `Article` (articles, isolated by department / owner) and
`ArticleCategory` (categories, granted per instance by explicit assignment), in the business domain `cms`. The
repository may not contain it; replace the names with your own resource. `<pkg>` is the proto package directory under
`api/`, `<module>` is the module path in this repository's `go.mod`, `<app>` is the entry directory name.

Read first: `api-endpoint.md`, `domain-model.md`, `errors.md`, `assembly.md` and `testing.md` in `docs/guides/`, and
their supplements of the same name in `docs/admin/`.

## 0. First decide whether the module needs data scope

Not every module needs isolation by person or department. Decide first (the constraints behind it:
`permission-model/data-scope.md` §5.5):

- Global data (system settings, dictionaries) → no scope; permission codes alone gate the entry.
- Purely personal data (drafts, favorites, no department ownership) → use scope but declare only `AnchorSelf`; do not
  create an `owner_dept_id` column.
- Data granted per instance set (by store / project / category, with few and stable instances) → register the grant
  dimension as an explicit assignment resource (see step 2, "About `ListGrantable`"); the data rows carry a foreign key
  column gated through `Cols{PK}` (step 4).
- Ownership is neither a department nor an instance set (per-person point exceptions, algorithmic visibility) → do not
  use the kernel scope; the business filters in its own repo, or builds its own `perm.ScopeResolver` (contract in
  `permission-model/data-scope.md` §5.6), with permission codes gating the entry.
- Regular business data isolated by person / department (the articles on this page) → follow all the steps below.

## 1. proto: the RPC and its HTTP annotation

```proto
// api/<pkg>/v1/article.proto (fictional)
service ArticleService {
  rpc ListArticle(ListArticleRequest) returns (ListArticleReply) {
    option (google.api.http) = { get: "/v1/articles" };
  }
  rpc CreateArticle(CreateArticleRequest) returns (Article) {
    option (google.api.http) = { post: "/v1/articles" body: "*" };
  }
  rpc DeleteArticle(DeleteArticleRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/articles/{id}" };
  }
}
```

The shape of a list endpoint (paging, filter fields, sort enum, list row type) follows `docs/guides/api-endpoint.md`
"List endpoints are paged by default". Then run `make proto`; it also yields compile-time constants such as
`v1.OperationArticleServiceListArticle` (used for binding in step 5). Without an anonymous requirement, leave the
`(keel.v1.auth)` annotation alone: login is required by default, and every unbound endpoint returns 403 (deny by
default, see `permission-model/function-permissions.md` §4.2).

## 2. biz: code constants, groups, Permission metadata, resource type constants

```go
// internal/biz/<domain>.go (the fictional article module)
package biz

import "<module>/internal/core/perm"

const (
    ResArticle         perm.Resource = "article"          // resource_type used by data scope
    ResArticleCategory perm.Resource = "article_category" // explicit assignment resource
)

// A code has exactly three segments: <business domain>:<module>:<action>. A business code must not use the
// business domain "sys": sys is reserved for the kernel batch, and Seal rejects a business module that registers it.
const (
    PermArticleView   perm.Code = "cms:article:view"
    PermArticleCreate perm.Code = "cms:article:create"
    PermArticleDelete perm.Code = "cms:article:delete"
)

// Groups. The first module in business domain cms declares the domain entry; any other module joining cms declares
// only its own Module entry (declaring the same group twice panics). There is no display name field: the display
// name is derived from the slug as an i18n key, with entries added in internal/i18n.
var ArticleGroups = []perm.Group{
    {Domain: "cms", Sort: 10},
    {Domain: "cms", Module: "article"},
}

// create/delete both depend on view (here a prerequisite code in the same module; dependencies across modules and
// business domains are allowed too; Effective() expands the closure automatically).
// There is no Module field: business domain / module are derived from the code segments (perm.Code.Domain()/Module());
// and no Name field: the display name is derived from the code as an i18n key (perm.Code.NameKey()).
var ArticlePerms = []perm.Permission{
    {Code: PermArticleView},
    {Code: PermArticleCreate, Deps: []perm.Code{PermArticleView}},
    {Code: PermArticleDelete, Deps: []perm.Code{PermArticleView}},
}

// Articles are an anchored resource (the table carries owner_user_id/owner_dept_id directly), so both anchors are
// declared; categories are an explicit assignment resource (Anchors=0 + ListGrantable, see below). NameKey is the
// i18n message ID of the display name. A function rather than a package variable: the closures capture the repo, so
// di.go calls it after constructing the repo.
func ArticleResourceDefs(repo ArticleRepo) []perm.ResourceDef {
    return []perm.ResourceDef{
        {Res: ResArticle, NameKey: "resource.article", Anchors: perm.AnchorSelf | perm.AnchorDept,
            DeptOccupancy: func(ctx context.Context, deptID int64) (bool, error) {
                return repo.HasInDept(ctx, deptID)
            }},
        {Res: ResArticleCategory, NameKey: "resource.article_category",
            ListGrantable: func(ctx context.Context) ([]perm.GrantOption, error) {
                cats, err := repo.AllCategories(ctx)
                // ... map to []perm.GrantOption{ID, Label}
            }},
    }
}
```

Naming (`permission-model/function-permissions.md` §4.1): constants are named `Perm<Domain><Action>`, code values are
`<business domain>:<module>:<action>`, and `Seal()` checks the format with a regular expression, so a malformed code
fails at startup.

**Three segments are mandatory**, even with only one level of grouping: write the module segment
(`cms:article:view`, not `article:view`; why: `permission-model/function-permissions.md` §4.1).

**About Group**: business domains and modules **are only groups, not permissions**: they cannot be granted, the
registry has no Permission for them, and on the grant screen they serve only as expandable containers and select-all
entries. They need only a slug and a sort value, registered through `perm.MustRegisterGroups` (step 7). `Seal()`
requires the business domain and module of every code to be declared as groups; a missing declaration is a startup
error rather than a bare slug silently shown on screen.

**About display names**: none of the three display name levels on the permission tree (business domain / module /
action) **is written in the declaration**. They are derived from the slug as i18n keys, and `ListPermissions`
translates them on the spot by the request's `Accept-Language`, the same server-side localization as resource names
and audit action names:

| Declaration | i18n key |
|---|---|
| `{Domain: "cms"}` | `perm.cms` |
| `{Domain: "cms", Module: "article"}` | `perm.cms.article` |
| `{Code: "cms:article:view"}` | `perm.cms.article.view` |

When adding a code or a group, add one entry each to `internal/i18n/app.en.toml` and `app.zh-CN.toml` (kernel codes
live in `sys.*.toml`, which the business area does not use). **Quote the whole key** (`["perm.cms.article"]`): written
bare, TOML parses it as a subtable of the previous message, and go-i18n rejects the whole file. A missing entry for a
kernel code is named by `TestPermissionRegistryHasLocalizedNames` in `cmd/<app>/registration_test.go`; business codes
are not checked and fall back to English, so a derived repo that wants a guarantee adds its own check.

`Group.Sort` is an ascending value among siblings (compared separately among business domains and among the modules of
one business domain); ties break by display name, then slug, stably. Business extensions use ordinary small values such
as `0, 10, 20`; do not take `1_000_000_000`, which is reserved for the kernel domain `sys`
(`permission-model/function-permissions.md` §4.1.1).

The kernel orders the actions within a module itself, not by the declaration order of the `Perms` slice
(`permission-model/function-permissions.md` §4.3.2). Standard CRUD actions use `view/create/update/delete`; do not add
numeric prefixes to codes or reorder the slice to adjust the screen.

**Do not write `Permission.RequireOTP` by hand**: it is a display flag that `Seal()` derives from the bindings
(`Requirement.RequireOTP` in step 5 is what decides), and Seal rejects a hand-written one.

**About `NameKey`**: the resource's display name on the data scope screen, as an i18n message ID (not a literal name).
`ListResources` translates it on the spot by the request's `Accept-Language` and returns it as `name`; the business
only adds the entry in `internal/i18n/app.*.toml`. Left empty, the screen falls back to the slug. The friendly names of
the scope types (SELF/ALL_SUB...) are a fixed frontend enum, already in the kernel texts.

**About `ListGrantable`**: declares that the resource can be granted by **explicit assignment** (`RESOURCE_GRANT`) and
supplies the candidate instances that can be granted. `RESOURCE_GRANT` means "the role is explicitly granted a set of
instances of this resource" (role × resource type × set of instance ids, stored in `sys_role_resource_grant`, ticked
on the role management screen and fully replaced in the same transaction as the scope policy). **It can be configured
only when declared**:

- The callback signature is `func(ctx) ([]perm.GrantOption, error)`, with `GrantOption{ID, Label}`. The instances are
  runtime business data (categories / brand groups / stores) that the kernel cannot enumerate, so the closure holds the
  business's own repo; Label is the display name from the business data, not an i18n entry.
- Only when declared does the configuration screen enable `RESOURCE_GRANT`; **left empty, `RESOURCE_GRANT` cannot be
  configured for the resource at all** (disabled in the frontend, rejected by the server): without candidates there is
  nothing to grant.
- The server checks that the instance set is non-empty and every id is among the candidates (an empty set is a dead
  policy that always denies all, so it is rejected).
- **Scope of use**: few and stable instances (groups / categories). The callback has no paging, and the candidates have
  a **hard limit of 100** (`perm.MaxGrantOptions`; above it, configuration and validation fail outright instead of
  truncating). **Candidates nearing the limit mean the resource's design needs another look**: switch to an ownership
  column (owner column) or a business-built `ScopeResolver`; do not work around the limit. Massive instance sets
  (customers / orders) should never be registered as explicit assignment resources in the first place.
- **Cleanup contract**: after the business deletes a grantable instance, it calls the kernel port `perm.GrantRevoker`
  (injected the same way as `ScopeResolver`; the kernel's `ScopeUseCase` implements both) to remove every role's grant
  rows for it. Only the kernel reads and writes the `sys_*` tables; the business requests cleanup and does not delete
  directly. The two steps span databases with no shared transaction: delete the instance first, then revoke the grants
  (see `docs/admin/domain-model.md` "No transactions across databases").

**About `Anchors`**: declares which scope types the resource supports in the screen's drop-down. `AnchorSelf` ↔ the
table has `owner_user_id` (SELF can be configured); `AnchorDept` ↔ the table has `owner_dept_id` (DEPT/ALL_SUB can be
configured; ALL is always available). The anchors must match the columns created in step 6, otherwise `CreateRole` /
`UpdateRole` refuse to configure a policy that always denies all. An explicit assignment resource declares no anchors
(`Anchors=0`), so its screen offers only explicit assignment and ALL.

**About `DeptOccupancy`**: lets department deletion see this resource's rows. Only empty departments can be deleted,
and a department holding rows of a resource that declares it counts as not empty; a resource without it is not checked
(`permission-model/management-boundary.md` §6).

**Derived resources (Via)**: when a resource has no ownership column of its own and its ownership comes through a
parent resource (an "order" through its "merchant"), declare
`ResourceDef{Res: ResOrder, Via: &perm.Derive{Parent: ResMerchant, FK: "merchant_id"}}`.
**Via is currently consumed only during `Seal()` validation (it checks that the parent resource exists and allows only
one level); it does not build a subquery on the query side for you.** The subquery (or a set of visible parent IDs
sent across systems) is written by the business in its own repo. Before choosing it, read
`permission-model/data-scope.md` §5.1 and §11 S10.

## 3. biz: DO, repo interface, use case (inject ScopeResolver)

```go
// internal/biz/<domain>.go (continued)
type ArticleRepo interface {
    // spec filters the article's own rows (owner columns); catSpec is the grant scope of the "category" resource,
    // applied to the category_id foreign key column (an explicit assignment gate, see the data layer).
    List(ctx context.Context, spec, catSpec perm.ScopeSpec, opts ListOptions) ([]*Article, int64, error)
    Create(ctx context.Context, a *Article) error
    // Delete returns the number of rows actually deleted. 0 rows (the id does not exist, or exists but is out of
    // scope) becomes ErrArticleNotFound in the use case; the two cases are deliberately indistinguishable (existence
    // is not leaked).
    Delete(ctx context.Context, spec, catSpec perm.ScopeSpec, id int64) (int64, error)
    // ...Get, category CRUD, AllCategories, HasInDept
}

// scope/revoker are injected as core/perm interfaces (the kernel implements both with ScopeUseCase);
// the business area depends only on core/perm interfaces and does not import core/sys.
type ArticleUseCase struct {
    repo    ArticleRepo
    scope   perm.ScopeResolver
    revoker perm.GrantRevoker // clears explicit assignment rows after a category is deleted (never write sys_* tables)
}

// Two independent resolutions: the row scope (ResArticle) and the category grant (ResArticleCategory), intersected
// (AND) in the data layer.
func (uc *ArticleUseCase) List(ctx context.Context, actor *perm.Principal, opts ListOptions) ([]*Article, int64, error) {
    spec, err := uc.scope.Resolve(ctx, actor, ResArticle)
    if err != nil {
        return nil, 0, err
    }
    catSpec, err := uc.scope.Resolve(ctx, actor, ResArticleCategory)
    if err != nil {
        return nil, 0, err
    }
    if opts.Limit <= 0 {
        opts.Limit = defaultArticlePageSize // the max is only the proto lte; fill only the default here
    }
    return uc.repo.List(ctx, spec, catSpec, opts)
}

// Create takes ownership from actor; the signature does not accept ownership fields from the client at all.
func (uc *ArticleUseCase) Create(ctx context.Context, actor *perm.Principal, title string, categoryID int64) (*Article, error) {
    a := &Article{Title: title, CategoryID: categoryID, OwnerUserID: actor.UserID, OwnerDeptID: actor.DeptID}
    if err := uc.repo.Create(ctx, a); err != nil {
        return nil, err
    }
    return a, nil
}
```

Key points (the constraints in `permission-model/data-scope.md` §5.5):

- Every query entry first calls `uc.scope.Resolve(ctx, actor, ResArticle)` and passes `spec` to the repo; the repo does
  not touch the `Principal` itself.
- Ownership on write (`OwnerUserID`/`OwnerDeptID`) is **filled by the server from `actor`**, and the function signature
  accepts no ownership fields from the client. It is not "validated, then ignored": the client never gets this input
  at all.
- "The id does not exist" and "it exists but is out of scope" return the same domain error (404 semantics, not 403).

## 4. data: repo implementation (ApplyScope on every path)

**List/Get/Update/Delete all apply `ApplyScope`.** This is the easiest step to miss: filtering only List while
Get/Update/Delete fetch by id directly leaves a back door around the scope control (`permission-model/data-scope.md`
§5.5, "filter on every path"). The category grant (catSpec) is applied to the foreign key column through
`Cols{PK: "category_id"}` and ANDed with the row scope:

```go
// internal/data/<domain>.go (fictional)
// catCols: the instance set of the category spec filters not this table's primary key but the category_id foreign
// key. The same grant is a row filter on the category table (default id column) and a gate on the article table.
var catCols = permdata.Cols{PK: "category_id"}

func (r *articleRepo) List(ctx context.Context, spec, catSpec perm.ScopeSpec, opts biz.ListOptions) ([]*biz.Article, int64, error) {
    q := r.data.DB.WithContext(ctx).Model(&article{}).
        Scopes(permdata.ApplyScope(spec), permdata.ApplyScopeWith(catSpec, catCols))
    // ... filter conditions go on the same q, then Count + Offset/Limit/Find
}

// Delete: RowsAffected==0 covers both "the id does not exist" and "the id is out of scope".
func (r *articleRepo) Delete(ctx context.Context, spec, catSpec perm.ScopeSpec, id int64) (int64, error) {
    tx := r.data.DB.WithContext(ctx).
        Scopes(permdata.ApplyScope(spec), permdata.ApplyScopeWith(catSpec, catCols)).
        Where("id = ?", id).Delete(&article{})
    return tx.RowsAffected, tx.Error
}
```

`owner_user_id`/`owner_dept_id` are the default ownership columns of `ApplyScope`; with different column names use
`permdata.ApplyScopeWith(spec, permdata.Cols{OwnerID: "salesman_id", DeptID: "org_id"})`.
The category table's own list uses `ApplyScope(catSpec)` directly: ResourceIDs apply to its primary key column, the
second way of consuming the same grant. Build WHERE clauses only on this table's columns; do not JOIN the `sys_*`
tables across databases (`docs/admin/domain-model.md`).

## 5. service: transport adapter and permission binding

The service only takes the `Principal` out of ctx and passes it to the use case (`actorFromContext` in
`internal/service/errors.go`); the decision is made in the authz middleware, not in the service. The operation → code
bindings **go in a separate file**:

```go
// internal/service/<domain>_perm.go (fictional)
var ArticleBindings = perm.Bindings{
    v1.OperationArticleServiceListArticle:  {Codes: []perm.Code{biz.PermArticleView}},
    v1.OperationArticleServiceCreateArticle: {Codes: []perm.Code{biz.PermArticleCreate}, Audit: true},
    v1.OperationArticleServiceDeleteArticle: {Codes: []perm.Code{biz.PermArticleDelete}, Audit: true},
}
```

Both sides (operation constants and code constants) are compile-time constants: when an RPC is renamed or a code
deleted, the reference fails to compile, so a "binding to a dangling operation" cannot surface only at runtime.
**A missing binding is not "allowed by default"**: an endpoint with neither a binding nor `allow_anonymous` fails
startup (`perm.operation-unowned`), and at run time an unbound operation is 403 for every logged-in user (deny by
default).

Write operations add `Audit: true`, which records `sys_audit_log` automatically after success (see
`permission-model/sessions-and-audit.md` §9); read operations do not. With `Audit: true`, also register the action
display name `audit.act.<Service>.<Method>` in `internal/i18n/app.en.toml` and `app.zh-CN.toml` (a new Service also
needs the target type name `audit.tt.<Target>`). `TestAuditedOperationsHaveLocalizedNames` checks only the kernel's
entries: a missing business entry is not reported and the audit list falls back to the raw `Service.Method`, so a
derived repo that wants a guarantee adds its own check.

The target ID that the audit records is taken automatically from the request's `id` field. When the target is
identified only by several fields (no single integer primary key), give the request type an `AuditTarget() string`
method, in a `.go` file of your own under `api/<pkg>/v1/` (`make proto` does not touch it); the trade-off is in
`permission-model/sessions-and-audit.md` §9.

### Options endpoints (`*Options`): steps to add one

Option sets for pickers / filters **do not call the management endpoints directly**; add `List<X>Options` to the
resource's own proto:

1. Carry only identifying fields (id + display name), no management fields: extra fields get picked up by the next
   person to prefill a form, breaking the rule "form initial values are fetched from the detail endpoint". When the
   count keeps growing with the business, fetch in batches by keyword (shape in `docs/guides/api-endpoint.md` "List
   endpoints are paged by default").
2. Place the route **before** routes shaped like `{id}`; otherwise `options` is matched as an id.
3. In `<domain>_perm.go`, bind the code at **the lowest gate of the consuming page**; do not reuse the management
   endpoint's code for convenience.

Two uses of the same data ("which roles exist in the system" and "which roles can I assign") are two endpoints; do not
merge them. The decision and the observed counterexamples are in
`docs/adr/admin-0008-option-endpoints-and-page-gates.md`.

## 6. Schema: ownership columns and indexes

```sql
-- A fragment of the business complete schema (fictional)
CREATE TABLE IF NOT EXISTS article (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    title         VARCHAR(128)    NOT NULL,
    category_id   BIGINT UNSIGNED NOT NULL,
    owner_user_id BIGINT UNSIGNED NOT NULL,      -- data scope: self
    owner_dept_id BIGINT UNSIGNED NOT NULL,      -- data scope: own department / subtree
    created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY idx_category_id (category_id),
    KEY idx_owner_user_id (owner_user_id),
    KEY idx_owner_dept_id (owner_dept_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;
```

The names `owner_user_id`/`owner_dept_id` match the anchors (the `Anchors` declaration in step 2); with different
column names use `ApplyScopeWith` instead of forcing a rename. Where the script goes and how it is organized: see
`docs/admin/domain-model.md`.

## 7. di.go: register permissions and wire dependencies

```go
// cmd/<app>/di.go (excerpt; business area packages share names with core/sys, so by convention they are imported
// as nbiz/ndata/nsvc)
articleRepo := ndata.NewArticleRepo(bizData)
// scopeUseCase implements both perm.ScopeResolver and perm.GrantRevoker, so it is injected twice
articleUseCase := nbiz.NewArticleUseCase(articleRepo, scopeUseCase, scopeUseCase)
articleService := nsvc.NewArticleService(articleUseCase, refRepo)

registerPermissions(articleRepo) // ← pass the repos the new module's ResourceDefs need
if err := perm.Seal(perm.ContractOperations()); err != nil {
    return nil, cleanup, err
}

httpRegisters = append(httpRegisters, nsvc.ArticleHTTP(articleService)) // route mounting, separate from permission registration

// registerPermissions, further down in di.go: the only place a module registers its permissions
func registerPermissions(articleRepo nbiz.ArticleRepo) {
    registerKernelPermissions()                  // kernel batch, already present, leave it alone
    perm.MustRegisterGroups(nbiz.ArticleGroups)  // ← a new business module adds this line
    perm.MustRegister(nbiz.ArticlePerms, nsvc.ArticleBindings, nbiz.ArticleResourceDefs(articleRepo)) // ← and this one
}
```

- **Groups and permissions are registered separately**: groups are only organization, and several business modules can
  hang their own modules under the same business domain. A missing group registration → `Seal()` fails, and startup
  fails.
- **Business modules use `perm.MustRegister`, not `perm.MustRegisterKernel`**: the latter is reserved for the kernel
  batch, and a business code with the `sys:` prefix fails startup. `Seal()`
  runs once, after all registration, with `perm.ContractOperations()`: an endpoint of the proto with neither a
  binding nor `allow_anonymous` fails startup (`perm.operation-unowned`), so a module left unregistered is found
  before it answers 403 to everyone.
- `cmd/<app>/registration_test.go` seals the same `registerPermissions` (with nil repos), so a new module needs no
  change there.
- The other assembly rules (two databases, HTTP registration appended after the kernel's) are in
  `docs/admin/assembly.md`.

## 8. Domain errors (if needed)

Four changes in one commit: the sentinel in `internal/biz/errors.go`, the reason in `error_reason.proto` (business
numbers start at 1000), the mapping in `RegisterErrors` in `internal/service/errors.go`, and the text in
`internal/i18n/app.*.toml`. Details in `docs/admin/errors.md`.

## 9. Frontend: menu, permission filtering, pages

**A permission code is written in one place only: the route's `staticData`.** Menu visibility and the 403 guard both
read it there; the menu list `web/src/nav.ts` lists only paths and icons and holds no second copy of the codes. The
check functions are in `web/src/lib/perm.ts` (`has`) and `web/src/lib/route-meta.ts` (`canEnter`); a new business module
**does not change these two files**:

- **Gate by code**: put the permission code in the route's `staticData.perm`; no code means public.
- **root-only**: `staticData.root: true` (matching `RootOnly` in the backend Bindings), as on the kernel's role
  management page.
- **Sub-views**: each sub-view that can be navigated to directly is its own route, with its code in its own
  `staticData`, stacked on top of the page's. The menu, the tab bar (`web/src/lib/sub-views.ts`) and the guard check
  layer by layer, and every layer must pass for entry.
- **Element level**: render conditionally with `usePerm()(code)` inside the component.

```tsx
// web/src/routes/_app/<domain>/index.tsx (fictional)
export const Route = createFileRoute('/_app/articles/')({
  staticData: { titleKey: 'app:articles.title', perm: 'cms:article:view' },
  ...articleList.route,
  component: ArticlesPage,
});

// button-level gating inside the page
const can = usePerm();
{can('cms:article:create') && <Button onClick={() => setCreateOpen(true)}>{t('articles.create')}</Button>}
```

`has` is based on the profile's `perms` (`isRoot` short-circuits to true, matching the backend `perm.Principal`
semantics). The 403 guard runs centrally in the authentication shell's `beforeLoad`; every layer the target URL hits
must pass, and if entry is not allowed it redirects to `/403`, so not a single loader request is sent. Frontend gating
is only a UX improvement (hiding entries); the backend authz middleware is the security boundary. The frontend takes
codes as `PermCode`, a union generated from the backend registration into `web/src/generated/perm-code.ts`
(`make perms`, also run by `make check-generated`): a code the backend does not register fails `tsc`. After adding or
renaming a code, run `make perms`.

### Page gate audit

**Requests sent when a page loads must not have a higher gate than the page's own entry gate** (decision in
`docs/adr/admin-0008-option-endpoints-and-page-gates.md`). How to audit: for each page (each sub-view counts as one),
list its "entry gate" (the `perm` / `root` of every route layer's `staticData` combined) and "all requests sent on
load", and compare them one by one with the bindings in `<domain>_perm.go`. Any request whose gate is stricter than
the entry gate is a mismatch. Fix it in one of two ways, by the nature of the data:

- **Data the page needs to render normally** → lower the gate (switch to the `*Options` tier).
- **Data that serves only a conditionally rendered element** → make the request follow that element's permission check
  (`can(...)` from `usePerm()`).

Run this comparison after changing a page's data sources or adjusting bindings.

### step-up: the frontend collects the code in the form

For operations that need step-up (those with `RequireOTP` in the bindings), the frontend **collects the code in the
form up front**, instead of "hit `OTP_STEPUP` first, then pop up a dialog and replay":

- An operation with other inputs → embed an OTP field in the form, submitted together with the other inputs.
- An operation with no other inputs (resetting someone else's OTP, deleting a role) → confirm and collect the code in
  one dialog.
- No authenticator bound → render guidance in place (go to the profile page to bind one), rather than letting the user
  fill everything in and then hit a wall.

Every request goes through `stepUp(otpCode)` in `web/src/lib/api.ts`, spread into the request init:
`api.POST(path, { body, ...stepUp(code) })`. **`X-OTP-Code` appears only in that one function in the whole frontend;
do not fall back to building headers yourself**: then a declared but forgotten `otpCode` goes unnoticed, because
TypeScript does not report unused parameters, and the code is never sent.

`OTP_STEPUP` / `OTP_INVALID` / `OTP_BIND_REQUIRED` can still come back from the server (it is the boundary and does not
trust the frontend); show them as an ordinary error toast. **A form has at most one step-up point**; the reason is in
`docs/adr/admin-0007-operation-split-and-step-up-placement.md`.

## 10. Testing

Assert, for every scope a role can be configured with, the visible set; with no policy, deny-all; Get/Update/Delete of
an id out of scope, the 404 domain error. In two layers:

- **The OR merge / deny-all / full-access short-circuit semantics of `ApplyScope` itself** are shared kernel behaviour,
  already covered in `internal/core/permdata/scope_test.go`; a new module does not retest them.
- **The business module tests its own part**: the use case forwards the `spec` resolved by `scope.Resolve` to the repo,
  ownership on write comes from `actor` rather than the input, and `Delete` returns the "not found" domain error when
  affected=0; the data layer asserts on real MySQL (`make integration`) that the WHERE built by the scope filter
  really drops rows out of scope. How to write them: `docs/admin/testing.md`.

## Checklist

| # | Step | Files |
|---|---|---|
| 1 | proto definition + `make proto` | `api/<pkg>/v1/<domain>.proto` |
| 2 | three-segment code constants + Groups + Perms + resource type constants | `internal/biz/<domain>.go` |
| 3 | DO + repo interface + use case (inject `perm.ScopeResolver`) | `internal/biz/<domain>.go` |
| 4 | GORM model (with owner columns) + repo implementation (`ApplyScope` on every path) | the `model.go` holding the PO + `internal/data/<domain>.go` |
| 5 | transport adapter + operation→code bindings | `internal/service/<domain>.go` + `internal/service/<domain>_perm.go` |
| 6 | table (owner_user_id/owner_dept_id + indexes) | business `0_schema.sql` (plus an incremental script once in production) |
| 7 | `perm.MustRegisterGroups(...)` + `perm.MustRegister(...)` in `registerPermissions` + dependency wiring + route mounting | `cmd/<app>/di.go` |
| 8 | the four domain error changes | see `docs/admin/errors.md` |
| 9 | codes in route `staticData` + `usePerm()` controls in pages + menu | `web/src/routes/_app/<domain>/`, `web/src/nav.ts` |
| 10 | scope assertion tests | `internal/biz/<domain>_test.go`, `internal/data/<domain>_test.go` |
| — | configure permission codes + scope on the role management screen (operations / root, no code) | — |

Mechanism details (the reasons, edge cases, the five scope semantics, ownership lifecycle, ARBAC, OTP, kernel
configuration, and more) are in `docs/admin/permission-model/`; its README says which page answers what.
