<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Startup errors

Every developer-facing error the admin kernel returns at startup or registration starts with `keel-admin/<id>`; each
ID has a section here (checked by `internal/core/errs/errs_test.go`). Errors starting with `keel/<id>` come from the
framework: see `keel:docs/errors.md`. Runtime failures and messages shown to
end users are not listed.

## perm.registered-after-seal

A permission, binding, resource or group was registered after `perm.Seal` ran. Sealing validates the whole registry
once; anything added later would never be checked.

Fix: call every `perm.MustRegister*` in `di.go` before `perm.Seal` (docs/admin/permission.md, step 7).

## perm.code-duplicate

Two `Permission` entries use the same code, usually because a business module is registered twice or two modules
picked the same code.

Fix: keep one `Permission` per code; register each module once.

## perm.operation-duplicate

The same RPC operation appears in two `Bindings` maps, so it would have two access requirements.

Fix: bind each operation in exactly one `Bindings` map (docs/admin/permission.md, step 5).

## perm.resource-duplicate

The same data-scope resource is registered twice.

Fix: register each `ResourceDef` once.

## perm.group-domain-missing

A permission group has no business domain, so it cannot be placed in the permission tree.

Fix: set `Domain` on every `Group` passed to `perm.MustRegisterGroups`.

## perm.group-duplicate

The same business-domain or module group is declared twice.

Fix: declare each group once; modules that share a domain only declare their own module group.

## perm.code-format

A permission code is not `<domain>:<module>:<action>` in lowercase.

Fix: rename the code (docs/admin/permission.md, step 2).

## perm.code-reserved-prefix

A business permission uses the `sys:` prefix, which belongs to the kernel.

Fix: pick another business domain.

## perm.require-otp-manual

A `Permission` sets `RequireOTP` itself. Whether a permission requires a one-time code is derived from the bindings
that use it, so a hand-set value could disagree with them.

Fix: remove it from the `Permission` and set `RequireOTP` on the `Requirement` in the bindings.

## perm.domain-group-missing

A permission's business domain has no group, so the permission cannot be shown in the role editor.

Fix: declare the domain group with `perm.MustRegisterGroups`.

## perm.module-group-missing

A permission's module has no group.

Fix: declare the module group with `perm.MustRegisterGroups`.

## perm.dep-unknown

A permission lists a dependency that is not registered, so granting it could never satisfy that dependency.

Fix: register the dependency or remove it from `Deps`.

## perm.binding-unknown-code

An operation is bound to a permission code that is not registered: the endpoint would be unreachable for everyone.

Fix: register the permission or correct the code in the binding.

## perm.operation-unowned

An operation of the proto contract has neither a binding in `perm.Bindings` nor `(keel.v1.auth).allow_anonymous`: it
would answer 403 to everyone. The message lists every such operation.

Fix: bind each one in its module's `perm.Bindings` (empty `Codes` means any signed-in user), or annotate it
allow_anonymous in the proto (docs/admin/permission-model/function-permissions.md).

## perm.operation-anonymous-bound

An operation is annotated `allow_anonymous` and also bound in `perm.Bindings`. Anonymous operations skip
authorization, so the binding never runs.

Fix: remove the binding, or remove allow_anonymous if the operation needs a signed-in user.

## perm.resource-parent-unknown

A resource derives its data scope (`Via`) from a parent that is not registered.

Fix: register the parent resource or correct `Via.Parent` (docs/admin/permission-model/data-scope.md).

## perm.resource-nested-derivation

A resource derives its scope from a resource that is itself derived. Only one `Via` level is allowed, so scope checks
stay a single join.

Fix: derive from a resource that has its own scope.

## perm.dep-cycle

Permission dependencies form a cycle, so none of them could be granted first.

Fix: remove one dependency on the cycle.

## sys.brand-title-missing

`sys.brand.title` is empty; the login page and browser tab need a product name.

Fix: set `sys.brand.title` (docs/admin/config.md).

## sys.business-zone-missing

`sys.business_time_zone` is empty; business days are computed in this zone.

Fix: set it to an IANA zone such as `Asia/Shanghai` (docs/admin/config.md).

## sys.business-zone-invalid

`sys.business_time_zone` is not a time zone known to the Go runtime.

Fix: use an IANA zone name; on minimal images make sure time zone data is available.

## sys.login-signer

The login token signer cannot be built from the default HTTP server's auth config. An empty `jwt_secret` fails
earlier, at config load, as `keel/kconfig.invalid`; this ID appears when the signer is given a config that skipped that
validation.

Fix: set a non-empty `jwt_secret` for the default HTTP server (docs/admin/auth.md).

## sys.cookie-name-missing

`server.http.default.auth.cookie_name` is empty. Login sets the session in a cookie of that name and the
authentication middleware reads the token from it, so without a name a browser session can never authenticate.

Fix: set `cookie_name` under `server.http.default.auth` (docs/admin/auth.md).

## sys.otp-key-missing

`sys.otp.aes_key` is empty; one-time-code secrets are stored encrypted with it.

Fix: inject `sys.otp.aes_key` from the deployment environment; never commit it.

## captcha.provider-unknown

`sys.captcha.provider` names a provider that is not registered.

Fix: set it to a registered provider, or register the provider with `captcha.MustRegister` in `di.go`
(docs/admin/auth.md).

## captcha.provider-not-ready

The selected captcha provider failed its self-check, usually a missing secret. Without it every login would be
rejected with no hint why.

Fix: supply the settings the message names, or set `sys.captcha.enabled` to false.

## captcha.provider-name-empty

`captcha.MustRegister` was called with an empty name, which `sys.captcha.provider` could never select. It panics at
assembly in `di.go`.

Fix: pass the provider's name, the value `sys.captcha.provider` uses to select it.

## captcha.provider-duplicate

Two providers were registered under the same name with `captcha.MustRegister`; the second would silently replace the
first. It panics at assembly in `di.go`.

Fix: register each provider once, under a distinct name.
