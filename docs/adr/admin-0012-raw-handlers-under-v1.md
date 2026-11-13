# Raw handlers also go under `/v1/`; no home-made prefixes at the root

<!-- Shared file: carried verbatim; never localized outside its source repository.
     Checked by scripts/check-cross-repo-sync.sh; see docs/upgrading-protocol.md. -->

Raw HTTP handlers (SSE, file downloads, WebSocket, static assets) are mounted under `/v1/` like proto RPCs: the
kernel's under `/v1/sys/*`, the business area's under `/v1/<business>/*`. **Root-level paths do not belong to the
server**; they belong entirely to the SPA. The one exception is `/health`, the liveness check Keel registers on every
HTTP server (`docs/admin/raw-http.md`).

The brand logo is the living example: it is an image, not an API, yet it is mounted at `/v1/sys/brand/logo`.

## Why the root cannot be used

In a service with an SPA, the root is the frontend router's address space: page routes (`/dashboard`, `/sys/users`)
are at the root, and the SPA's fallback handler catches `/` with a prefix route so that deep links and static files
all land on `index.html`. Any path the server registers at the root takes an address away from the frontend.

The consequences are disproportionately hard to trace: a server path **unconditionally** beats a page route of the
same name (it matches first), so that page "won't open" rather than "shows an error". Worse is conditional
occupation: a path registered only when some setting is on works fine locally and in most deployments, and when one
deployment turns the setting on, the page disappears for them. The person reporting the bug and the people who cannot
reproduce it talk past each other, with one line of deployment config between them.

`/v1/` has no such problem: it is the API's address space, and frontend routes structurally stay out of it. Within
that prefix, splitting further into `sys` and business areas keeps the kernel and derived repos from taking each
other's space.

## Two rejected approaches

**A home-made prefix at the root** (`/brand/logo`, `/assets/logo` and the like). It looks the most straightforward:
an image really is not an API. But it falls right into the trap above: `/brand` is an utterly ordinary business word,
a derived repo building a "brand management" page will collide with it, and the collision is conditional. Making it
safe would need yet another convention, "such-and-such prefix is reserved by the framework", for every derived repo
to remember: a convention that exists only to work around its own bad choice of location.

**A framework-reserved marker at the root** (a double-underscore prefix like `/__brand/logo`). The collision does
go away, but it sets up another address space and another convention when the API prefix already exists and already
meets the need. Two address spaces mean two sets of proxy config, two path lists, and two "where does this go"
decisions.

## Difference from the upstream template without an SPA

Upstream on the template chain (the level without a frontend), the raw handler example is mounted at the root, and
that is right: in a service without an SPA nobody claims the root, and mounting there is short and clear. This ADR
does not overturn that: **the rule's premise is "the root belongs to the SPA"**, and it does not apply to repositories
where the premise does not hold.


## Consequences

`/v1` is the prefix of a versioned API; putting an image that will never have a v2 inside it is semantically
imprecise. This is a deliberate trade-off: isolation of the address space is a structural guarantee, while semantic
precision is only the beauty of a name; when the former fails it becomes a hard-to-trace bug, the latter does not.

It also brings a side benefit: the dev proxy only needs to cover `/v1` (which it has to anyway), and the brand needs
no proxy entry of its own: one less piece of config to keep in sync with server paths.
