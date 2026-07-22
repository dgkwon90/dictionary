package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
)

// errUnsupportedMediaType signals a Content-Type other than application/json
// (review R-01: JSON endpoints previously accepted any Content-Type, including
// text/plain, which some cross-origin request paths can send without triggering
// a browser CORS preflight).
var errUnsupportedMediaType = errors.New("Content-Type must be application/json")

// decodeJSONBody reads and decodes exactly one JSON value from the request body,
// capped at maxBytes, rejecting unknown fields and any trailing content after that
// single value. json.Decoder.Decode on its own only consumes one JSON value and
// silently ignores anything after it (e.g. a body smuggled behind
// `{...}{"ignored":"payload"}`), so callers across handlers previously had no
// defense against that (review R-01/R-08, RW-02). It also takes over closing
// r.Body — callers must not close it themselves.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64, log *slog.Logger) error {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return errUnsupportedMediaType
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	defer func() {
		if err := r.Body.Close(); err != nil {
			log.Error("close request body", "error", err)
		}
	}()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	// codex review: the second Decode's own error (a confusing "cannot unmarshal
	// ... into struct{}" for some trailing shapes) is discarded in favor of one
	// consistent message — callers only need to know "there was more than one
	// value", not decode internals of a throwaway probe value.
	if err := decoder.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON value")
	}
	return nil
}

// writeJSONDecodeError maps a decodeJSONBody error to the appropriate HTTP status:
// 415 for an unsupported Content-Type, 413 when http.MaxBytesReader's limit was
// hit, 400 for any other decode failure (malformed JSON, unknown field, trailing
// data).
func writeJSONDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	switch {
	case errors.Is(err, errUnsupportedMediaType):
		writeError(w, http.StatusUnsupportedMediaType, err.Error())
	case errors.As(err, &maxBytesErr):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

// isJSONContentType reports whether contentType's media type (ignoring
// parameters like charset) is exactly application/json.
func isJSONContentType(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json"
}
