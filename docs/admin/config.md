<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Configuration: the kernel's `sys` branch and the two databases

Read this before adding or changing a config field, a `sys.*` key or database config, together with
`docs/guides/config.md`.

The guide covers the two configs (`config.yaml` and `app.yaml`), load priority, local overrides and reload semantics;
this page covers only what an admin built on keel-admin adds. How to configure the frontend shell (brand, time
zones, languages, appearance) is in `docs/admin/shell.md`.

## `sys.*`: in the hot-reload file, read once at startup

The `sys` branch of `configs/app.yaml` holds the kernel's runtime parameters: session length, the cookie security
flag, login protection, the OTP secret key, brand, business time zone and captcha. di.go reads it once with
`server.ProvideSysConf(bootstrap)` and hands the values to the constructors, so **a change needs a restart**; do not
assume it takes effect because it sits in `app.yaml`. The same holds when it is written in `app.dev.yaml`.

- **Business config does not go into `sys`.** Business config that should reload gets its own field and message on
  `Bootstrap` in `internal/conf/app.proto` (the guide's "The service config: app.proto → make config → Dynamic"),
  read with `.Get()` at runtime. Mixing it into `sys` makes "does this need a restart?" impossible to answer.
- **Some keys are required, and startup fails without them**: `sys.brand.title`, `sys.business_time_zone` (an IANA
  name; an unknown name also fails), `sys.otp.aes_key` (the repository holds only a development placeholder;
  production injects it at deploy time); and `sys.captcha.turnstile.secret_key` when captcha is on with
  `turnstile`.
- **Keys commonly overridden in local development** (in `app.dev.yaml`): `sys.cookie_secure: false` (local http),
  `sys.lockout.enabled: false` (so repeated login tests do not lock you out), `sys.otp.aes_key`.
- The `kconfig: dev overlay loaded` warning logged when the local overlay applies is an incident if it shows up in
  production logs: `app.yaml` holds the OTP secret key.

Captcha config keys:

| provider | What to configure | Notes |
|---|---|---|
| `base64` | `base64_length` / `base64_ttl` | No external dependency |
| `turnstile` | `turnstile.site_key` / `turnstile.secret_key` | `site_key` is public and sent to the login page with `/v1/sys/auth/login-meta`; production injects `secret_key` at deploy time |

How the two providers behave on the server and what they require from the deployment is in `docs/admin/auth.md`
"Captcha (captcha provider)".

## `config.yaml`: the blocks used and the two databases

The admin uses only `app` / `logger` / `otel` / `server.http.default` / `database` / `redis` (plus `client` when it
calls business systems). `server.http.default.auth` is always on; `cookie_name` is the name of the session cookie
(see `docs/admin/auth.md`).

`database` has two agreed names, and **both are required**: `default` = the business database (the business area's
own tables), `admin` = the admin database (all `sys_*` tables). `default` belongs to the derived repo's own code and
the kernel takes a named instance as a guest; there is no fallback, so a single-database deployment writes the same
DSN into both `source` entries. The trade-offs are in `docs/adr/admin-0002-two-databases-and-default-ownership.md`.

## Anti-patterns

- ❌ Putting business fields into `sys`.
- ❌ Changing `sys.*` and assuming it takes effect without a restart.
- ❌ Committing real values of `sys.otp.aes_key` or `secret_key` to `app.yaml`.
- ❌ Configuring only one database, or letting business code read `database.admin`.
