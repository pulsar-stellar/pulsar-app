package api

import (
	"context"
	"net/http"
	"time"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
)

// contractStatsReader is the slice of the contracts store the health endpoint
// needs: one aggregate read. Defining it here, where it is consumed, keeps the
// api package's dependency on the store as narrow as the handler's need and
// lets a test substitute a fake without a database. *store.Contracts satisfies
// it.
type contractStatsReader interface {
	Stats(ctx context.Context) (store.ContractStats, error)
}

// healthPayload is the /health success body carried under the envelope's data.
// It matches the SDK's HealthPayloadSchema: ok is the liveness flag, version
// the build, and the two counts the indexer's progress. latest_ledger and
// tracked_contracts are real numbers here rather than the schema's permitted
// null, since the store reports a zero for an empty table.
type healthPayload struct {
	OK               bool   `json:"ok"`
	Version          string `json:"version"`
	LatestLedger     int64  `json:"latest_ledger"`
	TrackedContracts int    `json:"tracked_contracts"`
}

// handleHealth reports whether the indexer can answer, and how far it has
// indexed. It reads the store rather than returning a static ok:true, so the
// response is a real readiness signal: when the store read fails the indexer
// cannot report its state, so the handler returns a 500 internal envelope
// rather than a 200 with fabricated zeros. The underlying error can name a host
// or carry store internals, so it is logged server-side against this request
// and never sent to the client.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	stats, err := s.contracts.Stats(r.Context())
	if err != nil {
		s.log.ErrorContext(r.Context(), "health_stats_failed",
			"request_id", RequestIDFromContext(r.Context()),
			"error", err.Error(),
		)
		writeError(w, apierror.New(apierror.CodeInternalStore, "The indexer could not read its status."))
		return
	}

	body := healthPayload{
		OK:               true,
		Version:          s.version,
		LatestLedger:     stats.LatestLedger,
		TrackedContracts: stats.Count,
	}
	writeData(w, http.StatusOK, body, float64(time.Since(start).Microseconds())/1000.0)
}

// handleMethodNotAllowed answers a request whose path exists but whose method
// does not with a 404 not_found envelope carrying NOT_FOUND_METHOD. ADR-017
// fixes the status set without a 405, so the mismatch collapses to 404; the
// catalog code keeps it distinct from the NOT_FOUND_ROUTE an unknown path gets.
func (s *Server) handleMethodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	writeError(w, apierror.New(apierror.CodeNotFoundMethod, "The request path does not accept this method."))
}
