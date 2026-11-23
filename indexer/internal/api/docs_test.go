package api

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
)

// wantDocsCSP is the policy every /docs response carries (ADR-059). Scripts and
// styles load only from this server, nothing is framed, and no form or base
// URL can point elsewhere.
const wantDocsCSP = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// docsAssetCases is every asset /docs serves, with the content type it must
// carry and the embedded file it must return byte for byte.
var docsAssetCases = []struct {
	name, contentType, file string
}{
	{"swagger-ui-bundle.js", "text/javascript; charset=utf-8", "docsui/vendor/swagger-ui-bundle.js"},
	{"swagger-ui.css", "text/css; charset=utf-8", "docsui/vendor/swagger-ui.css"},
	{"init.js", "text/javascript; charset=utf-8", "docsui/init.js"},
	{"LICENSE", "text/plain; charset=utf-8", "docsui/vendor/LICENSE"},
	{"NOTICE", "text/plain; charset=utf-8", "docsui/vendor/NOTICE"},
	{"swagger-ui-bundle.js.LICENSE.txt", "text/plain; charset=utf-8", "docsui/vendor/swagger-ui-bundle.js.LICENSE.txt"},
}

func TestDocsServesThePage(t *testing.T) {
	t.Parallel()

	rr := serve(probeServer(), http.MethodGet, "/docs", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /docs = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
	body := rr.Body.String()
	for _, ref := range []string{
		`href="/docs/assets/swagger-ui.css"`,
		`src="/docs/assets/swagger-ui-bundle.js"`,
		`src="/docs/assets/init.js"`,
	} {
		if !strings.Contains(body, ref) {
			t.Errorf("page does not reference %s", ref)
		}
	}
}

// The page carries no inline script and no inline style, so it runs under
// script-src 'self' and style-src 'self' with no exception.
func TestDocsPageHasNoInlineCode(t *testing.T) {
	t.Parallel()

	page, err := docsUI.ReadFile("docsui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	scripts := regexp.MustCompile(`(?is)<script([^>]*)>(.*?)</script>`).FindAllSubmatch(page, -1)
	if len(scripts) == 0 {
		t.Fatal("page has no script tags")
	}
	for _, m := range scripts {
		if !bytes.Contains(m[1], []byte("src=")) || len(bytes.TrimSpace(m[2])) != 0 {
			t.Errorf("inline script found: %s", m[0])
		}
	}
	for _, bad := range []string{"<style", "style=", "javascript:"} {
		if bytes.Contains(bytes.ToLower(page), []byte(bad)) {
			t.Errorf("page contains %q", bad)
		}
	}
	if regexp.MustCompile(`(?i)\son[a-z]+\s*=`).Match(page) {
		t.Error("page has an inline event handler")
	}
}

func TestDocsServesEachAsset(t *testing.T) {
	t.Parallel()

	srv := probeServer()
	for _, tc := range docsAssetCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rr := serve(srv, http.MethodGet, "/docs/assets/"+tc.name, "")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
			if ct := rr.Header().Get("Content-Type"); ct != tc.contentType {
				t.Errorf("Content-Type = %q, want %q", ct, tc.contentType)
			}
			want, err := docsUI.ReadFile(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(rr.Body.Bytes(), want) {
				t.Errorf("body is not the embedded %s", tc.file)
			}
		})
	}
}

// Every /docs response, the page and each asset, carries the strict CSP,
// nosniff, and no-referrer.
func TestDocsSetsStrictHeaders(t *testing.T) {
	t.Parallel()

	srv := probeServer()
	targets := []string{"/docs"}
	for _, tc := range docsAssetCases {
		targets = append(targets, "/docs/assets/"+tc.name)
	}
	for _, target := range targets {
		h := serve(srv, http.MethodGet, target, "").Header()
		if got := h.Get("Content-Security-Policy"); got != wantDocsCSP {
			t.Errorf("%s Content-Security-Policy = %q, want %q", target, got, wantDocsCSP)
		}
		if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s X-Content-Type-Options = %q, want nosniff", target, got)
		}
		if got := h.Get("Referrer-Policy"); got != "no-referrer" {
			t.Errorf("%s Referrer-Policy = %q, want no-referrer", target, got)
		}
	}
}

// Only the listed assets are served. Our own page, the vendoring metadata, an
// unknown name, and a path that tries to climb out of the asset directory all
// answer the ADR-017 404.
func TestDocsUnknownAssetIs404(t *testing.T) {
	t.Parallel()

	srv := probeServer()
	for _, target := range []string{
		"/docs/assets/nope.js",
		"/docs/assets/index.html",
		"/docs/assets/VERSION",
		"/docs/assets/SHA256SUMS",
		"/docs/assets/..%2Fdocs.go",
		"/docs/assets/vendor%2Fswagger-ui.css",
		"/docs/assets/",
	} {
		rr := serve(srv, http.MethodGet, target, "")
		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rr.Code)
			continue
		}
		var env decodedError
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Errorf("GET %s body is not the JSON envelope: %v", target, err)
			continue
		}
		if env.Error.Code != string(apierror.ClassNotFound) {
			t.Errorf("GET %s error.code = %q, want not_found", target, env.Error.Code)
		}
	}
}

