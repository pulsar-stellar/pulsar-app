package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/validate"
)

// contractStatsReader is the slice of the contracts store the health endpoint
// needs: one aggregate read. It stays its own interface so the health handler's
// dependency is documented as just this, even though the Server now holds the
// wider contractStore. *store.Contracts satisfies it.
type contractStatsReader interface {
	Stats(ctx context.Context) (store.ContractStats, error)
}

// contractStore is the slice of the contracts store the api package's handlers
// use, defined here where it is consumed so the package depends on the store
// only through the methods it calls and a test can substitute a fake without a
// database. It embeds contractStatsReader so /health keeps reading through the
// same value. *store.Contracts satisfies it.
type contractStore interface {
	contractStatsReader
	List(ctx context.Context) ([]models.Contract, error)
	Register(ctx context.Context, id string) (models.Contract, error)
	Get(ctx context.Context, id string) (models.Contract, error)
	Delete(ctx context.Context, id string) error
}

// maxRegisterBodyBytes caps the registration body the handler will read. The
// body is a single small JSON object, so the cap is generous while still
// bounding what an abusive client can make the handler buffer.
const maxRegisterBodyBytes = 64 << 10 // 64 KiB

// contractListPayload is the GET /contracts success body carried under the
// envelope's data. The SDK's listContracts reads data.items, so the records sit
// under an items key rather than as a bare array, matching ADR-021's list
// shape. Items is never nil on the wire: an empty table serializes as [], not
// null, so the SDK always sees an array.
type contractListPayload struct {
	Items []models.Contract `json:"items"`
}

// elapsedMs is the milliseconds since start, the figure every handler reports
// as meta.took_ms. It is microsecond-resolution so a sub-millisecond handler
// still reports a non-zero, non-negative number the SDK's schema accepts.
func elapsedMs(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000.0
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
	writeData(w, http.StatusOK, body, elapsedMs(start))
}

// handleListContracts answers GET /contracts with the tracked contracts under
// the envelope's data.items, oldest first, per ADR-021's list shape. An empty
// table is a 200 with an empty items array, not an error: "nothing tracked" is
// a valid state the SDK maps to an empty slice. A store failure is a 500 with
// the underlying error logged server-side and kept out of the response.
func (s *Server) handleListContracts(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	contracts, err := s.contracts.List(r.Context())
	if err != nil {
		s.log.ErrorContext(r.Context(), "contracts_list_failed",
			"request_id", RequestIDFromContext(r.Context()),
			"error", err.Error(),
		)
		writeError(w, apierror.New(apierror.CodeInternalStore, "The indexer could not list its contracts."))
		return
	}
	if contracts == nil {
		contracts = []models.Contract{}
	}
	writeData(w, http.StatusOK, contractListPayload{Items: contracts}, elapsedMs(start))
}

// handleRegisterContract answers POST /contracts. The body is {contract_id};
// the handler bounds and decodes it, validates the ID through the same
// validate.ContractID the config layer uses, then upserts. Registration is
// idempotent per ADR-018: registering a contract already tracked returns the
// existing record at 200, with no status change and no progress reset, so a
// client that retries a lost response is not punished. A malformed body is a
// 400 VALIDATION_BODY; a well-formed body with a bad ID is a 400
// VALIDATION_CONTRACT_ID; a store failure is a 500 with the cause logged.
func (s *Server) handleRegisterContract(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var body struct {
		ContractID string `json:"contract_id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRegisterBodyBytes))
	if err := dec.Decode(&body); err != nil {
		writeError(w, apierror.New(apierror.CodeValidationBody, `The request body must be a JSON object of the form {"contract_id": "C..."}.`))
		return
	}
	if err := validate.ContractID(body.ContractID); err != nil {
		writeError(w, apierror.New(apierror.CodeValidationContractID, "The contract_id is not a well-formed Soroban contract ID."))
		return
	}

	contract, err := s.contracts.Register(r.Context(), body.ContractID)
	if err != nil {
		s.log.ErrorContext(r.Context(), "contract_register_failed",
			"request_id", RequestIDFromContext(r.Context()),
			"error", err.Error(),
		)
		writeError(w, apierror.New(apierror.CodeInternalStore, "The indexer could not register the contract."))
		return
	}
	writeData(w, http.StatusOK, contract, elapsedMs(start))
}

// handleGetContract answers GET /contracts/:id with the contract's record, or,
// when the indexer is not tracking it, a 404 carrying a not_found envelope, the
// pairing ADR-019 requires so the SDK's getContract can return null. The path
// ID is validated first: a malformed ID is a 400 VALIDATION_CONTRACT_ID, which
// the SDK never triggers since it validates client-side, but a direct caller
// can. A store failure other than absence is a 500 with the cause logged.
func (s *Server) handleGetContract(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	id := chi.URLParam(r, "id")
	if err := validate.ContractID(id); err != nil {
		writeError(w, apierror.New(apierror.CodeValidationContractID, "The contract ID is not a well-formed Soroban contract ID."))
		return
	}

	contract, err := s.contracts.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apierror.New(apierror.CodeNotFoundContract, "The indexer is not tracking this contract."))
			return
		}
		s.log.ErrorContext(r.Context(), "contract_get_failed",
			"request_id", RequestIDFromContext(r.Context()),
			"error", err.Error(),
		)
		writeError(w, apierror.New(apierror.CodeInternalStore, "The indexer could not read the contract."))
		return
	}
	writeData(w, http.StatusOK, contract, elapsedMs(start))
}

// handleDeleteContract answers DELETE /contracts/:id. A successful delete is a
// 204 with no body, per ADR-039. Deleting a contract the indexer is not
// tracking is not an idempotent success but a 404 carrying a not_found
// envelope, the same structured absence signal GET sends: an operator who
// deletes the wrong ID learns it was already untracked rather than reading a
// silent success. The path ID is validated first, and a store failure other
// than absence is a 500 with the cause logged.
func (s *Server) handleDeleteContract(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := validate.ContractID(id); err != nil {
		writeError(w, apierror.New(apierror.CodeValidationContractID, "The contract ID is not a well-formed Soroban contract ID."))
		return
	}

	if err := s.contracts.Delete(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, apierror.New(apierror.CodeNotFoundContract, "The indexer is not tracking this contract."))
			return
		}
		s.log.ErrorContext(r.Context(), "contract_delete_failed",
			"request_id", RequestIDFromContext(r.Context()),
			"error", err.Error(),
		)
		writeError(w, apierror.New(apierror.CodeInternalStore, "The indexer could not delete the contract."))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMethodNotAllowed answers a request whose path exists but whose method
// does not with a 404 not_found envelope carrying NOT_FOUND_METHOD. ADR-017
// fixes the status set without a 405, so the mismatch collapses to 404; the
// catalog code keeps it distinct from the NOT_FOUND_ROUTE an unknown path gets.
func (s *Server) handleMethodNotAllowed(w http.ResponseWriter, _ *http.Request) {
	writeError(w, apierror.New(apierror.CodeNotFoundMethod, "The request path does not accept this method."))
}
