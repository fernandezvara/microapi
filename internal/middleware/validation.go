package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"microapi/internal/models"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func ValidateNames(set, collection string) error {
	if !nameRe.MatchString(set) {
		return httpError(http.StatusBadRequest, "invalid set name")
	}
	if collection != "" && !nameRe.MatchString(collection) {
		return httpError(http.StatusBadRequest, "invalid collection name")
	}
	return nil
}

func httpError(code int, msg string) error { return &HTTPError{Code: code, Message: msg} }

type HTTPError struct {
	Code    int
	Message string
}

func (e *HTTPError) Error() string { return e.Message }

func WriteJSON(w http.ResponseWriter, status int, success bool, data interface{}, errStr *string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(models.APIResponse{Success: success, Data: data, Error: errStr})
}

// Logger logs method, path, query/path params and captures response status & duration
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()

		// capture status and size
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)

		// gather path params from chi
		var pathParams map[string]string
		if rc := chi.RouteContext(r.Context()); rc != nil {
			pathParams = make(map[string]string, len(rc.URLParams.Keys))
			for i, k := range rc.URLParams.Keys {
				if i < len(rc.URLParams.Values) {
					pathParams[k] = rc.URLParams.Values[i]
				}
			}
		}

		slog.Info("request",
			slog.String("req_id", chimw.GetReqID(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("raw_query", r.URL.RawQuery),
			slog.Any("query", r.URL.Query()),
			slog.Any("path_params", pathParams),
			slog.Int("status", sr.status),
			slog.Int("bytes", sr.size),
			slog.Duration("duration", time.Since(started)),
		)
	})
}

// statusRecorder wraps ResponseWriter to record status code and size
type statusRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	// in case Write is called without WriteHeader
	if sr.status == 0 {
		sr.status = http.StatusOK
	}
	n, err := sr.ResponseWriter.Write(b)
	sr.size += n
	return n, err
}

func LimitBody(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}
