<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Login, sessions and user settings: what the kernel already does

Read this before touching login, the session cookie, anonymous endpoints, captcha, user settings or anything that
uses the client IP, together with `docs/guides/auth.md`.

The guide covers JWT authentication in general (configuration, the anonymous allowlist, reading the current user,
issuing tokens, checks after authentication); this page covers what the kernel of an admin built on keel-admin
**already does and must not be reinvented**. What you can do (permission codes, data scope, step-up) is in
`docs/admin/permission.md`.

## Sessions: settled facts

The whole chain of login, issuing, invalidation and logout lives in the kernel (`internal/core/sys/`):

- **A single token, no refresh token.** `sys.session_ttl` is a hard limit with no renewal: when it expires, the user
  logs in again, so it must be long enough to cover a whole working session. A refresh token would rotate on every
  use, and several browser tabs refreshing at once would sign each other out.
- **The token is delivered in an httpOnly cookie**, and the frontend never touches it: it cannot read it, and XSS
  cannot steal it. The cookie name comes from `server.http.default.auth.cookie_name`; issuing, clearing and the
  authentication middleware share that one name (how Keel reads a token from a cookie:
  `keel:docs/auth.md#token-from-a-cookie`). **An `Authorization` header still wins over the cookie** (the
  restricted token of the OTP binding flow relies on that and is sent in an explicit header). `SameSite=Strict`;
  `Secure` comes from `sys.cookie_secure` and is not sniffed from TLS (behind a reverse proxy, sniffing would turn
  it off silently).
- **Claims hold only `sub` / `iat` / `exp` (plus the token version); permissions never go into the token**; the
  `principal` middleware computes them on every request. The token lives long while permissions change at any time,
  so a copy in the token is a snapshot bound to go stale. Business code adds no custom claims either.
- **Signing in drops everything the frontend cached for the previous session**, as logging out does: a session that
  ended without a logout (kicked, authenticator reset) would otherwise gate pages and show data with the old
  identity.
- **Invalidation uses `token_version`**: changing the password, disabling the user or kicking them out increments it,
  and every token of that user is void at once. The cost is that invalidation is all or nothing: a single device
  cannot be logged out alone.
- **Logout must call `/v1/sys/auth/logout`**: the cookie is httpOnly, so the frontend cannot delete it. The endpoint
  is anonymous, because a user whose session has already expired must still be able to clear the dead cookie.
- **"Remember me" only decides whether the cookie is persistent or session-only**; the token itself always lasts
  `session_ttl`.

## The three links after authentication

The admin appends `principal` → `authz` → `audit` after authentication (`internal/core/middleware/`; order:
`docs/admin/assembly.md`, "newApp: three more links in the chain, the SPA on the same server"). `principal` is the
ready example of the guide's "Checks after authentication": it reads the verified claims, checks the token version and turns them into a
principal; on an anonymous endpoint without claims it passes the request through, and `authz` decides whether login
is required.

- **Permission bindings live in code**: the business area registers them with `internal/core/perm`, not in the
  database, so a binding is reviewed and versioned with the endpoint it guards. **A missing binding fails startup**
  (`perm.operation-unowned`); at run time an unbound operation is 403 (deny by default). Steps in
  `docs/admin/permission.md`.
- **Anonymous endpoints**: the kernel has login, login-meta, captcha and logout. Before adding an anonymous endpoint, make sure it
  really needs no identity: it also leaves `principal` / `authz`, and permission bindings have no effect on it.
- **Raw routes skip all three links**: `sc.RequireLogin()` only checks the login state; it checks no permission and
  writes no audit record. See `docs/admin/raw-http.md`.

## Captcha (captcha provider)

The human check before login is **one of three, chosen at deploy time**: off, an image code (`base64`) or Cloudflare
Turnstile (`turnstile`), configured in `sys.captcha` in `configs/app.yaml` and read once at startup; the config
keys are in `docs/admin/config.md`.

