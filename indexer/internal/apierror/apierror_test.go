package apierror

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// isUpperSnake reports whether s is a non-empty run of A-Z, 0-9, and
// underscore, which is the shape ADR-038 fixes for a catalog code.
func isUpperSnake(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}

// classStatus is the HTTP status each wire class must carry. It is the
// invariant a registry typo would break: a validation code with a 500, say.
var classStatus = map[Class]int{
	ClassValidation:  http.StatusBadRequest,
	ClassNotFound:    http.StatusNotFound,
	ClassRateLimited: http.StatusTooManyRequests,
	ClassInternal:    http.StatusInternalServerError,
}

// Every registered code must name one of the four wire classes, carry the
// status that class implies, and document a meaning. This is the completeness
// check ADR-038 promises: the catalog cannot drift into an entry a consumer or
// docs/error-codes.md cannot use.
func TestRegistryEntriesAreComplete(t *testing.T) {
	t.Parallel()

	for _, code := range Codes() {
		code := code
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()

			if !isUpperSnake(string(code)) {
				t.Errorf("code %q is not UPPER_SNAKE", code)
			}

			class := code.Class()
			wantStatus, known := classStatus[class]
			if !known {
				t.Fatalf("code %q has class %q, which is not one of the four wire classes", code, class)
			}
			if got := code.Status(); got != wantStatus {
				t.Errorf("code %q class %q: status = %d, want %d", code, class, got, wantStatus)
			}
			if strings.TrimSpace(code.Meaning()) == "" {
				t.Errorf("code %q has no meaning; docs/error-codes.md would have nothing to publish", code)
			}
		})
	}
}

func TestCodesIsSortedAndCoversTheRegistry(t *testing.T) {
	t.Parallel()

	got := Codes()
	if len(got) != len(registry) {
		t.Fatalf("Codes() returned %d codes, registry has %d", len(got), len(registry))
	}
	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i] < got[j] }) {
		t.Errorf("Codes() is not sorted: %v", got)
	}
	for _, c := range got {
		if !c.Registered() {
			t.Errorf("Codes() returned %q, which reports itself unregistered", c)
		}
	}
}

func TestKnownCodeReportsItsRegisteredValues(t *testing.T) {
	t.Parallel()

	if got := CodeInternalPanic.Class(); got != ClassInternal {
		t.Errorf("Class() = %q, want %q", got, ClassInternal)
	}
	if got := CodeInternalPanic.Status(); got != http.StatusInternalServerError {
		t.Errorf("Status() = %d, want %d", got, http.StatusInternalServerError)
	}
	if CodeInternalPanic.Meaning() == "" {
		t.Error("Meaning() is empty for a registered code")
	}
	if !CodeInternalPanic.Registered() {
		t.Error("Registered() = false for a registered code")
	}
}

func TestNotFoundRouteMapsToNotFound404(t *testing.T) {
	t.Parallel()

	if got := CodeNotFoundRoute.Class(); got != ClassNotFound {
		t.Errorf("Class() = %q, want %q", got, ClassNotFound)
	}
	if got := CodeNotFoundRoute.Status(); got != http.StatusNotFound {
		t.Errorf("Status() = %d, want %d", got, http.StatusNotFound)
	}
	if !CodeNotFoundRoute.Registered() {
		t.Error("Registered() = false for a registered code")
	}
}

// An unregistered code must degrade to internal/500, never to an empty class
// or a zero status, because either would reach the wire as something the SDK
// cannot validate.
func TestUnregisteredCodeDegradesToInternal(t *testing.T) {
	t.Parallel()

	const stray = Code("NOT_IN_THE_CATALOG")

	if got := stray.Class(); got != ClassInternal {
		t.Errorf("Class() = %q, want %q for an unregistered code", got, ClassInternal)
	}
	if got := stray.Status(); got != http.StatusInternalServerError {
		t.Errorf("Status() = %d, want %d for an unregistered code", got, http.StatusInternalServerError)
	}
	if got := stray.Meaning(); got != "" {
		t.Errorf("Meaning() = %q, want empty for an unregistered code", got)
	}
	if stray.Registered() {
		t.Error("Registered() = true for an unregistered code")
	}
}

func TestNewCarriesCodeClassStatusAndMessage(t *testing.T) {
	t.Parallel()

	err := New(CodeInternalPanic, "boom")

	if err.Code() != CodeInternalPanic {
		t.Errorf("Code() = %q, want %q", err.Code(), CodeInternalPanic)
	}
	if err.Class() != ClassInternal {
		t.Errorf("Class() = %q, want %q", err.Class(), ClassInternal)
	}
	if err.Status() != http.StatusInternalServerError {
		t.Errorf("Status() = %d, want %d", err.Status(), http.StatusInternalServerError)
	}
	if err.Message() != "boom" {
		t.Errorf("Message() = %q, want %q", err.Message(), "boom")
	}
	if !strings.Contains(err.Error(), "INTERNAL_PANIC") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("Error() = %q, want it to contain the code and message", err.Error())
	}
}

func TestWrapPreservesTheCauseChain(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("underlying failure")
	err := Wrap(CodeInternalPanic, "while handling request", sentinel)

	if !errors.Is(err, sentinel) {
		t.Error("errors.Is could not find the wrapped cause")
	}
	if got := errors.Unwrap(err); got != sentinel {
		t.Errorf("Unwrap() = %v, want the sentinel", got)
	}
	if !strings.Contains(err.Error(), sentinel.Error()) {
		t.Errorf("Error() = %q, want it to include the cause", err.Error())
	}
}

// A caller must be able to recover the catalogued error from a chain that a
// higher layer has wrapped further, which is how the responder will classify
// an error a handler returned.
func TestErrorIsRecoverableWithErrorsAs(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("request failed: %w", New(CodeInternalPanic, "boom"))

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatal("errors.As could not recover the *Error from the chain")
	}
	if apiErr.Code() != CodeInternalPanic {
		t.Errorf("recovered Code() = %q, want %q", apiErr.Code(), CodeInternalPanic)
	}
}
