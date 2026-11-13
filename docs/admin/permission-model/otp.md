<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

# OTP and login captcha

Read this before adding a step-up operation, or touching OTP login admission or the login captcha.

## 7. Dimension four: OTP

### 7.1 Login admission (fail-closed)

Any role with `force_otp=1` (or a root account) → login requires OTP. A user forced to use OTP who has not bound an
authenticator gets a 10-minute bind-scope restricted token instead of the access token (`scope=otp_bind`, may only call
BindOTP/ConfirmOTP; step 4 of `function-permissions.md` §4.4), and the frontend guides the binding.

- **Login does not reveal account state.** Apart from `CAPTCHA_INVALID` and `IP_NOT_ALLOWED`, every rejection
  (unknown, disabled or locked account; missing, wrong or exhausted one-time code; wrong password) reports the same
  `INVALID_CREDENTIAL`, with a message telling the user to check the username and password and, for an account with a
  bound authenticator, to also enter the one-time code.
- The login IP allow list is the only rejection reported explicitly (the message includes the current IP), so the
  person knows which address to ask an administrator to add. It is checked only **at the moment of login** and does
  not invalidate sessions already logged in: changing the allow list mostly adds an address rather than tightening,
  and kicking everyone out every time is disruptive; to tighten, use "Force sign-out" separately (which also requires
  the one-time code).
- Wrong one-time codes and wrong passwords both count toward brute-force protection; a missing code, or a correct code
  with its uses exhausted, does not count.
- Any error in an OTP-related query rejects the login (fail-closed); a DB failure never degrades to "no OTP needed".

### 7.2 Per-operation step-up

An operation bound with `Requirement.RequireOTP: true` must carry the `X-OTP-Code` header on every call, verified and
used on the spot; **root is not exempt**.

**The decision reads only `Requirement.RequireOTP`** (authz reads nothing else). `Permission.RequireOTP` is not a second
criterion but a display marker **derived** by `Seal()` from the bindings: as soon as any step-up operation references a
code, the code is marked as "includes operations that need re-verification", so the grant UI can flag it. Writing it by
hand is rejected by Seal: two declarations would inevitably drift (if a code marked step-up only on the binding side
sent a hand-written field to the frontend, the high-risk flag would be missing).

**The frontend cooperates (explicit integration, no automatic replay)**:

- The template's web core provides a **standard OTP input component** (a 6-digit code input embedded in the
  confirmation dialog); the business uses it explicitly in the form or confirmation dialog that triggers a require_otp
  operation, and the request carries `X-OTP-Code` once;
- The caller passes `needOtp` to the form or action dialog of an operation whose binding has `RequireOTP`; nothing
  exposes the list of such operations, so the two are kept in step by hand (the backend binding decides). The code is
  collected before submission and the request is sent once, with no implicit replay;
- 401 `OTP_STEPUP` is only the backend's fallback: when the frontend missed the integration, the global error message
  guides a retry, **with no automatic replay**;
- 401 `OTP_INVALID`: a code was sent but is wrong. It is reported separately from "no code", otherwise someone who
  mistyped one digit would be sent in circles by `OTP_STEPUP`'s "enter the code", sure they had filled it in (the login
  flow is not covered by this: it reports one uniform error so as not to leak the account's state, see §7.1);
- 500 `UNEXPECTED_ERROR`: the verification itself failed. It must not be reported as any of the above either: both "no
  code" and "wrong code" tell the user they made a mistake, while the fault is on the server;
- 401 `OTP_BIND_REQUIRED`: tells the user to bind OTP first and guides them to the binding flow in the profile.

### 7.3 Replay protection

Every successful verification of a code is counted per user and per code for 90 seconds, which covers the code's
whole validity under clock skew; login admission and step-up share the same count. A count above `sys.otp.max_uses` is
rejected (`OTP_ALREADY_USED`).

`sys.otp.max_uses` is in app.yaml (read once at startup, default **3**; 0 or negative counts as not configured
and falls back to the default: the literal meaning of 0 is "no code is usable", which would lock everyone out). It is a
deliberately relaxed replay protection: set to N, the same code can be submitted N times within the 90s window,
including by an attacker who intercepted it, and the risk grows linearly with N; set to 1 it is RFC 6238 §5.2's one use
per code.

To ask for codes less often, the first choice is still to merge a multi-step sensitive operation into a single endpoint
(see §7.2 and `UpdateRole`), which does not spend this quota. The frontend must not design around the limit either (such
as putting several step-up points in one form): a deployment can set it back to 1 at any time.

### 7.4 Login captcha (built in, configurable)

Runs as **step 0** of the login state machine (before the password check), independent of and complementary to
brute-force protection (`sessions-and-audit.md` §8).

**Interface and registration** (the same philosophy as perm: an interface + explicit registration in di.go + switching
by config):

```go
type Provider interface { // internal/core/captcha
    Issue(ctx context.Context) (*Challenge, error)  // issue a challenge {ID, ImageBase64};
                                                           // implementations that need no server-issued challenge return nil
    Verify(ctx context.Context, id, answer, remoteIP string) (bool, error)
    Ready() error                                          // usable right now (such as its secret being set)
}

// di.go: built-in base64 (local image code, mojocn/base64Captcha) and turnstile (Cloudflare)
captcha.MustRegister("base64", base64captcha.New(redis, length, ttl))
captcha.MustRegister(turnstile.Name, turnstileProvider)
```

**Configuration**: the `sys.captcha` section of app.yaml (`deployment-and-config.md` §13, read once at startup, restart
after changes): `enabled / provider / base64_length / base64_ttl / turnstile.{site_key, secret_key}`, out-of-the-box
defaults in `configs/app.yaml`; secret parameters (turnstile.secret_key, otp.aes_key) are never committed: the app.yaml
in the repository holds only development placeholders, and production injects them on the deployment side (the red lines
in `deployment-and-config.md` §13). When `enabled`, **the selected provider checks itself at startup** (`Ready`): a
missing secret or an unregistered name fails startup and lists the names in the registry; without that check it would
show up as "every login is rejected": fail-closed is right, but with no clue why. If an unregistered name is still hit
at runtime (say the registry was changed), it stays fail-closed: verification always fails (with a Warn log), never
silently degrading to "no captcha".

**Contract**:

- `LoginRequest` gains optional `captcha_id` + `captcha_answer` (base64: both required; for providers that need no
  server-issued challenge (remote widget verification): id empty, answer = the widget token);
- The anonymous `login-meta` endpoint tells the login page whether captcha is on and which provider to render; the
  anonymous `captcha` endpoint serves `Issue` and exists only for Issue-type providers. **A challenge is single use**:
  verification consumes it whether it passes or not, so the same answer cannot be retried;
- A failed check returns `CAPTCHA_INVALID` and **does not count toward the brute-force failure count**;
- fail-closed: when `enabled` and the provider's verification errors (such as Turnstile unreachable), it counts as not
  passed.

**Frontend**: the web core login page renders by login-meta: nothing at all when disabled, an image code for `base64`
(click the image to refresh), the Cloudflare widget for `turnstile` (following color mode and language). Both reset on
"attempt number": a failed login starts a fresh round, because both the challenge and the widget token are single use,
and not refreshing guarantees another failure. Only the built-in providers are supported.
