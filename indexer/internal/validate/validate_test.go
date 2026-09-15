package validate_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/validate"
)

// showcaseContract is pulsar-core's deployed showcase contract, a real
// well-formed ID, used here as the canonical valid case.
const showcaseContract = "CDNWTVUDKCCGW7GOC6SBLUFXXUCD2YDHWRDUSXZ6CYBQKQWLCUYYWI5L"

func TestContractID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		ok   bool
	}{
		{"showcase contract", showcaseContract, true},
		{"all-A body", "C" + strings.Repeat("A", 55), true},
		{"only base32 digits in body", "C" + strings.Repeat("2", 27) + strings.Repeat("7", 28), true},

		{"empty", "", false},
		{"account id prefix G", "G" + strings.Repeat("A", 55), false},
		{"lowercase c prefix", "c" + strings.Repeat("A", 55), false},
		{"too short", "CDNW", false},
		{"one char short", "C" + strings.Repeat("A", 54), false},
		{"one char long", showcaseContract + "X", false},
		{"lowercase body", "C" + strings.Repeat("a", 55), false},
		{"base32 excludes 0", "C" + strings.Repeat("0", 55), false},
		{"base32 excludes 1", "C" + strings.Repeat("1", 55), false},
		{"base32 excludes 8", "C" + strings.Repeat("8", 55), false},
		{"leading space", " " + showcaseContract, false},
		{"trailing space", showcaseContract + " ", false},
		// A trailing newline must not slip through on the first line: the anchor
		// is the end of the string, not the end of a line.
		{"trailing newline", showcaseContract + "\n", false},
		{"embedded newline", "C" + strings.Repeat("A", 27) + "\n" + strings.Repeat("A", 27), false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validate.ContractID(tc.id)
			if tc.ok {
				if err != nil {
					t.Fatalf("ContractID(%q) = %v, want nil", tc.id, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ContractID(%q) = nil, want an error", tc.id)
			}
			if !errors.Is(err, validate.ErrContractID) {
				t.Fatalf("ContractID(%q) error = %v, want it to be ErrContractID", tc.id, err)
			}
		})
	}
}
