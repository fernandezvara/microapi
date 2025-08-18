package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/xid"

	"microapi/internal/database"
	"microapi/internal/middleware"
	"microapi/internal/models"
	"microapi/internal/query"
)

// MCPDiscovery returns tool definitions for MCP clients.
func (h *Handlers) MCPDiscovery(w http.ResponseWriter, r *http.Request) {
	tools := []map[string]any{
		{
			"name":        "list_sets",
			"description": "List all available sets",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		},
		{
			"name":        "create_document",
			"description": "Create a new document in a collection",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"set":        map[string]any{"type": "string"},
					"collection": map[string]any{"type": "string"},
					"document":   map[string]any{"type": "object"},
				},
				"required": []string{"set", "collection", "document"},
			},
		},
		{
			"name":        "get_document",
			"description": "Get a document by id",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"set":        map[string]any{"type": "string"},
					"collection": map[string]any{"type": "string"},
					"id":         map[string]any{"type": "string"},
				},
				"required": []string{"set", "collection", "id"},
			},
		},
		{
			"name":        "update_document",
			"description": "Patch fields of a document by id",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"set":        map[string]any{"type": "string"},
					"collection": map[string]any{"type": "string"},
					"id":         map[string]any{"type": "string"},
					"patch":      map[string]any{"type": "object"},
				},
				"required": []string{"set", "collection", "id", "patch"},
			},
		},
		{
			"name":        "delete_document",
			"description": "Delete a document by id",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"set":        map[string]any{"type": "string"},
					"collection": map[string]any{"type": "string"},
					"id":         map[string]any{"type": "string"},
				},
				"required": []string{"set", "collection", "id"},
			},
		},
		{
			"name":        "query_collection",
			"description": "Query a collection with optional where/order/limit/offset",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"set":          map[string]any{"type": "string"},
					"collection":   map[string]any{"type": "string"},
					"where":        map[string]any{"type": "string", "description": "JSON string of where filters"},
					"order_by":     map[string]any{"type": "string"},
					"limit":        map[string]any{"type": "integer"},
					"offset":       map[string]any{"type": "integer"},
					"include_meta": map[string]any{"type": "boolean", "default": true},
				},
				"required": []string{"set", "collection"},
			},
		},
	}
	middleware.WriteJSON(w, http.StatusOK, true, map[string]any{"tools": tools}, nil)
}

// MCPOperation is the request format for POST /mcp
// { "tool": "create_document", "args": { ... } }

type mcpRequest struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

func (h *Handlers) MCPCall(w http.ResponseWriter, r *http.Request) {
	var req mcpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("invalid JSON body"))
		return
	}
	switch req.Tool {
	case "list_sets":
		listSetsMCP(h.db, w)
	case "create_document":
		createDocMCP(h, w, req.Args)
	case "get_document":
		getDocMCP(h, w, req.Args)
	case "update_document":
		updateDocMCP(h, w, req.Args)
	case "delete_document":
		deleteDocMCP(h, w, req.Args)
	case "query_collection":
		queryCollectionMCP(h, w, req.Args)
	default:
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("unknown tool"))
	}
}

func listSetsMCP(db *sql.DB, w http.ResponseWriter) {
	rows, err := db.Query(`SELECT DISTINCT set_name FROM metadata ORDER BY set_name`)
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

func createDocMCP(h *Handlers, w http.ResponseWriter, args map[string]any) {
	set, _ := args["set"].(string)
	collection, _ := args["collection"].(string)
	rawDoc, _ := args["document"].(map[string]any)
	if set == "" || collection == "" {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("set and collection are required"))
		return
	}
	if err := middleware.ValidateNames(set, collection); err != nil {
		writeErr(w, err)
		return
	}
	if err := database.EnsureSetTable(h.db, set); err != nil {
		writeErr(w, err)
		return
	}
	if err := database.EnsureCollectionMetadata(h.db, set, collection); err != nil {
		writeErr(w, err)
		return
	}
	body, verr := sanitizeForCreate(rawDoc)
	if verr != nil {
		middleware.WriteJSON(w, verr.Code, false, nil, models.Ptr(verr.Message))
		return
	}
	id := xid.New().String()
	now := time.Now().Unix()
	b, _ := json.Marshal(body)
	_, err := h.db.Exec("INSERT INTO "+tableName(set)+" (id, collection, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", id, collection, string(b), now, now)
	if err != nil {
		writeErr(w, err)
		return
	}
	// include meta by default
	if body == nil {
		body = map[string]any{}
	}
	body["_meta"] = map[string]any{"id": id, "created_at": now, "updated_at": now}
	middleware.WriteJSON(w, http.StatusCreated, true, body, nil)
}

