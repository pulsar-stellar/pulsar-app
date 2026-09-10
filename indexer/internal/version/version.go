// Package version carries the indexer's build version, which the /health
// endpoint reports so an operator can tell which build is answering.
//
// Version is a var, not a const, so a release build can stamp it without
// editing source:
//
//	go build -ldflags "-X github.com/pulsar-stellar/pulsar-app/indexer/internal/version.Version=0.1.0"
//
// An unstamped build (a local `go run`, a test binary) keeps the "dev"
// fallback rather than an empty string, which the SDK's HealthPayloadSchema
// rejects.
package version

// Version is the build version reported by /health. It is overwritten at link
// time on a release build and stays "dev" otherwise.
var Version = "dev"
