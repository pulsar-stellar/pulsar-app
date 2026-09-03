package store

import (
	"strings"
	"testing"
	"time"
)

// ADR-032: a timestamp reaches this package as a time.Time from Postgres, as
// SQLite's CURRENT_TIMESTAMP format from a defaulted column, or as RFC3339
// from this package's own writes. All three normalise to UTC.
func TestScanTimeAcceptsEveryShapeAColumnCanProduce(t *testing.T) {
	t.Parallel()

	want := time.Date(2026, 9, 1, 15, 28, 50, 0, time.UTC)

	cases := []struct {
		name string
		src  any
		want time.Time
	}{
		{"postgres time.Time", want, want},
		{"sqlite CURRENT_TIMESTAMP", "2026-09-01 15:28:50", want},
		{"our own RFC3339 write", "2026-09-01T15:28:50Z", want},
		{"RFC3339 with nanoseconds", "2026-09-01T15:28:50.123456789Z", want.Add(123456789)},
		{"RFC3339 with an offset", "2026-09-01T17:28:50+02:00", want},
		{"bytes rather than a string", []byte("2026-09-01T15:28:50Z"), want},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, err := scanTime(c.src)
			if err != nil {
				t.Fatalf("scanTime(%v): unexpected error: %v", c.src, err)
			}
			if !got.Equal(c.want) {
				t.Errorf("scanTime(%v) = %v, want %v", c.src, got, c.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("scanTime(%v) returned %v, want a UTC time", c.src, got.Location())
			}
		})
	}
}

// A shape this package does not recognise is an error rather than a guess. A
// wrong timestamp is not something to discover later from a wrong wire value.
func TestScanTimeRejectsWhatItCannotParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  any
		want string
	}{
		{"null", nil, "null"},
		{"a number", int64(1788255912), "not a time or a string"},
		{"an epoch as a string", "1788255912", "matches none of"},
		{"Go's String() output", "2026-09-01 15:29:26 +0000 UTC", "matches none of"},
		{"empty", "", "matches none of"},
		{"a date alone", "2026-09-01", "matches none of"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			_, err := scanTime(c.src)
			if err == nil {
				t.Fatalf("scanTime(%v) succeeded, want an error", c.src)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err.Error(), c.want)
			}
		})
	}
}

// The format binding uses must round trip through the scanner, or a value this
// package wrote would not read back as what it wrote.
func TestFormatTimeRoundTripsThroughScanTime(t *testing.T) {
	t.Parallel()

	original := time.Date(2026, 9, 1, 15, 28, 50, 123456789, time.UTC)

	formatted := formatTime(original)
	if !strings.HasSuffix(formatted, "Z") {
		t.Errorf("formatTime produced %q, which carries no UTC offset", formatted)
	}
	// Go's String() output is what SQLite silently stores if a time.Time is
	// bound directly. formatTime must not resemble it. See ADR-032.
	if strings.Contains(formatted, " ") {
		t.Errorf("formatTime produced %q, which looks like Go's String() rather than RFC3339", formatted)
	}

	back, err := scanTime(formatted)
	if err != nil {
		t.Fatalf("scanTime on formatTime's output: %v", err)
	}
	if !back.Equal(original) {
		t.Errorf("round trip gave %v, want %v", back, original)
	}
}

// A non-UTC input is normalised on the way out, so the wire never carries a
// local time.
func TestFormatTimeConvertsToUTC(t *testing.T) {
	t.Parallel()

	zone := time.FixedZone("UTC+2", 2*60*60)
	local := time.Date(2026, 9, 1, 17, 28, 50, 0, zone)

	if got, want := formatTime(local), "2026-09-01T15:28:50Z"; got != want {
		t.Errorf("formatTime = %q, want %q", got, want)
	}
}
