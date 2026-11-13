VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# buf plugins are pinned in api/buf.gen.yaml as go run ...@version; nothing to install globally.
# buf itself is pinned too: @latest would generate different output on different machines and days, and the
# generated files are committed, so that drift shows up as "make proto gives me a huge diff".
.PHONY: init
init:
	go install github.com/bufbuild/buf/cmd/buf@v1.75.0

.PHONY: proto
# scripts/sync-proto.sh first copies the imported .proto files into third_party/ from the versions in go.mod; then
# generate from the root buf.yaml workspace, api/ only. Delete api/'s generated files first: buf generate only writes
# outputs for .proto files that still exist, so a deleted .proto would leave its *.pb.go behind, still compiling.
# Not buf generate --clean: the outputs are api/ itself and the repo root, so it would delete sources too.
proto:
	scripts/sync-proto.sh
	find api -name '*.pb.go' -delete
	buf generate api --template api/buf.gen.yaml -o api
	# The frontend's error routing depends on the reason enum; regenerate it too (see scripts/gen-error-reason.mjs)
	node scripts/gen-error-reason.mjs
	# The frontend's API types are derived from openapi.yaml, generated in the same chain as the reasons and
	# committed: a wrong field name fails tsc on the spot instead of yielding undefined at runtime.
	# openapi-typescript is a devDependency of web/; run pnpm install in web/ once first.
	cd web && pnpm exec openapi-typescript ../openapi.yaml -o src/generated/openapi.ts

# app.proto goes through buf too, not a global protoc: protoc and protoc-gen-go would have to be installed on each
# machine, and whether and which version is installed would change the output, which is committed. app.proto
# imports only one WKT (duration), which buf bundles, so no --proto_path.
# The template is inline rather than a file: internal/conf has a single proto, not worth another config file.
# Keep the protoc-gen-go version the same as in api/buf.gen.yaml.
CONF_GEN_TEMPLATE = {"version":"v2","plugins":[{"local":["go","run","google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11"],"out":".","opt":["paths=source_relative"]}]}

.PHONY: config
config:
	cd internal/conf && buf generate --template '$(CONF_GEN_TEMPLATE)' .

# The frontend's permission-code union comes from the same registration di.go runs (cmd/admin/di.go,
# registerPermissions); the generator is a skipped-by-default test in cmd/admin, so it reuses that function as is.
.PHONY: perms
perms:
	go test ./cmd/admin -run '^TestGeneratePermCodeTS$$' -count=1 -args -update-perm-codes >/dev/null

# routeTree.gen.ts is written by the router plugin when vite starts; there is no separate generate command: start
# vite with web/'s own config and close it at once. No @tanstack/router-cli: it is a second generator, and when its
# version differs from the plugin's built-in one the same routes come out two ways. Needs pnpm install in web/.
.PHONY: routes
routes:
	cd web && node --input-type=module -e "import { createServer } from 'vite'; const s = await createServer({ server: { middlewareMode: true }, appType: 'custom', logLevel: 'error' }); await s.close();"

# Generated files (defined in GLOSSARY.md) are committed but written only by the generate commands. Regenerate
# and compare checksums before and after: a difference means someone edited a generated file by hand, or changed
# its source without regenerating. It compares the working tree before and after, not git: right after a proto
# change, before committing, generated files always differ from git, and that is exactly when check runs most.
# On a mismatch the generated files are already fresh: review the diff, commit them, and the next run passes.
.PHONY: check-generated
check-generated:
	@list() { { find api internal/conf -name '*.pb.go'; find third_party web/src/generated -type f; echo openapi.yaml; echo web/src/routeTree.gen.ts; } | LC_ALL=C sort | xargs cksum; }; \
	before=$$(mktemp); after=$$(mktemp); trap 'rm -f "$$before" "$$after"' EXIT; \
	list >"$$before"; $(MAKE) -s proto config perms routes >/dev/null || exit 1; list >"$$after"; \
	diff "$$before" "$$after" >/dev/null || { \
		echo "Generated files do not match their sources (edited by hand, or the source changed without regenerating). Regenerated; the files below changed; review the diff and commit them:"; \
		diff "$$before" "$$after" | sed -n 's/^[<>] [0-9]* [0-9]* /  /p' | sort -u; exit 1; }

.PHONY: generate
generate:
	go mod tidy
	go generate ./...

# The binaries go straight to production; every entry point shares these build flags:
#   CGO_ENABLED=0  fully static, no dependency on the target's C library; runs in minimal images (scratch /
#                  distroless);
#   -trimpath      strips the build machine's absolute source paths: stack traces leak no local directories, and
#                  the same source builds the same on any machine;
#   -s -w          strips the symbol table and DWARF debug info, 20-30% smaller. The cost is that debuggers such
#                  as dlv cannot attach to production processes; panic stack traces are unaffected (they use
#                  pclntab, not the symbol table).
GO_BUILD = CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)"

# Single-binary delivery: embeds the SPA by default (run make build-web first to produce web/dist)
.PHONY: build
build:
	mkdir -p bin/ && $(GO_BUILD) -tags embed_ui -o ./bin/admin ./cmd/admin

