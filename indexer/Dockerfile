# The indexer image, per ADR-047.
#
# Build context is this directory, not the repository root: the indexer is its
# own Go module (ADR-002), so nothing from the pnpm workspace belongs here.
#
#   docker build -t pulsar-indexer ./indexer
#
# There is deliberately no `# syntax=` directive. That line makes BuildKit fetch
# and run an external build frontend from a mutable tag, and this file uses no
# syntax that needs one: COPY --from predates BuildKit, and there are no
# RUN --mount or heredoc forms. Adding one back means taking on that dependency,
# so pin it to a digest if a future change genuinely needs newer syntax.

# The tag is a major-minor floor rather than an exact patch. GOTOOLCHAIN=auto
# lets the `go` directive in go.mod select the exact toolchain, so go.mod stays
# the single source of the Go version (ADR-030) instead of this tag becoming a
# third place it lives, after go.mod itself and CI's go-version-file read of it.
FROM golang:1.26 AS builder

ENV GOTOOLCHAIN=auto
WORKDIR /src

# The version /health reports. internal/version keeps Version as a var
# specifically so a release build can stamp it at link time, and the point of
# that field is telling an operator which build is answering, so an image that
# shipped the "dev" fallback would defeat it. The default keeps a plain local
# build honest; the publish workflow passes the release tag.
#
# BuildKit exports this as an environment variable rather than substituting it
# into the command text, and a shell does not re-evaluate command substitution
# found inside a parameter's value, so a hostile value cannot execute here. It
# can still break the build: a value containing a space makes -ldflags a
# malformed flag and `go build` exits non-zero. The publish workflow therefore
# validates the tag against a strict pattern before passing it, so a bad tag
# fails in the workflow with a clear message rather than as a compile error.
ARG VERSION=dev

# Dependencies resolve in their own layer, so editing source does not refetch
# the module graph on every build.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 is set explicitly rather than inherited from the base image.
# ADR-008 chose a pure-Go SQLite driver precisely so the binary links no C
# toolchain, which is what makes the scratch final stage below possible at all;
# a builder that silently enabled cgo would produce a binary that cannot run
# there. -trimpath keeps build paths out of the binary, and -s -w drop the
# symbol table and DWARF data, which the daemon does not need in production.
#
# The migrations are compiled in by go:embed, so the image needs no SQL files
# and no separate migrate step: the daemon applies them itself at boot.
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X github.com/pulsar-stellar/pulsar-app/indexer/internal/version.Version=${VERSION}" \
      -o /out/pulsar-indexer \
      ./cmd/pulsar-indexer

FROM scratch

# ADR-008's stated payoff: nothing in the final image but the binary and the one
# file it cannot do without.
#
# The CA bundle is that file, and it is not optional. The daemon polls Soroban
# RPC over https, and a scratch image without it fails certificate verification
# on the first poll. That is a runtime failure no build check catches, which is
# why this image is run against testnet before release rather than only built.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/pulsar-indexer /pulsar-indexer

# Numeric because scratch carries no /etc/passwd for a name to resolve against.
# 65532 is the conventional non-root id, matching distroless.
USER 65532:65532

# Matches PULSAR_INDEXER_LISTEN_ADDR's default. Changing that variable means
# publishing a different port; this is documentation, not a binding.
EXPOSE 8080

# No HEALTHCHECK instruction: scratch has no shell and no HTTP client to run one
# with, and shipping a second static binary just to probe localhost would
# enlarge the image to buy what an orchestrator already does better. GET /health
# is public and unauthenticated (ADR-044), so compose and Render probe it over
# HTTP. Note that /health reports store reachability but not poll recency, so a
# container whose poller cannot reach RPC still answers 200; closing that is H5.
#
# The image holds no state and sets no WORKDIR, so the working directory is /.
# Postgres, the deployed engine, needs no filesystem writes at all. The SQLite
# path needs a writable mount and an absolute DSN pointing into it: .env.example
# ships the relative `file:./pulsar.db`, which resolves to /pulsar.db here and
# fails at migration time because UID 65532 cannot write to a scratch root. WAL
# mode also creates -wal and -shm siblings, so the mounted directory must be
# writable, not just a single mounted file, and since a scratch image has no
# /etc/group the process carries no supplementary groups, so a group-writable
# volume will not do: chown the host directory to 65532:65532.

LABEL org.opencontainers.image.source="https://github.com/pulsar-stellar/pulsar-app" \
      org.opencontainers.image.description="Pulsar indexer: polls Soroban RPC, decodes contract events, and serves them over REST and GraphQL" \
      org.opencontainers.image.licenses="Apache-2.0"

ENTRYPOINT ["/pulsar-indexer"]