func getDocMCP(h *Handlers, w http.ResponseWriter, args map[string]any) {
	set, _ := args["set"].(string)
	collection, _ := args["collection"].(string)
	id, _ := args["id"].(string)
	if set == "" || collection == "" || id == "" {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("set, collection and id are required"))
		return
	}
	if err := middleware.ValidateNames(set, collection); err != nil {
		writeErr(w, err)
		return
	}
	if err := database.EnsureSetTable(h.db, set); err != nil {
		writeErr(w, err)
		return
	}
	var dataStr string
	var created, updated int64
	err := h.db.QueryRow("SELECT data, created_at, updated_at FROM "+tableName(set)+" WHERE id = ? AND collection = ?", id, collection).Scan(&dataStr, &created, &updated)
	if err == sql.ErrNoRows {
		middleware.WriteJSON(w, http.StatusNotFound, false, nil, models.Ptr("not found"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(dataStr), &m)
	if m == nil {
		m = map[string]any{}
	}
	m["_meta"] = map[string]any{"id": id, "created_at": created, "updated_at": updated}
	middleware.WriteJSON(w, http.StatusOK, true, m, nil)
}

func updateDocMCP(h *Handlers, w http.ResponseWriter, args map[string]any) {
	set, _ := args["set"].(string)
	collection, _ := args["collection"].(string)
	id, _ := args["id"].(string)
	patch, _ := args["patch"].(map[string]any)
	if set == "" || collection == "" || id == "" {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("set, collection and id are required"))
		return
	}
	if err := middleware.ValidateNames(set, collection); err != nil {
		writeErr(w, err)
		return
	}
	sanitized, verr := sanitizeForPutPatch(patch, id)
	if verr != nil {
		middleware.WriteJSON(w, verr.Code, false, nil, models.Ptr(verr.Message))
		return
	}
	// read existing
	var dataStr string
	err := h.db.QueryRow("SELECT data FROM "+tableName(set)+" WHERE id = ? AND collection = ?", id, collection).Scan(&dataStr)
	if err == sql.ErrNoRows {
		middleware.WriteJSON(w, http.StatusNotFound, false, nil, models.Ptr("not found"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(dataStr), &m)
	if m == nil {
		m = map[string]any{}
	}
	for k, v := range sanitized {
		m[k] = v
	}
	now := time.Now().Unix()
	_, err = h.db.Exec("UPDATE "+tableName(set)+" SET data = ?, updated_at = ? WHERE id = ? AND collection = ?", mustJSON(m), now, id, collection)
	if err != nil {
		writeErr(w, err)
		return
	}
	var created, updated int64
	err = h.db.QueryRow("SELECT created_at, updated_at FROM "+tableName(set)+" WHERE id = ? AND collection = ?", id, collection).Scan(&created, &updated)
	if err != nil {
		writeErr(w, err)
		return
	}
	m["_meta"] = map[string]any{"id": id, "created_at": created, "updated_at": updated}
	middleware.WriteJSON(w, http.StatusOK, true, m, nil)
}

func deleteDocMCP(h *Handlers, w http.ResponseWriter, args map[string]any) {
	set, _ := args["set"].(string)
	collection, _ := args["collection"].(string)
	id, _ := args["id"].(string)
	if set == "" || collection == "" || id == "" {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("set, collection and id are required"))
		return
	}
	if err := middleware.ValidateNames(set, collection); err != nil {
		writeErr(w, err)
		return
	}
	_, _ = h.db.Exec("DELETE FROM "+tableName(set)+" WHERE id = ? AND collection = ?", id, collection)
	middleware.WriteJSON(w, http.StatusOK, true, map[string]any{"deleted": id}, nil)
}

func queryCollectionMCP(h *Handlers, w http.ResponseWriter, args map[string]any) {
	set, _ := args["set"].(string)
	collection, _ := args["collection"].(string)
	whereStr, _ := args["where"].(string)
	orderBy, _ := args["order_by"].(string)
	// limit/offset may be float64 when decoded into interface{}
	var limit, offset int
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}
	if v, ok := args["offset"].(float64); ok {
		offset = int(v)
	}
	includeMeta := true
	if v, ok := args["include_meta"].(bool); ok {
		includeMeta = v
	}
	if set == "" || collection == "" {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("set and collection are required"))
		return
	}
	if err := middleware.ValidateNames(set, collection); err != nil {
		writeErr(w, err)
		return
	}
	if err := database.EnsureSetTable(h.db, set); err != nil {
		writeErr(w, err)
		return
	}
	pw, err := query.ParseWhere(whereStr)
	if err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr(err.Error()))
		return
	}

	// total count for pagination (ignores limit/offset) to maintain parity with REST
	countSQL, countArgs := query.BuildCount(query.BuildOpts{Set: set, Collection: collection, Where: pw})
	var total int64
	if err := h.db.QueryRow(countSQL, countArgs...).Scan(&total); err == nil {
		w.Header().Set("X-Total-Items", fmt.Sprintf("%d", total))
	}

	sqlStr, argsSQL := query.BuildSelect(query.BuildOpts{Set: set, Collection: collection, Where: pw, OrderBy: orderBy, Limit: limit, Offset: offset})
	rows, err := h.db.Query(sqlStr, argsSQL...)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	var results []map[string]any
	for rows.Next() {
		var id string
		var dataStr string
		var created, updated int64
		if err := rows.Scan(&id, &dataStr, &created, &updated); err == nil {
			var m map[string]any
			_ = json.Unmarshal([]byte(dataStr), &m)
			if includeMeta {
				if m == nil {
					m = map[string]any{}
				}
				m["_meta"] = map[string]any{"id": id, "created_at": created, "updated_at": updated}
			}
			results = append(results, m)
		}
	}
	if results == nil {
		results = []map[string]any{}
	}
	middleware.WriteJSON(w, http.StatusOK, true, results, nil)
}
