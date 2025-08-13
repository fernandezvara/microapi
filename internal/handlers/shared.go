package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"microapi/internal/config"
	"microapi/internal/middleware"
)

type Handlers struct {
	db  *sql.DB
	cfg *config.Config
}

func New(db *sql.DB, cfg *config.Config) *Handlers { return &Handlers{db: db, cfg: cfg} }

// sanitizeForCreate removes optional _meta and rejects any other top-level keys starting with "_".
func sanitizeForCreate(body map[string]any) (map[string]any, *middleware.HTTPError) {
	if body == nil { return map[string]any{}, nil }
	// Allow and drop _meta entirely on create
	delete(body, "_meta")
	for k := range body {
		if strings.HasPrefix(k, "_") {
			return nil, &middleware.HTTPError{Code: http.StatusBadRequest, Message: "fields starting with '_' are reserved"}
		}
	}
	return body, nil
}

// sanitizeForPutPatch allows an optional _meta; if _meta.id is present it must match the route id.
// _meta.created_at/_meta.updated_at are ignored. Any other top-level keys starting with '_' are rejected.
func sanitizeForPutPatch(body map[string]any, id string) (map[string]any, *middleware.HTTPError) {
	if body == nil { return map[string]any{}, nil }
	if v, ok := body["_meta"]; ok {
		if meta, okm := v.(map[string]any); okm {
			if rawID, okid := meta["id"]; okid {
				if sid, oks := rawID.(string); !oks || sid != id {
					return nil, &middleware.HTTPError{Code: http.StatusBadRequest, Message: "body _meta.id must match resource id"}
				}
			}
		} else {
			return nil, &middleware.HTTPError{Code: http.StatusBadRequest, Message: "_meta must be an object"}
		}
		// Drop _meta from stored document
		delete(body, "_meta")
	}
	for k := range body {
		if strings.HasPrefix(k, "_") {
			return nil, &middleware.HTTPError{Code: http.StatusBadRequest, Message: "fields starting with '_' are reserved"}
		}
	}
	return body, nil
}

func suppressMeta(r *http.Request) bool { return r.URL.Query().Get("meta") == "0" }

func writeDocResponse(w http.ResponseWriter, r *http.Request, status int, data map[string]any, id string, created, updated int64) {
	if !suppressMeta(r) {
		if data == nil { data = map[string]any{} }
		data["_meta"] = map[string]any{
			"id":         id,
			"created_at": created,
			"updated_at": updated,
		}
	}
	middleware.WriteJSON(w, status, true, data, nil)
}
