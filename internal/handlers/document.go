package handlers

import (
	"database/sql"
	"encoding/json"
	"strings"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/xid"

	"microapi/internal/database"
	"microapi/internal/middleware"
	"microapi/internal/models"
	"microapi/internal/query"
	"microapi/internal/validation"
)

func (h *Handlers) CreateDocument(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	if err := database.EnsureSetTable(h.db, set); err != nil { writeErr(w, err); return }
	if err := database.EnsureCollectionMetadata(h.db, set, collection); err != nil { writeErr(w, err); return }

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("invalid JSON body")); return }
	sanitized, verr := sanitizeForCreate(body)
	if verr != nil { middleware.WriteJSON(w, verr.Code, false, nil, models.Ptr(verr.Message)); return }

	// Schema validation (if schema exists)
	if err := validation.ValidateDocument(h.db, set, collection, sanitized); err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr(err.Error()))
		return
	}

	id := xid.New().String()
	now := time.Now().Unix()
	dataBytes, _ := json.Marshal(sanitized)
	_, err := h.db.Exec("INSERT INTO "+tableName(set)+" (id, collection, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", id, collection, string(dataBytes), now, now)
	if err != nil { writeErr(w, err); return }

	writeDocResponse(w, r, http.StatusCreated, sanitized, id, now, now)
}

func (h *Handlers) GetDocument(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	id := chi.URLParam(r, "id")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	if err := database.EnsureSetTable(h.db, set); err != nil { writeErr(w, err); return }
	var dataStr string; var created, updated int64
	err := h.db.QueryRow("SELECT data, created_at, updated_at FROM "+tableName(set)+" WHERE id = ? AND collection = ?", id, collection).Scan(&dataStr, &created, &updated)
	if err == sql.ErrNoRows { middleware.WriteJSON(w, http.StatusNotFound, false, nil, models.Ptr("not found")); return }
	if err != nil { writeErr(w, err); return }
	var m map[string]any
	_ = json.Unmarshal([]byte(dataStr), &m)
	writeDocResponse(w, r, http.StatusOK, m, id, created, updated)
}

func (h *Handlers) ReplaceDocument(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	id := chi.URLParam(r, "id")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("invalid JSON body")); return }
	sanitized, verr := sanitizeForPutPatch(body, id)
	if verr != nil { middleware.WriteJSON(w, verr.Code, false, nil, models.Ptr(verr.Message)); return }
	// Schema validation
	if err := validation.ValidateDocument(h.db, set, collection, sanitized); err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr(err.Error()))
		return
	}
	now := time.Now().Unix()
	_, err := h.db.Exec("UPDATE "+tableName(set)+" SET data = ?, updated_at = ? WHERE id = ? AND collection = ?", mustJSON(sanitized), now, id, collection)
	if err != nil { writeErr(w, err); return }
	var created, updated int64
	err = h.db.QueryRow("SELECT created_at, updated_at FROM "+tableName(set)+" WHERE id = ? AND collection = ?", id, collection).Scan(&created, &updated)
	if err != nil { writeErr(w, err); return }
	writeDocResponse(w, r, http.StatusOK, sanitized, id, created, updated)
}

func (h *Handlers) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	id := chi.URLParam(r, "id")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil { middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("invalid JSON body")); return }
	sanitized, verr := sanitizeForPutPatch(patch, id)
	if verr != nil { middleware.WriteJSON(w, verr.Code, false, nil, models.Ptr(verr.Message)); return }
	// Load existing
	var dataStr string
	err := h.db.QueryRow("SELECT data FROM "+tableName(set)+" WHERE id = ? AND collection = ?", id, collection).Scan(&dataStr)
	if err == sql.ErrNoRows { middleware.WriteJSON(w, http.StatusNotFound, false, nil, models.Ptr("not found")); return }
	if err != nil { writeErr(w, err); return }
	var m map[string]any
	_ = json.Unmarshal([]byte(dataStr), &m)
	for k, v := range sanitized { m[k] = v }
	// Schema validation
	if err := validation.ValidateDocument(h.db, set, collection, m); err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr(err.Error()))
		return
	}
	now := time.Now().Unix()
	_, err = h.db.Exec("UPDATE "+tableName(set)+" SET data = ?, updated_at = ? WHERE id = ? AND collection = ?", mustJSON(m), now, id, collection)
	if err != nil { writeErr(w, err); return }
	var created, updated int64
	err = h.db.QueryRow("SELECT created_at, updated_at FROM "+tableName(set)+" WHERE id = ? AND collection = ?", id, collection).Scan(&created, &updated)
	if err != nil { writeErr(w, err); return }
	writeDocResponse(w, r, http.StatusOK, m, id, created, updated)
}

func (h *Handlers) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	id := chi.URLParam(r, "id")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	_, _ = h.db.Exec("DELETE FROM "+tableName(set)+" WHERE id = ? AND collection = ?", id, collection)
	middleware.WriteJSON(w, http.StatusOK, true, map[string]any{"deleted": id}, nil)
}

func (h *Handlers) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.AllowDeleteCollections { middleware.WriteJSON(w, http.StatusForbidden, false, nil, models.Ptr("collection deletion disabled")); return }
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	whereStr := r.URL.Query().Get("where")
	if strings.TrimSpace(whereStr) == "" {
		_, _ = h.db.Exec("DELETE FROM "+tableName(set)+" WHERE collection = ?", collection)
		middleware.WriteJSON(w, http.StatusOK, true, map[string]any{"deleted_collection": collection}, nil)
		return
	}
	pw, err := query.ParseWhere(whereStr)
	if err != nil { middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr(err.Error())); return }
	sqlStr := "DELETE FROM "+tableName(set)+" WHERE collection = ?"
	args := []any{collection}
	for _, c := range pw.Conds { sqlStr += " AND " + c.SQL; args = append(args, c.Args...) }
	_, _ = h.db.Exec(sqlStr, args...)
	middleware.WriteJSON(w, http.StatusOK, true, map[string]any{"deleted_conditional": true}, nil)
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
