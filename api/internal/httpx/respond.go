package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/rinaldo-mist/property-mapper/api/internal/logging"
)

// errorEnvelope is the single error shape every failing endpoint returns.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"requestId,omitempty"`
}

// JSON writes a successful response.
func JSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// The status line is already sent, so this can only be logged.
		logging.FromContext(r.Context()).Error("encode response", "error", err)
	}
}

// NoContent writes 204.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Fail writes an error response and logs the underlying cause.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := AsAPIError(err)
	reqID := middleware.GetReqID(r.Context())
	log := logging.FromContext(r.Context())

	if apiErr.Status >= http.StatusInternalServerError {
		log.Error("request failed",
			"code", apiErr.Code, "status", apiErr.Status, "error", apiErr.Error())
	} else {
		log.Debug("request rejected",
			"code", apiErr.Code, "status", apiErr.Status, "message", apiErr.Message)
	}

	JSON(w, r, apiErr.Status, errorEnvelope{Error: errorBody{
		Code:      apiErr.Code,
		Message:   apiErr.Message,
		Fields:    apiErr.Fields,
		RequestID: reqID,
	}})
}

// DecodeJSON reads a JSON body with a size cap and strict field checking.
//
// DisallowUnknownFields turns a typo in a client payload into a clear 400 rather
// than a silently ignored field, which is the kind of bug that survives review.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	const maxBody = 1 << 20 // 1 MiB; JSON endpoints never carry file data

	if ct := r.Header.Get("Content-Type"); ct != "" && !isJSONContentType(ct) {
		return UnsupportedMedia("Content-Type harus application/json.")
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return TooLarge("Permintaan terlalu besar.")
		}
		return BadRequest("Format JSON tidak valid.")
	}
	// Exactly one JSON value per request; trailing content means a malformed body.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BadRequest("Body hanya boleh berisi satu objek JSON.")
	}
	return nil
}

func isJSONContentType(ct string) bool {
	for i := range len(ct) {
		if ct[i] == ';' {
			ct = ct[:i]
			break
		}
	}
	return trimSpace(ct) == "application/json"
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
