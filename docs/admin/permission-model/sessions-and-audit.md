<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Sessions, caching and audit

Read this before relying on when a permission change takes effect, kicking sessions, or adding an audited action.

## 8. Sessions and caching

- **Disabling takes effect immediately**: principal resolution with `status != 1` → no principal → 401. A disabled user
  is cut off on the next request, equivalent to kicking the session, with no token blacklist.
- **The permission cache is per role**: principal aggregates over the user's RoleIDs. Changing a role's permissions /
  deleting a role deletes only that role's key, so **revocation is immediate**; user-level changes (roles or department
  through `UpdateUser`, Disable) need no cache operation at all (RoleIDs are queried on every request). Cache size is
  O(number of roles).
- **token_version session invalidation**: each user has a token version, written into the `ver` claim
  at issue; principal compares `claims.ver == user.token_version` on every request (it loads the user row anyway), and a
  mismatch → no principal → 401. The following operations do `token_version + 1`, invalidating all of that user's old
  tokens on the next request:
  - **KickUser** (`sys:account:kick`, ARBAC boundary + audit): an administrator kicks a user out;
  - **ChangeMyPassword**: after changing their own password, all the user's other devices are signed out (the current
    device swaps in a new token through a fresh login in the frontend);
  - **ResetPassword**: after an administrator resets the password, the target user is signed out everywhere;
  - **DisableUser**: a second safeguard alongside the status check;
  - **SetUserRoot**: granting or removing root signs the target user out;
  - **ResetOTP**: after an administrator resets the authenticator, the target user is signed out everywhere: a
    reset usually means the authenticator was lost or stolen.
  The mechanism has no Redis dependency, no extra query, and is fail-closed; "kick = all of the user's devices signed
  out".
- The bind-scope token (OTP binding bootstrap) also carries `ver`: if the user is disabled or kicked during binding, the
  bind token is invalidated too.
- **Login brute-force protection (app.yaml `sys.lockout.*`, `deployment-and-config.md` §13, restart after changes)**:
  `enabled / max_failures / lock_minutes / scope` (on by default, 5 attempts / 15 minutes, keyed by default on
  `username_ip`; the meaning of scope and the deployment premise that nginx overwrites XFF are in `docs/admin/auth.md`).
  When on, consecutive failures on the same counter key reaching the threshold (a Redis counter, window = lock duration)
  lock it; during the lock, login always returns INVALID_CREDENTIAL, **not revealing the lock** (`otp.md` §7.1); a
  successful login resets the count; lock events go to `sys_login_log`; a Redis failure fails open (brute-force
  protection is a speed bump and must not lock everyone out because of an infrastructure failure). When off, the
  deployment layer (WAF / gateway rate limiting) is the fallback.
- The session itself (a single httpOnly-cookie token with no refresh token, `sys.session_ttl` as a hard limit, logout
  through the server) is described in `docs/admin/auth.md` "Sessions: settled facts"; to force someone else out, use
  KickUser.

## 9. Audit

- Operations bound with `Audit: true` write `sys_audit_log` after success; root is not exempt.
- **Entry structure**: `operator_id` (from the Principal), `action` (= the operation, cannot be forged),
  `target_type` / `target_id` (the object operated on), `ip`, `created_at`, `request_payload`, `reason` (attached by the
  operator through the `X-Audit-Reason` header).
- **Target extraction rules**: `target_type` is derived only from the operation (the service name without `Service`),
  so the request cannot change it; `target_id` first uses the request's `AuditTarget() string` if implemented, otherwise
  the resource primary key field `id`; if neither yields anything it stays empty and the entry is still written (better
  incomplete than missing). `AuditTarget` is for requests whose object is identified only by several fields (such as
  `COLLECTION/WAKEUP/3`), truncated beyond the column width of 64.
- **Request body and the sensitive-data red line**: `request_payload` records the full request body, but fields marked
  `(keel.v1.redact).sensitive` are replaced with `"***"` before storage: passwords, OTP codes and tokens never enter the
  audit table. The redaction is driven by the proto contract, not by a field-name list in the middleware: marked once,
  it applies to every endpoint using that message; the annotation is described in `keel:docs/proto.md#redact`.
  **A new secret request field must be marked**; an unmarked one is stored in clear text, and the kernel does not
  guess field names.
- **The other tier, `(keel.v1.redact).bulky`**: base64 images such as avatars are replaced with `"[omitted N bytes]"`
  (keeping the byte count). It is not a secrecy measure; it keeps one audit entry from being filled by a 1.4MB image and
  the following fields from being truncated away. The fields the kernel has marked are `avatar` / `image_base64`; new
  fields of the same kind in the business area are marked the same way.
- **No before/after data snapshots**: only business code knows their shape, so the kernel can neither redact them
  (it gets an `any` with no schema) nor guarantee completeness (missing one field still compiles on both sides). The
  cost is that the audit of a delete keeps only `target_id` and cannot say what the deleted object was; the trade-off
  and the path to adding it back are in the package comment of `internal/core/audit`.
- Login attempts (success / failure / failure reason / lock events) all go to `sys_login_log`. A request that fails
  the captcha is not recorded: it reaches no account, and it does not count towards the lockout either.
- Each query endpoint is bound to one view code: `ListAudit`, the detail `GetAudit` and the action candidates
  `ListAuditActions` are bound to `sys:audit:view` (list rows carry no request body or reason; the detail fetches them
  by ID), `ListLogin` is bound to `sys:login_log:view`; whoever has the code sees everything, with no data scope, so
  viewing logs can be given to a dedicated role (auditors) without making it a super administrator. The log tables are
  append-only; the archive policy is in the deployment checklist.
