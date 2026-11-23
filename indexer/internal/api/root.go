package api

import (
	"net/http"
	"time"
)

// docsSiteURL is the published documentation site, which the root index links
// to (ADR-057).
const docsSiteURL = "https://emeditweb.gitbook.io/pulsar-stellar-sdk"

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
// listed, and TestRootLinksResolve holds that.
func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	start := time.Now()
	writeData(w, http.StatusOK, rootPayload{
		Name:    "pulsar-indexer",
		Version: s.version,
		Links: map[string]string{
			"health":    "/health",
			"graphql":   "/graphql",
			"docs_site": docsSiteURL,
		},
	}, elapsedMs(start))
}
