package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"microapi/internal/models"
)

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func ValidateNames(set, collection string) error {
	if !nameRe.MatchString(set) { return httpError(http.StatusBadRequest, "invalid set name") }
	if collection != "" && !nameRe.MatchString(collection) { return httpError(http.StatusBadRequest, "invalid collection name") }
	return nil
}

func httpError(code int, msg string) error { return &HTTPError{Code: code, Message: msg} }

type HTTPError struct{ Code int; Message string }

func (e *HTTPError) Error() string { return e.Message }

func WriteJSON(w http.ResponseWriter, status int, success bool, data interface{}, errStr *string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(models.APIResponse{Success: success, Data: data, Error: errStr})
}

// Simple request logger producing structured logs
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := slog.Time("start", time.Now())
		slog.Info("request", slog.String("method", r.Method), slog.String("path", r.URL.Path), start)
		next.ServeHTTP(w, r)
	})
}

func LimitBody(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}
