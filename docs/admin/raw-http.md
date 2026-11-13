<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# Raw routes: three admin additions

Read this before adding a webhook, SSE, download or other raw route to the admin, together with
`docs/guides/raw-http.md`.

The guide covers choosing between a contract endpoint and a raw route and how to write SSE / WebSocket / download /
webhook; this page covers only the rules an admin built on keel-admin adds. `<app>` is the entry directory name.

## They bypass the whole permission middleware

In the admin, a raw route bypasses not only encoding and decoding but also `principal` (the principal), `authz`
(permission-code bindings and step-up) and `audit` (write-operation audit): nothing in `perm.Bindings` applies to
it, and its operations never reach the audit log. `sc.RequireLogin()` / `sc.Anonymous()` only settle whether login
is required. A raw route that needs a permission check must take the principal, check it and write the audit record
itself in the handler. **That is the strongest reason to prefer contract endpoints in the admin**; management writes
always go through contract endpoints.

The kernel itself has a single raw-route user: `server.RegisterSPA` (`internal/server/spa.go`) mounts the frontend
pages and the two brand paths (`/v1/sys/brand`, `/v1/sys/brand/logo`) on the same HTTP server, all with
`sc.Anonymous()`, because the login page must show the logo and shell before login.

## Paths go under `/v1/`; the root belongs to the SPA

The whole root belongs to the SPA (page routes plus the fallback handler). A path the server registers at the root
unconditionally hijacks the page route of the same name, and the problem only appears once that path is registered:
the hardest kind to track down. **Kernel raw routes go under `/v1/sys/*`, business ones under `/v1/<business>/*`.**
The reasoning is in `docs/adr/admin-0012-raw-handlers-under-v1.md`.

The one exception is `/health`, the liveness check Keel gives every HTTP server: `api.NewHTTPServer` registers it
before all other routes, so the SPA fallback cannot swallow it
(`keel:docs/http.md#liveness-every-http-server-has-health`).
The cost is that the frontend **cannot have a page route named `/health`**: it would never open.

## Registered in the business area, appended as newApp's variadic arguments

```go
// internal/service/http.go (business area; a fictional payment callback)
func NewStreamRegister(webhook *WebhookHandler) api.StreamRegister {
    return func(sc *api.StreamContext) {
        // The signature is the credential itself; no framework authentication at all: sc.Server()
        sc.Server().HandleFunc("/v1/payment/hooks", webhook.HandlePayment)
    }
}

// cmd/<app>/di.go: appended as newApp's variadic arguments (the kernel adds the SPA itself; do not write it)
app, err := newApp(appFactory, httpServerRegister, authUseCase, otpUseCase,
    auditRecorder, sentinelGroup, brand,
    nsvc.NewStreamRegister(webhookHandler))
```

Never write it in `internal/server/` (kernel, overwritten whole). **The SPA always comes last**: it is a fallback on
`HandlePrefix("/")`, routes match in registration order, and placed before the business routes it would shadow every
business raw route; `newApp` already guarantees that order.

## Anti-patterns

- ❌ A raw route at the root (`/hooks/payment`).
- ❌ Writing `perm.Bindings` for a raw route: the binding table applies only to contract endpoints that pass through
  the chain, so it has no effect.
- ❌ Using a raw route for a management write: it bypasses `audit`, and the change never reaches the audit log.
