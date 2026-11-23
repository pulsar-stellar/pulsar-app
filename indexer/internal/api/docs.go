package api

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

// docsUI holds the /docs page, its initialiser, and the vendored Swagger UI
// release with its licences and checksums (ADR-059).
//
//go:embed docsui
var docsUI embed.FS

// docsCSP is the policy on every /docs response. Scripts and styles load only
// from this server, the page talks only to this server, nothing may frame it,
// and no form or base URL can point elsewhere. Swagger UI runs under it with no
// inline exception: its styles arrive through the stylesheet and the DOM's
// style properties, which style-src does not govern.
const docsCSP = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// docsAsset is one file GET /docs/assets/{file} serves.
type docsAsset struct {
	path        string
	contentType string
}

// docsAssets is the allowlist of what /docs/assets serves, keyed by the name in
// the URL. Anything else, including the vendoring metadata and the page itself,
// is a 404, so no request can reach a file by path.
var docsAssets = map[string]docsAsset{
	"swagger-ui-bundle.js":             {"docsui/vendor/swagger-ui-bundle.js", "text/javascript; charset=utf-8"},
	"swagger-ui.css":                   {"docsui/vendor/swagger-ui.css", "text/css; charset=utf-8"},
	"init.js":                          {"docsui/init.js", "text/javascript; charset=utf-8"},
	"LICENSE":                          {"docsui/vendor/LICENSE", "text/plain; charset=utf-8"},
	"NOTICE":                           {"docsui/vendor/NOTICE", "text/plain; charset=utf-8"},
	"swagger-ui-bundle.js.LICENSE.txt": {"docsui/vendor/swagger-ui-bundle.js.LICENSE.txt", "text/plain; charset=utf-8"},
}

// docsHeaders sets the /docs security headers on the page and every asset,
// including the 404 for an unknown asset and a 429 from the read bucket: it is
// mounted ahead of the rate limiter. X-Frame-Options repeats frame-ancestors
// for browsers that predate it. nosniff comes from SecurityHeaders, which wraps
// the whole surface.
func docsHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", docsCSP)
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// docsETags caches each embedded file's strong ETag, its quoted sha256, so the
// 1.7 MB bundle is hashed once per process rather than once per request.
var docsETags sync.Map // path -> string

func docsETag(path string, data []byte) string {
	if etag, ok := docsETags.Load(path); ok {
		return etag.(string)
	}
	sum := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	docsETags.Store(path, etag)
	return etag
}

// handleDocs serves the Swagger UI page for the indexer API (ADR-059). It is
// revalidated on each visit so a release's new asset references take effect.
func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	s.serveEmbedded(w, r, "docsui/index.html", "text/html; charset=utf-8", "no-cache")
}

// handleDocsAsset serves one allowlisted /docs asset.
func (s *Server) handleDocsAsset(w http.ResponseWriter, r *http.Request) {
	asset, ok := docsAssets[chi.URLParam(r, "file")]
	if !ok {
		s.handleNotFound(w, r)
		return
	}
	s.serveEmbedded(w, r, asset.path, asset.contentType, "public, max-age=3600")
}

// serveEmbedded writes one embedded file with a content ETag. http.ServeContent
// answers a matching If-None-Match with a 304 and no body, so a repeat view
// costs a token but not the transfer. A read failure means the binary was
// built without a file it embeds, which the tests rule out; it still answers
// with the internal envelope rather than panicking.
func (s *Server) serveEmbedded(w http.ResponseWriter, r *http.Request, path, contentType, cacheControl string) {
	data, err := docsUI.ReadFile(path)
	if err != nil {
		s.log.ErrorContext(r.Context(), "docs_asset_missing",
			"request_id", RequestIDFromContext(r.Context()),
			"path", path,
			"error", err.Error(),
		)
		writeError(w, apierror.New(apierror.CodeInternalPanic, "The indexer could not serve its API docs."))
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", docsETag(path, data))
	http.ServeContent(w, r, path, time.Time{}, bytes.NewReader(data))
}
