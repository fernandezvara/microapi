package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"microapi/internal/database"
	"microapi/internal/middleware"
	"microapi/internal/models"
)

func (h *Handlers) ListSets(w http.ResponseWriter, r *http.Request) {
    // Get number of collections per set from metadata
    rows, err := h.db.Query(`SELECT set_name, COUNT(*) AS colls FROM metadata GROUP BY set_name ORDER BY set_name`)
    if err != nil {
        middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
        return
    }
    defer rows.Close()

    // Build response structure
    setsMap := map[string]map[string]int64{}
    var totalDocs int64
    for rows.Next() {
        var set string
        var colls int64
        if err := rows.Scan(&set, &colls); err != nil { continue }

        // Count documents in the physical set table; if table is missing, treat as 0
        var docs int64
        if err := h.db.QueryRow("SELECT COUNT(*) FROM "+tableName(set)).Scan(&docs); err != nil {
            docs = 0
        }
        totalDocs += docs
        setsMap[set] = map[string]int64{"colls": colls, "docs": docs}
    }

    out := map[string]any{
        "sets": setsMap,
        "stats": map[string]any{
            "total_docs": totalDocs,
        },
    }
    middleware.WriteJSON(w, http.StatusOK, true, out, nil)
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
