package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"microapi/internal/database"
	"microapi/internal/middleware"
	"microapi/internal/models"
)

type createIndexReq struct {
	Path  string   `json:"path"`
	Paths []string `json:"paths"`
}

func (h *Handlers) CreateIndex(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	if err := database.EnsureSetTable(h.db, set); err != nil { writeErr(w, err); return }
	if err := database.EnsureCollectionMetadata(h.db, set, collection); err != nil { writeErr(w, err); return }

	var body createIndexReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("invalid JSON body"))
		return
	}
	paths := body.Paths
	if len(paths) == 0 && strings.TrimSpace(body.Path) != "" {
		paths = []string{body.Path}
	}
	paths = database.NormalizePaths(paths)
	if len(paths) == 0 {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("path or paths required"))
		return
	}
	// verify at least one path exists in some document
	for _, p := range paths {
		exists, err := database.EnsurePathExists(h.db, set, collection, p)
		if err != nil { writeErr(w, err); return }
		if !exists {
			middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr(fmt.Sprintf("path not found in any document: %s", p)))
			return
		}
	}
	idxName := database.IndexName(collection, paths)
	if err := database.CreateIndexMetadata(h.db, set, collection, idxName, paths); err != nil { writeErr(w, err); return }
	// async create
	go func() {
		if err := database.CreateSQLIndex(h.db, set, idxName, paths); err != nil {
			_ = database.SetIndexStatus(h.db, set, collection, idxName, "error", err.Error())
			return
		}
		_ = database.SetIndexStatus(h.db, set, collection, idxName, "ready", "")
	}()
	middleware.WriteJSON(w, http.StatusAccepted, true, map[string]any{"name": idxName, "status": "creating"}, nil)
}

func (h *Handlers) ListIndexes(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	out, err := database.ListIndexes(h.db, set, collection)
	if err != nil { writeErr(w, err); return }
	middleware.WriteJSON(w, http.StatusOK, true, out, nil)
}

func (h *Handlers) GetIndexStatus(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	pathEnc := chi.URLParam(r, "path")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	p, _ := url.PathUnescape(pathEnc)
	paths := database.NormalizePaths([]string{p})
	idxName := database.IndexName(collection, paths)
	row := h.db.QueryRow(`SELECT status, error, usage_count, last_used_at, created_at FROM idx_metadata WHERE set_name = ? AND collection_name = ? AND idx_name = ?`, set, collection, idxName)
	var status, errtxt string
	var usage, last, created int64
	if err := row.Scan(&status, &errtxt, &usage, &last, &created); err != nil {
		middleware.WriteJSON(w, http.StatusNotFound, false, nil, models.Ptr("index not found"))
		return
	}
	middleware.WriteJSON(w, http.StatusOK, true, map[string]any{
		"name":         idxName,
		"status":       status,
		"error":        errtxt,
		"usage_count":  usage,
		"last_used_at": last,
		"created_at":   created,
	}, nil)
}

func (h *Handlers) DeleteIndex(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	qpaths := strings.TrimSpace(r.URL.Query().Get("paths"))
	var idxName string
	if qpaths != "" {
		parts := strings.Split(qpaths, ",")
		parts = database.NormalizePaths(parts)
		idxName = database.IndexName(collection, parts)
	} else {
		pathEnc := chi.URLParam(r, "path")
		p, _ := url.PathUnescape(pathEnc)
		pp := database.NormalizePaths([]string{p})
		idxName = database.IndexName(collection, pp)
	}
	if err := database.DropSQLIndex(h.db, idxName); err != nil { writeErr(w, err); return }
	_, _ = h.db.Exec(`DELETE FROM idx_metadata WHERE set_name = ? AND collection_name = ? AND idx_name = ?`, set, collection, idxName)
	middleware.WriteJSON(w, http.StatusOK, true, map[string]any{"deleted": idxName, "at": time.Now().Unix()}, nil)
}
