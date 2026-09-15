// Package validate holds the input validators the indexer applies at its
// boundaries. Two boundaries check a contract ID: the config layer reading the
// bootstrap list at startup, and the HTTP handlers reading an ID from a request
// path or body. Sharing one validator keeps those two boundaries in agreement
// with each other, and keeps the indexer in agreement with @pulsar-stellar/sdk,
// whose ContractIdSchema enforces the same shape from the client side.
package validate

import (
	"errors"
	"regexp"
)

// ErrContractID is the error ContractID returns for a malformed ID. It is a
// sentinel so a caller can branch on the failure with errors.Is rather than
// matching a message: the config layer wraps it with the offending variable and
// position, and an HTTP handler maps it to a validation-class apierror.
var ErrContractID = errors.New("not a well-formed contract ID")

// contractIDPattern matches a Soroban contract ID: the letter C followed by 55
// base32 (RFC 4648) characters, 56 in total. It is the same pattern
// @pulsar-stellar/sdk's ContractIdSchema enforces, so the indexer and the
// published client never disagree on whether an ID is well-formed.
//
// \A and \z anchor the whole string, not \A...$ , so a value with a trailing
// newline is rejected rather than accepted on its first line. This is a shape
// check, not a claim that the contract exists or that its strkey checksum is
// valid: it rejects the common mistakes, a truncated paste or an account ID
// beginning with G, before they reach the store.
var contractIDPattern = regexp.MustCompile(`\AC[A-Z2-7]{55}\z`)

// ContractID returns nil when id is a well-formed Soroban contract ID, and
// ErrContractID when it is not. It never panics, so it is safe on a request
// path where a panic would become a 500.
func ContractID(id string) error {
	if !contractIDPattern.MatchString(id) {
		return ErrContractID
	}
	return nil
}
