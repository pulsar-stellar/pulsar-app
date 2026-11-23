package api

import (
	"net/http"
	"time"
)

// rootPayload is the GET / body carried under the envelope's data: what this
// server is, which build, and where its surface and documentation live.
type rootPayload struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Links   map[string]string `json:"links"`
}

// handleRoot answers GET / with an index of the served surface, so the bare URL
// tells a visitor where to go instead of answering 404 (ADR-057). It reads no
// store. Relative links are this server's routes; only routes that exist are
// listed, and TestRootLinksResolve holds that. docs_site is the configured
// documentation site, left out when none is set.
func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	start := time.Now()
	links := map[string]string{
		"health":  "/health",
		"openapi": "/openapi.json",
		"graphql": "/graphql",
	}
	if s.docsURL != "" {
		links["docs_site"] = s.docsURL
	}
	writeData(w, http.StatusOK, rootPayload{
		Name:    "pulsar-indexer",
		Version: s.version,
		Links:   links,
	}, elapsedMs(start))
}
