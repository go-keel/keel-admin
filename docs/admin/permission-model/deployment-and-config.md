<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Deployment and kernel configuration

Read this before deploying an admin, or changing the `sys:` section of app.yaml.

## 12. Deployment and security checklist

- Secrets such as `jwt_secret` and `sys.otp.aes_key` never go into git and **are split by environment**: local
  development writes them into `config.dev.yaml` and `app.dev.yaml` (both gitignored, development only; the two main
  config files in the repository hold only development placeholders), and production injects the real values into
  `config.yaml` / `app.yaml` on the deployment side (a mounted Secret and the like; secrets of an added captcha provider
  likewise); loading a `*.dev.yaml` logs the Warn `kconfig: dev overlay loaded`: **seeing it in production logs is an
  incident signal** that the build carried the development config into the image (Keel's dev overlay:
  `keel:docs/config.md#dev-overlay-configdevyaml`);
- root's initial password is admin888, seeded by the table creation script, not set through a config file;
- Forcing root to change its password on first sign-in is not built into the template: root changes it in the personal
  center right after the first sign-in (make this part of the deployment procedure), never by editing the kernel
  script;
- The session token travels in an httpOnly cookie (`SameSite=Strict` against CSRF; `Secure` requires
  `sys.cookie_secure=true` in production, with no TLS sniffing, since `r.TLS` is always nil behind a reverse proxy), see
  `sessions-and-audit.md` §8;
- Upstream ships only complete schemas, `migrations/admin/0_schema.sql` and `migrations/business/0_schema.sql`,
  updated in place, with no incremental migrations; no AutoMigrate. Which directory belongs to whom and how this
  repository's own tables are scripted: `docs/admin/domain-model.md`, "Schema";
- When a new business resource joins scope, code review must check (`data-scope.md` §5.5): the table has the
  `owner_user_id`/ `owner_dept_id` ownership columns (matching the declared anchors), **every path
  List/Get/Update/Delete** uses `ApplyScope`, ownership on write is taken from the Principal, and handover does not
  change the created_by audit column.
- `sys_audit_log` / `sys_login_log` are append-only and archived on the operations schedule (partitioning or periodic
  dumps); the template builds none in.
- When exposed to the public internet in production, turn on `sys.captcha.enabled` (§13, restart after changing the
  config); secret parameters are never committed, and production injects them on the deployment side (the red lines in
  §13).

## 13. Kernel configuration (the `sys:` section of app.yaml): read once at startup, restart after changes

app.yaml supports hot reload as a mechanism, but the kernel's `sys:` section is taken as one snapshot at assembly
(`server.ProvideSysConf`) and never read back, so a change takes effect only after a restart; business configuration
gets its own section on `Bootstrap` and decides for itself. The rules are in `docs/admin/config.md` "`sys.*`"; this
section keeps the reasons.

**Why `sys:` is not hot-reloaded** (it is mostly security parameters: captcha, lockout, cookie, session):

- A security switch that can be changed online is itself an attack surface: whoever takes over one administrator session
  could first turn off the protection and then brute-force other accounts; changing a file has to go through the
  deployment channel;
- File configuration ships with the release through git: reviewable, revertible, reproducible per environment, with no
  drift of "configuration that exists only in some environment's store"; runtime configuration backed by a store also
  silently falls back to defaults when the store fails, dropping its armour exactly when the system is most fragile;
- The admin is a single binary that restarts in seconds, an acceptable cost. Trade-off: emergency changes to
  brute-force protection / captcha parameters go through the operations channel, not the management UI.

**The only source of truth for the field list is the comments in `internal/conf/app.proto` and `configs/app.yaml`**
(brand / session_ttl / cookie_secure / business_time_zone / captcha / lockout / otp); this section keeps no second
table. One exception: when `sys.brand.logo` is set to a disk path, the **file contents** are read from disk on every
request (a new image takes effect at once), but the path itself is still a startup snapshot.

**Red lines** (independent of the implementation):

- Secrets (`sys.otp.aes_key` and the like) never have real values committed: app.yaml is the main configuration actually
  deployed in production, so production injects the real values into app.yaml on the deployment side (a mounted Secret
  and the like). app.yaml **does not take part** in the `KEEL_` environment variable overlay (kconfig.NewDynamic reads
  only the files), so "secrets from environment variables" does not hold here;
- The `sys:` section governs only the admin kernel itself; runtime parameters of external business services do not
  go there (a business reading admin configuration = a reverse dependency, violating the positioning boundary in
  `README.md` §1).
