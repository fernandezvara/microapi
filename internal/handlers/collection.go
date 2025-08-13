package handlers

import (
	"fmt"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"microapi/internal/database"
	"microapi/internal/middleware"
	"microapi/internal/models"
	"microapi/internal/query"
)

func (h *Handlers) QueryCollection(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	collection := chi.URLParam(r, "collection")
	if err := middleware.ValidateNames(set, collection); err != nil { writeErr(w, err); return }
	if err := database.EnsureSetTable(h.db, set); err != nil { writeErr(w, err); return }

	whereStr := r.URL.Query().Get("where")
	pw, err := query.ParseWhere(whereStr)
	if err != nil { middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr(err.Error())); return }

	orderBy := r.URL.Query().Get("order_by")
	limit := parseInt(r.URL.Query().Get("limit"), 0)
	offset := parseInt(r.URL.Query().Get("offset"), -1)

	// total count for pagination (ignores limit/offset)
	countSQL, countArgs := query.BuildCount(query.BuildOpts{Set: set, Collection: collection, Where: pw})
	var total int64
	if err := h.db.QueryRow(countSQL, countArgs...).Scan(&total); err == nil {
		w.Header().Set("X-Total-Items", fmt.Sprintf("%d", total))
	}

	sqlStr, args := query.BuildSelect(query.BuildOpts{Set: set, Collection: collection, Where: pw, OrderBy: orderBy, Limit: limit, Offset: offset})
	rows, err := h.db.Query(sqlStr, args...)
	if err != nil { writeErr(w, err); return }
	defer rows.Close()
	var results []map[string]any
	for rows.Next() {
		var id string; var dataStr string; var created, updated int64
		if err := rows.Scan(&id, &dataStr, &created, &updated); err == nil {
			var m map[string]any
			_ = json.Unmarshal([]byte(dataStr), &m)
			if !suppressMeta(r) {
				m["_meta"] = map[string]any{
					"id":         id,
					"created_at": created,
					"updated_at": updated,
				}
			}
			results = append(results, m)
		}
	}
	middleware.WriteJSON(w, http.StatusOK, true, results, nil)
}

func parseInt(s string, def int) int {
	if s == "" { return def }
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil { return def }
	return v
}
