package validate_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/validate"
)

// overflowDigits is a run of digits far larger than an int64 holds, used to
// prove the numeric validators reject a value that is shaped like a number but
// names no storable id or bound.
var overflowDigits = strings.Repeat("9", 40)

func TestEventLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want int
		err  error
	}{
		{"empty is the default page size", "", 50, nil},
		{"minimum", "1", 1, nil},
		{"mid range", "50", 50, nil},
		{"maximum", "500", 500, nil},
		{"leading zeros parse", "050", 50, nil},

		{"zero is below the minimum", "0", 0, validate.ErrLimit},
		{"above the maximum", "501", 0, validate.ErrLimit},
		{"negative", "-1", 0, validate.ErrLimit},
		{"leading plus", "+5", 0, validate.ErrLimit},
		{"not a number", "abc", 0, validate.ErrLimit},
		{"decimal", "1.5", 0, validate.ErrLimit},
		{"leading space", " 5", 0, validate.ErrLimit},
		{"trailing space", "5 ", 0, validate.ErrLimit},
		{"overflows int", overflowDigits, 0, validate.ErrLimit},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := validate.EventLimit(tc.raw)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("EventLimit(%q) error = %v, want %v", tc.raw, err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("EventLimit(%q) = %v, want nil", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("EventLimit(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestEventOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
		err  error
	}{
		{"empty defaults to descending", "", "desc", nil},
		{"ascending", "asc", "asc", nil},
		{"descending", "desc", "desc", nil},

		{"uppercase is rejected", "ASC", "", validate.ErrOrder},
		{"title case is rejected", "Desc", "", validate.ErrOrder},
		{"spelled out is rejected", "ascending", "", validate.ErrOrder},
		{"numeric is rejected", "1", "", validate.ErrOrder},
		{"other word is rejected", "up", "", validate.ErrOrder},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := validate.EventOrder(tc.raw)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("EventOrder(%q) error = %v, want %v", tc.raw, err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("EventOrder(%q) = %v, want nil", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("EventOrder(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestEventCursor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
		err  error
	}{
		{"empty means no cursor", "", "", nil},
		{"digits", "12345", "12345", nil},
		{"zero is well-formed", "0", "0", nil},

		{"not a number", "abc", "", validate.ErrCursor},
		{"negative", "-5", "", validate.ErrCursor},
		{"decimal", "12.3", "", validate.ErrCursor},
		{"leading space", " 12", "", validate.ErrCursor},
		{"overflows an event id", overflowDigits, "", validate.ErrCursor},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := validate.EventCursor(tc.raw)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("EventCursor(%q) error = %v, want %v", tc.raw, err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("EventCursor(%q) = %v, want nil", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("EventCursor(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestLedgerBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want int64
		err  error
	}{
		{"empty is unset", "", 0, nil},
		{"zero", "0", 0, nil},
		{"positive", "100", 100, nil},

		{"negative", "-1", 0, validate.ErrLedgerBound},
		{"not a number", "abc", 0, validate.ErrLedgerBound},
		{"decimal", "10.0", 0, validate.ErrLedgerBound},
		{"leading space", " 10", 0, validate.ErrLedgerBound},
		{"overflows int64", overflowDigits, 0, validate.ErrLedgerBound},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := validate.LedgerBound(tc.raw)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("LedgerBound(%q) error = %v, want %v", tc.raw, err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LedgerBound(%q) = %v, want nil", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("LedgerBound(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestLedgerRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		from, to int64
		ok       bool
	}{
		{"both unset", 0, 0, true},
		{"lower unset", 0, 100, true},
		{"upper unset holds no ceiling", 100, 0, true},
		{"ascending window", 5, 10, true},
		{"equal bounds", 10, 10, true},
		{"inverted window is rejected", 10, 5, false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validate.LedgerRange(tc.from, tc.to)
			if tc.ok {
				if err != nil {
					t.Fatalf("LedgerRange(%d, %d) = %v, want nil", tc.from, tc.to, err)
				}
				return
			}
			if !errors.Is(err, validate.ErrLedgerRange) {
				t.Fatalf("LedgerRange(%d, %d) error = %v, want %v", tc.from, tc.to, err, validate.ErrLedgerRange)
			}
		})
	}
}

func TestEventID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want int64
		err  error
	}{
		{"one", "1", 1, nil},
		{"many digits", "12345", 12345, nil},
		{"zero is well-formed", "0", 0, nil},

		{"empty", "", 0, validate.ErrEventID},
		{"not a number", "abc", 0, validate.ErrEventID},
		{"negative", "-5", 0, validate.ErrEventID},
		{"decimal", "12.3", 0, validate.ErrEventID},
		{"hex", "0x1F", 0, validate.ErrEventID},
		{"leading space", " 12", 0, validate.ErrEventID},
		{"overflows a BIGSERIAL", overflowDigits, 0, validate.ErrEventID},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := validate.EventID(tc.raw)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("EventID(%q) error = %v, want %v", tc.raw, err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("EventID(%q) = %v, want nil", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("EventID(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}
