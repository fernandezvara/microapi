package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"microapi/internal/database"
	"microapi/internal/middleware"
	"microapi/internal/models"
)

func (h *Handlers) ListSets(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT DISTINCT set_name FROM metadata ORDER BY set_name`)
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}
	defer rows.Close()
	var sets []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		sets = append(sets, s)
	}
	middleware.WriteJSON(w, http.StatusOK, true, sets, nil)
}

func (h *Handlers) GetSetStats(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	if err := middleware.ValidateNames(set, ""); err != nil {
		writeErr(w, err)
		return
	}
	if err := database.EnsureSetTable(h.db, set); err != nil {
		writeErr(w, err)
		return
	}

	// counts per collection
	q := "SELECT collection, COUNT(*), MIN(created_at) FROM " + tableName(set) + " GROUP BY collection"
	rows, err := h.db.Query(q)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	res := map[string]models.CollectionStat{}
	for rows.Next() {
		var coll string
		var cnt int
		var created int64
		if err := rows.Scan(&coll, &cnt, &created); err == nil {
			res[coll] = models.CollectionStat{Count: cnt, CreatedAt: created}
		}
	}
	middleware.WriteJSON(w, http.StatusOK, true, res, nil)
}

func (h *Handlers) DeleteSet(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.AllowDeleteSets {
		middleware.WriteJSON(w, http.StatusForbidden, false, nil, models.Ptr("set deletion disabled"))
		return
	}
	set := chi.URLParam(r, "set")
	if err := middleware.ValidateNames(set, ""); err != nil {
		writeErr(w, err)
		return
	}
	_, err := h.db.Exec("DROP TABLE IF EXISTS " + tableName(set))
	if err != nil {
		writeErr(w, err)
		return
	}
	_, _ = h.db.Exec(`DELETE FROM metadata WHERE set_name = ?`, set)
	middleware.WriteJSON(w, http.StatusOK, true, map[string]any{"deleted": set}, nil)
}

func tableName(set string) string { return "data_" + set }

func writeErr(w http.ResponseWriter, err error) {
	s := err.Error()
	code := http.StatusInternalServerError
	var he *middleware.HTTPError
	if ok := asHTTPError(err, &he); ok {
		code = he.Code
		s = he.Message
	}
	middleware.WriteJSON(w, code, false, nil, models.Ptr(s))
}

func asHTTPError(err error, target **middleware.HTTPError) bool {
	if e, ok := err.(*middleware.HTTPError); ok {
		*target = e
		return true
	}
	return false
}
