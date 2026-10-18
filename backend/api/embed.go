// Package api holds the API contract, openapi.yaml. The binary embeds it to validate requests (internal/httpapi),
// to serve it in dev mode, and to validate seed-admin input against the same schemas.
package api

import _ "embed"

// Spec is openapi.yaml.
//
//go:embed openapi.yaml
var Spec []byte
