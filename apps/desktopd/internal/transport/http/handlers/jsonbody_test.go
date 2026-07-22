package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
)

// newJSONRequest builds a test request with Content-Type: application/json set,
// matching what real API clients send and what decodeJSONBody now requires
// (review R-01, RW-01).
func newJSONRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Content-Type", "application/json")
	return req
}