| provider | What the server does | Deployment prerequisite |
|---|---|---|
| `base64` | Draws the image itself, stores the answer in Redis, deletes it after checking | No external dependency |
| `turnstile` | Verifies the token produced by the widget against Cloudflare siteverify (sending the client IP) | The server can reach Cloudflare; like the login IP allowlist it requires the proxy in front to overwrite `X-Forwarded-For` (see "Client IP" below) |

With `enabled: true`, **the selected provider checks itself at startup** (`captcha.Provider.Ready`): a missing
secret or an unregistered name fails startup, instead of showing up in production as "every login is rejected".

Only the built-in providers (`base64`, `turnstile`) are supported: the login page renders only them, and its form is
part of the frontend kernel. Report a need for another provider upstream instead of adding one here.

## User settings

The `sys_user.settings` column holds **user settings** (the JSON of the `UserSettings` message), sent down with
`Profile`. How they differ from **preferences** (which follow the device and live in localStorage) is in
`GLOSSARY.md` "The two kinds of choices a person makes"; the trade-offs are in
`docs/adr/admin-0011-user-settings-storage-and-write-granularity.md`.

- **What belongs here**: choices that follow the account and serve only the user's own UI. The kernel currently has
  only favorite menu items (identified by route path; their order is the favorites order).
- **What does not**: anything the server must query across users (that needs an index, so its own column or table),
  anything security-related that must be audited (that needs `Audit` and its own endpoint), and business-system data
  (it belongs in the business database).
- **Field number ranges**: numbers 1–99 of `UserSettings` belong to the kernel, 100 and up to derived repos; the
  server ignores unknown fields when parsing. The whole serialized value must not exceed the kernel's limit
  (`biz.MaxSettingsBytes`).
- **Writes are per item, never a whole overwrite**: each kind of setting has its own add/remove endpoints (favorites
  use `POST /v1/sys/auth/favorites/add|remove`), idempotent and returning the complete settings after the change;
  the server locks the user's row in a transaction and does read-modify-write. There is no "send back the whole
  thing" update endpoint: when two browsers each change one item, the later submission would erase the earlier one.
  The one exception is `favorites/reorder`, which overwrites the order and accepts only a permutation of the current
  set.
- The server does **not** check whether a favorite path points to an existing page: routes are the frontend's
  business; the frontend filters out deleted routes and items the user lost permission for before rendering.

## Client IP: the proxy must overwrite X-Forwarded-For

**The nginx in front of the program must overwrite `X-Forwarded-For`** (set it up per
`docs/guides/deploy-nginx.md`); when nginx is not on the same host, list it in `client_ip.trusted_proxies`. The
client IP comes from `metadata.GetRemoteIP` (`keel:docs/metadata.md#how-the-client-ip-is-computed`), and
three kernel features depend on it being real: login lockout counts per username + IP (`sys.lockout.scope`), the
login IP allowlist, and the audit log. If a client could forge the header, it could dodge lockout, pass the
allowlist and falsify audit records.

- **The admin is a public entry, so do not enable `mw.WithTrustedMetadata`**: that option keeps a client IP sent by
  the caller, which is only safe on internal servers (`keel:docs/metadata.md#trust-model`).
- **Before adding any feature that uses the IP as a security criterion, come back to this section** and make sure
  the deployment satisfies it.

## Anti-patterns

- ❌ Putting permission codes, roles or departments into the claims to save a query.
- ❌ The frontend reading the token to decide whether the user is logged in: it cannot read it; the login state is
  whatever the endpoints return.
- ❌ Dispatching authentication failures on the HTTP status in the frontend: dispatch on the reason, see
  `docs/admin/errors.md`.
- ❌ Building a second login, token issuing or refresh-token flow.
- ❌ Reading `X-Forwarded-For` or `X-Real-IP` yourself instead of `metadata.GetRemoteIP`, or deploying without a proxy
  that overwrites it.