# Backend binary without the embedded frontend (for local development with the vite dev server)
.PHONY: build-noweb
build-noweb:
	mkdir -p bin/ && $(GO_BUILD) -o ./bin/admin ./cmd/admin

# --frozen-lockfile: the output must be built from the exact versions in the lockfile, never upgrading
# dependencies along the way; the same commit must produce the same dist on any machine.
.PHONY: build-web
build-web:
	cd web && pnpm install --frozen-lockfile && pnpm build

# `bin/` and `web/dist/` are in .gitignore, so git will not clean them, and `git clean -fdx` would also take
# `configs/*.dev.yaml`, private keys and `node_modules` with it; without this target there is no safe way to clear
# build output. Not a prerequisite of build: a full rebuild should not be the cost of every compile.
.PHONY: clean
clean:
	rm -rf bin/ web/dist/

# Whether repository paths referenced in the docs exist. AGENTS.md tells the AI to read the matching guide before
# acting; with a wrong path it works from the wrong thing, and questions it less than a human would.
#
# Two tiers, see the distribution boundary in `scripts/check-doc-links.sh`: `lint` checks only the kernel /
# scripts / docs, the tier shipped to derived repositories; `lint-strict` checks everything, **for this
# repository's self-check**. This repository is a template: run lint-strict after changing docs; derived
# repositories run lint.
# lint-strict also checks the registration of shared files and segments, both ways for each form, by marker, not
# file name:
#   whole shared files: docs, scripts and configs (including .mjs) under docs/, scripts/ or deploy/ with the
#   shared-file marker in their first 10 lines must be registered in the sync list, on a line of their own or
#   under a directory entry (such as docs/admin); registered files must carry the marker (directory entries are
#   checked file by file, only under docs/, scripts/ and deploy/; kernel code carries no marker).
#   shared segments: any .md with any shared segment begin marker (`<!-- shared segment begin <source repo>: `)
#   must have an entry registering that file as seg (any source repository, active or commented out); a file
#   registered as seg must have at least one shared segment, with begin and end markers paired per name.
# The keel-layout block is what this repository compares against upstream (active); the keel-admin block is
# registered at the end (commented out, for derived repositories to enable); each one missing is one check fewer
# for them.
.PHONY: lint
lint:
	@scripts/check-doc-links.sh