// The vendored files are exactly what SHA256SUMS records, so a hand edit to the
// bundle, or a refresh that skipped the script, fails CI (ADR-059).
func TestVendoredAssetsMatchSHA256SUMS(t *testing.T) {
	t.Parallel()

	sums, err := docsUI.ReadFile("docsui/vendor/SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			t.Fatalf("malformed SHA256SUMS line %q", scanner.Text())
		}
		want, name := fields[0], fields[1]
		data, err := docsUI.ReadFile("docsui/vendor/" + name)
		if err != nil {
			t.Errorf("SHA256SUMS lists %s, which is not embedded: %v", name, err)
			continue
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s sha256 = %s, SHA256SUMS says %s", name, got, want)
		}
		listed[name] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	entries, err := fs.ReadDir(docsUI, "docsui/vendor")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if name := e.Name(); name != "SHA256SUMS" && name != "VERSION" && !listed[name] {
			t.Errorf("docsui/vendor/%s is embedded but not in SHA256SUMS", name)
		}
	}

	version, err := docsUI.ReadFile("docsui/vendor/VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`\A\d+\.\d+\.\d+\n\z`).Match(version) {
		t.Errorf("VERSION = %q, want a plain version and a newline", version)
	}
}

// "Try it out" exists for GET only. The initialiser names exactly one submit
// method list, and that list is ["get"], so no write endpoint gets a button.
func TestDocsInitialiserSubmitsOnlyGET(t *testing.T) {
	t.Parallel()

	init, err := docsUI.ReadFile("docsui/init.js")
	if err != nil {
		t.Fatal(err)
	}
	lists := regexp.MustCompile(`supportedSubmitMethods\s*:\s*\[([^\]]*)\]`).FindAllSubmatch(init, -1)
	if len(lists) != 1 {
		t.Fatalf("found %d supportedSubmitMethods lists, want exactly 1", len(lists))
	}
	methods := regexp.MustCompile(`"([^"]*)"|'([^']*)'`).FindAllSubmatch(lists[0][1], -1)
	if len(methods) != 1 || !(string(methods[0][1]) == "get" || string(methods[0][2]) == "get") {
		t.Errorf("supportedSubmitMethods = [%s], want [\"get\"] only", lists[0][1])
	}
	for _, want := range []string{
		`url: "/openapi.json"`, "validatorUrl: null", "queryConfigEnabled: false",
		"authorizeBtn: nothing", "authorizeOperationBtn: nothing", "plugins: [hideAuthorize]",
		"deepLinking: false",
	} {
		if !bytes.Contains(init, []byte(want)) {
			t.Errorf("init.js does not set %s", want)
		}
	}
}

// /docs and its assets are public reads, so they spend from the read bucket.
func TestDocsIsReadRateLimited(t *testing.T) {
	t.Parallel()

	for _, target := range []string{"/docs", "/docs/assets/init.js"} {
		srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3", WithReadRateLimit(1, 1))
		serve(srv, http.MethodGet, target, "")
		if rr := serve(srv, http.MethodGet, target, ""); rr.Code != http.StatusTooManyRequests {
			t.Errorf("second GET %s = %d, want 429", target, rr.Code)
		}
	}
}

// Each asset carries an ETag of its content, and a request that presents it
// gets a 304 with no body, so a repeat view doesn't resend the 1.7 MB bundle.
func TestDocsAssetsRevalidateWithETag(t *testing.T) {
	t.Parallel()

	srv := probeServer()
	first := serve(srv, http.MethodGet, "/docs/assets/swagger-ui-bundle.js", "")
	etag := first.Header().Get("ETag")
	data, err := docsUI.ReadFile("docsui/vendor/swagger-ui-bundle.js")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if want := `"` + hex.EncodeToString(sum[:]) + `"`; etag != want {
		t.Fatalf("ETag = %q, want the content's sha256 %q", etag, want)
	}

	req := httptest.NewRequest(http.MethodGet, "/docs/assets/swagger-ui-bundle.js", nil)
	req.Header.Set("If-None-Match", etag)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotModified {
		t.Errorf("conditional GET = %d, want 304", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("304 carried a %d-byte body", rr.Body.Len())
	}
	if got := rr.Header().Get("Content-Security-Policy"); got != wantDocsCSP {
		t.Errorf("304 Content-Security-Policy = %q, want the docs policy", got)
	}
}

// The docs headers ride on every /docs response the docs routes produce,
// including a 429 from the read bucket, and framing is refused for browsers
// that predate frame-ancestors too.
func TestDocsHeadersCoverRateLimitedAndFraming(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, fakeContracts{}, fakeEvents{}, "1.2.3", WithReadRateLimit(1, 1))
	serve(srv, http.MethodGet, "/docs", "")
	rr := serve(srv, http.MethodGet, "/docs", "")
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("second GET /docs = %d, want 429", rr.Code)
	}
	if got := rr.Header().Get("Content-Security-Policy"); got != wantDocsCSP {
		t.Errorf("429 Content-Security-Policy = %q, want the docs policy", got)
	}
	for _, target := range []string{"/docs", "/docs/assets/init.js"} {
		if got := serve(probeServer(), http.MethodGet, target, "").Header().Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("%s X-Frame-Options = %q, want DENY", target, got)
		}
	}
}
