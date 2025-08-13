package handlers

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"microapi/internal/database"
	"microapi/internal/middleware"
	"microapi/internal/models"
	"microapi/internal/validation"
)

func (h *Handlers) PutSchema(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	if err := database.EnsureSetTable(h.db, set); err != nil { writeErr(w, err); return }
	if err := database.EnsureCollectionMetadata(h.db, set, collection); err != nil { writeErr(w, err); return }

	body, err := io.ReadAll(r.Body)
	if err != nil { writeErr(w, err); return }
	trim := string(body)
	if len(trim) == 0 || trim == "null" {
		if err := validation.DeleteSchema(h.db, set, collection); err != nil { writeErr(w, err); return }
		middleware.WriteJSON(w, http.StatusOK, true, map[string]any{"schema": nil}, nil)
		return
	}
	// store provided schema
	if err := validation.SetSchemaJSON(h.db, set, collection, body); err != nil { middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr(err.Error())); return }
	// echo back schema
	var schema any
	_ = json.Unmarshal(body, &schema)
	middleware.WriteJSON(w, http.StatusOK, true, map[string]any{"schema": schema}, nil)
}

func (h *Handlers) GetCollectionInfo(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	if err := database.EnsureSetTable(h.db, set); err != nil { writeErr(w, err); return }

	// schema
	schemaBytes, err := validation.GetSchemaJSON(h.db, set, collection)
	if err != nil { writeErr(w, err); return }
	var schema any
	if schemaBytes != nil { _ = json.Unmarshal(schemaBytes, &schema) }

	// indexes
	indexes, err := database.ListIndexes(h.db, set, collection)
	if err != nil { writeErr(w, err); return }

	// stats
	var count int64
	var created sql.NullInt64
	row := h.db.QueryRow("SELECT COUNT(*), MIN(created_at) FROM "+tableName(set)+" WHERE collection = ?", collection)
	_ = row.Scan(&count, &created)
	stats := map[string]any{"count": count}
	if created.Valid { stats["created_at"] = created.Int64 }

	middleware.WriteJSON(w, http.StatusOK, true, map[string]any{
		"schema":  schema,
		"indexes": indexes,
		"stats":   stats,
	}, nil)
}