.PHONY: lint-strict
lint-strict:
	@STRICT=1 scripts/check-doc-links.sh
	@list=scripts/cross-repo-sync.list; \
	whole=$$(awk '/^#?keel-(layout|admin)[ \t]/ { s = 0; for (i = 3; i <= NF; i++) if ($$i == "seg") s = 1; if (!s) print $$2 }' $$list); \
	registered() { for w in $$whole; do case "$$1" in "$$w" | "$$w"/*) return 0 ;; esac; done; return 1; }; \
	miss=0; \
	for f in $$(find docs scripts deploy -type f \( -name '*.md' -o -name '*.sh' -o -name '*.mjs' -o -name '*.conf' -o -name '*.example' \)); do \
		head -n 10 "$$f" | grep -qF 'check-cross-repo-sync.sh' || continue; \
		registered "$$f" || { echo "  not registered: $$f"; miss=1; }; \
	done; \
	for w in $$whole; do \
		[ -e "$$w" ] || { echo "  registered but missing: $$w"; miss=1; continue; }; \
		for f in $$(find "$$w" -type f \( -name '*.md' -o -name '*.sh' -o -name '*.mjs' -o -name '*.conf' -o -name '*.example' \)); do \
			case "$$f" in docs/* | scripts/* | deploy/*) ;; *) continue ;; esac; \
			head -n 10 "$$f" | grep -qF 'check-cross-repo-sync.sh' || { echo "  registered but no shared-file marker in the header: $$f"; miss=1; }; \
		done; \
	done; \
	segs=$$(awk '/^#?keel-(layout|admin)[ \t]/ { for (i = 3; i <= NF; i++) if ($$i == "seg") print $$2 }' $$list); \
	start='^<!-- shared segment begin [A-Za-z0-9_-]*: '; \
	for f in $$(grep -rlE "$$start" --include='*.md' --exclude-dir=.git --exclude-dir=.claude --exclude-dir=node_modules . | sed 's#^\./##'); do \
		printf '%s\n' "$$segs" | grep -qxF "$$f" || { echo "  has shared segments but is not registered as seg: $$f"; miss=1; }; \
	done; \
	for f in $$segs; do \
		[ -f "$$f" ] || { echo "  registered as seg but missing: $$f"; miss=1; continue; }; \
		names=$$(sed -n 's/^<!-- shared segment begin \([A-Za-z0-9_-]*\): .*/\1/p' "$$f" | sort -u); \
		[ -n "$$names" ] || { echo "  registered as seg but has no shared segment begin marker: $$f"; miss=1; continue; }; \
		for r in $$names; do \
			[ "$$(grep -c "^<!-- shared segment begin $${r}: " "$$f")" -eq "$$(grep -cx "<!-- shared segment end $$r -->" "$$f" || true)" ] || { echo "  begin and end markers of shared segment $$r are not paired: $$f"; miss=1; }; \
		done; \
	done; \
	[ $$miss -eq 0 ] || { echo "Markers and registrations of shared files and segments must match both ways: every file with the shared-file marker or a shared segment must be registered in scripts/cross-repo-sync.list (the latter as seg; for a new registration, the UPGRADING.md entry tells derived repositories to add the line), and every registered file must carry the matching marker"; exit 1; }

# Rename after forking: replaces this repository's module references in go.mod, non-generated .go, .proto
# go_package and .md, together with the proto package segment under api/ (directory and package), in one go;
# never touches *.pb.go (sed would corrupt rawDesc). The package segment defaults to the last element of the
# module path; API_DOMAIN overrides it; optional CMD=<entry name> also renames the cmd/admin directory and the
# bin/admin binary. Idempotent; the reasons are in the script header.
.PHONY: rename
rename:
	@test -n "$(MODULE)" || (echo "Usage: make rename MODULE=example.com/demo [API_DOMAIN=demo] [CMD=demo-admin]" && exit 1)
	@MODULE="$(MODULE)" API_DOMAIN="$(API_DOMAIN)" CMD="$(CMD)" scripts/rename.sh

# Cross-repository verbatim check: how a derived repository finds upstream changes to shared files and kernel trees
# (skipped, with a note, when no upstream clone is checked out locally). The list is scripts/cross-repo-sync.list; the reasons are at the top of
# scripts/check-cross-repo-sync.sh.
.PHONY: check-sync
check-sync:
	@scripts/check-cross-repo-sync.sh

.PHONY: all
all: proto config perms routes generate

# git hooks do not travel with a clone; run this once after forking. .githooks/pre-commit stops a commit that
# changes the propagation surface without touching UPGRADING.md, and checks for dead links; the criterion and
# exemption are at the top of that file. Automatic merges do not trigger pre-commit; pre-merge-commit checks
# dead links for them.
.PHONY: init-hooks
init-hooks:
	git config core.hooksPath .githooks

.PHONY: root-hash
root-hash: ## Generate the root password hash: make root-hash PASS=yourpass
	@go run ./scripts/roothash "$(PASS)"

# Derived repositories have no go.work; modules resolve through go.mod. go.work hides the versions in go.mod, so a
# local build passing does not mean a derived repository builds; GOWORK=off reproduces their resolution.
# Always run it after changing the github.com/go-keel/keel version; the version follows upstream and derived repositories do
# not upgrade it on their own (see docs/upgrading-protocol.md).
.PHONY: check-mod
check-mod:
	GOWORK=off go build ./...

# Real-database tests (build tag integration): the same data-layer cases as make check, on a fresh MySQL database
# built from migrations/*/0_schema.sql. Needs TEST_MYSQL_DSN, e.g. 'root:keel@tcp(127.0.0.1:53306)/?parseTime=true'; a missing
# variable fails. Not part of make check.
.PHONY: integration
integration:
	GOWORK=off go test -race -tags integration -count=1 -p 1 ./...

# golangci-lint runs with go run at a pinned version: nothing to install, not in go.mod. Default linters.
GOLANGCI = github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

# The agent's self-check: these repositories have no CI, so consistency is up to the agent; every command needed
# to confirm the repository is clean, in one. Run it after changing code or docs; it stops at the first failing
# step. make echoes each command before running it, so on failure the last echoed line is the failing step.
# lint-strict is this repository's (a template) self-check tier; a derived repository changes it to lint after
# forking, for the reasons in the README section on forking.
# Tests, like check-mod, turn go.work off: derived repositories test against the versions in go.mod, and only
# this way shows whether a test passes only because of unreleased behavior in the local Keel source.
# The two frontend steps (Biome + tsc, vitest) need pnpm install run in web/ first.
# check-generated goes first: only once the generated files come from the current sources do the build, type
# check and tests that follow test the real thing; after adding a route file, tsc reports the path as missing
# unless the route tree is regenerated first. It needs buf installed (make init).
.PHONY: check
check:
	$(MAKE) check-generated
	@# Only files git knows (tracked + new, not ignored): recursing by directory would scan the full repository
	@# copies of subagents under .claude/worktrees/, which git treats as nested repositories and does not list.
	@bad=$$(gofmt -l $$(for f in $$(git ls-files -co --exclude-standard '*.go'); do [ -f "$$f" ] && echo "$$f"; done)); \
		test -z "$$bad" || { echo "$$bad"; echo "gofmt: the files above are not formatted"; exit 1; }
	go build ./...
	go vet ./...
	GOWORK=off go vet -tags integration ./...
	GOWORK=off go run $(GOLANGCI) run ./...
	GOWORK=off go test -race ./... -count=1
	buf lint
	$(MAKE) check-mod
	cd web && pnpm lint && pnpm test && pnpm build
	$(MAKE) lint-strict
	@# This template's own rule that its text is English; a derived repository deletes this line unless it wants the same.
	@scripts/check-english.sh
	$(MAKE) check-sync
