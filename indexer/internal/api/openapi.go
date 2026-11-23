package api

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

// openAPISpec is the hand-written OpenAPI document for the REST surface
// (ADR-058), embedded so the binary serves exactly what CI checked.
//
//go:embed openapi.json
var openAPISpec []byte

// versionedSpec returns the embedded document with info.version set to
// version, so the served spec never claims a different version from the
// binary. Key order may change; the content does not.
func versionedSpec(version string) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(openAPISpec, &doc); err != nil {
		return nil, fmt.Errorf("openapi: parse document: %w", err)
	}
	var info map[string]any
	if err := json.Unmarshal(doc["info"], &info); err != nil {
		return nil, fmt.Errorf("openapi: parse info: %w", err)
	}
	info["version"] = version
	rawInfo, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("openapi: encode info: %w", err)
	}
	doc["info"] = rawInfo
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("openapi: encode document: %w", err)
	}
	return out, nil
}

// handleOpenAPI serves the OpenAPI document. It is the one JSON route outside
// the ADR-017 envelope, because OpenAPI tools expect the bare document. The
// document is built once per server and reused.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	s.openAPIOnce.Do(func() {
		s.openAPIDoc, s.openAPIErr = versionedSpec(s.version)
	})
	if s.openAPIErr != nil {
		s.log.ErrorContext(r.Context(), "openapi_build_failed",
			"request_id", RequestIDFromContext(r.Context()),
			"error", s.openAPIErr.Error(),
		)
		writeError(w, apierror.New(apierror.CodeInternalPanic, "The indexer could not serve its API description."))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// The document changes only with a release, so clients may reuse it for a
	// few minutes instead of spending the shared read bucket on each fetch.
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.openAPIDoc)
}
