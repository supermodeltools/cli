// Package api provides the HTTP client and base request primitives for the
// Supermodel API. It handles authentication headers, idempotency keys, error
// parsing, and response decoding.
//
// This is a shared kernel package. It must contain no business logic.
// Slice packages under internal/ may import it freely.
package api
