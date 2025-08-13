package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/xid"

	"microapi/internal/config"
	"microapi/internal/database"
	"microapi/internal/middleware"
	"microapi/internal/query"
)

type ListSetsArgs struct{}

type CreateDocumentArgs struct {
	Set        string                 `json:"set" jsonschema:"the set name"`
	Collection string                 `json:"collection" jsonschema:"the collection name"`
	Document   map[string]interface{} `json:"document" jsonschema:"the document object to create"`
}

type GetDocumentArgs struct {
	Set        string `json:"set"`
	Collection string `json:"collection"`
	ID         string `json:"id"`
}

type UpdateDocumentArgs struct {
	Set        string                 `json:"set"`
	Collection string                 `json:"collection"`
	ID         string                 `json:"id"`
	Patch      map[string]interface{} `json:"patch"`
}

type DeleteDocumentArgs struct {
	Set        string `json:"set"`
	Collection string `json:"collection"`
	ID         string `json:"id"`
}

type QueryCollectionArgs struct {
	Set         string `json:"set"`
	Collection  string `json:"collection"`
	Where       string `json:"where" jsonschema:"JSON string representing filters"`
	OrderBy     string `json:"order_by"`
	Limit       int    `json:"limit"`
	Offset      int    `json:"offset"`
	IncludeMeta *bool  `json:"include_meta" jsonschema:"include _meta in results (default true)"`
}

func main() {
	// Structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	db, err := database.Open(cfg)
	if err != nil {
		logger.Error("failed to open database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		logger.Error("failed to migrate database", slog.String("error", err.Error()))
		os.Exit(1)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "microapi-mcp", Title: "Micro API MCP", Version: "v1.0.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{Name: "list_sets", Description: "List all available sets"}, listSetsTool(db))
	mcp.AddTool(server, &mcp.Tool{Name: "create_document", Description: "Create a new document in a collection"}, createDocumentTool(db))
	mcp.AddTool(server, &mcp.Tool{Name: "get_document", Description: "Get a document by id"}, getDocumentTool(db))
	mcp.AddTool(server, &mcp.Tool{Name: "update_document", Description: "Patch fields of a document by id"}, updateDocumentTool(db))
	mcp.AddTool(server, &mcp.Tool{Name: "delete_document", Description: "Delete a document by id"}, deleteDocumentTool(db))
	mcp.AddTool(server, &mcp.Tool{Name: "query_collection", Description: "Query a collection with optional where/order/limit/offset"}, queryCollectionTool(db))

	if err := server.Run(context.Background(), mcp.NewStdioTransport()); err != nil {
		log.Fatal(err)
	}
}

func listSetsTool(db *sql.DB) func(context.Context, *mcp.ServerSession, *mcp.CallToolParamsFor[ListSetsArgs]) (*mcp.CallToolResultFor[any], error) {
	return func(ctx context.Context, _ *mcp.ServerSession, _ *mcp.CallToolParamsFor[ListSetsArgs]) (*mcp.CallToolResultFor[any], error) {
		rows, err := db.Query(`SELECT DISTINCT set_name FROM metadata ORDER BY set_name`)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		defer rows.Close()
		var sets []string
		for rows.Next() {
			var s string
			_ = rows.Scan(&s)
			sets = append(sets, s)
		}
		return &mcp.CallToolResultFor[any]{StructuredContent: sets}, nil
	}
}

func createDocumentTool(db *sql.DB) func(context.Context, *mcp.ServerSession, *mcp.CallToolParamsFor[CreateDocumentArgs]) (*mcp.CallToolResultFor[any], error) {
	return func(ctx context.Context, _ *mcp.ServerSession, params *mcp.CallToolParamsFor[CreateDocumentArgs]) (*mcp.CallToolResultFor[any], error) {
		args := params.Arguments
		if args.Set == "" || args.Collection == "" {
			return errorResult("set and collection are required"), nil
		}
		if err := middleware.ValidateNames(args.Set, args.Collection); err != nil {
			return errorResult(err.Error()), nil
		}
		if err := database.EnsureSetTable(db, args.Set); err != nil {
			return errorResult(err.Error()), nil
		}
		if err := database.EnsureCollectionMetadata(db, args.Set, args.Collection); err != nil {
			return errorResult(err.Error()), nil
		}
		// sanitize: drop _meta and reject other _* fields
		if args.Document == nil {
			args.Document = map[string]any{}
		}
		delete(args.Document, "_meta")
		for k := range args.Document {
			if len(k) > 0 && k[0] == '_' {
				return errorResult("fields starting with '_' are reserved"), nil
			}
		}
		id := xid.New().String()
		now := time.Now().Unix()
		b, _ := json.Marshal(args.Document)
		_, err := db.Exec("INSERT INTO "+tableName(args.Set)+" (id, collection, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", id, args.Collection, string(b), now, now)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		res := cloneMap(args.Document)
		res["_meta"] = map[string]any{"id": id, "created_at": now, "updated_at": now}
		return &mcp.CallToolResultFor[any]{StructuredContent: res}, nil
	}
}

func getDocumentTool(db *sql.DB) func(context.Context, *mcp.ServerSession, *mcp.CallToolParamsFor[GetDocumentArgs]) (*mcp.CallToolResultFor[any], error) {
	return func(ctx context.Context, _ *mcp.ServerSession, params *mcp.CallToolParamsFor[GetDocumentArgs]) (*mcp.CallToolResultFor[any], error) {
		args := params.Arguments
		if args.Set == "" || args.Collection == "" || args.ID == "" {
			return errorResult("set, collection and id are required"), nil
		}
		if err := middleware.ValidateNames(args.Set, args.Collection); err != nil {
			return errorResult(err.Error()), nil
		}
		if err := database.EnsureSetTable(db, args.Set); err != nil {
			return errorResult(err.Error()), nil
		}
		var dataStr string
		var created, updated int64
		err := db.QueryRow("SELECT data, created_at, updated_at FROM "+tableName(args.Set)+" WHERE id = ? AND collection = ?", args.ID, args.Collection).Scan(&dataStr, &created, &updated)
		if err == sql.ErrNoRows {
			return errorResult("not found"), nil
		}
		if err != nil {
			return errorResult(err.Error()), nil
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(dataStr), &m)
		if m == nil {
			m = map[string]any{}
		}
		m["_meta"] = map[string]any{"id": args.ID, "created_at": created, "updated_at": updated}
		return &mcp.CallToolResultFor[any]{StructuredContent: m}, nil
	}
}

func updateDocumentTool(db *sql.DB) func(context.Context, *mcp.ServerSession, *mcp.CallToolParamsFor[UpdateDocumentArgs]) (*mcp.CallToolResultFor[any], error) {
	return func(ctx context.Context, _ *mcp.ServerSession, params *mcp.CallToolParamsFor[UpdateDocumentArgs]) (*mcp.CallToolResultFor[any], error) {
		args := params.Arguments
		if args.Set == "" || args.Collection == "" || args.ID == "" {
			return errorResult("set, collection and id are required"), nil
		}
		if err := middleware.ValidateNames(args.Set, args.Collection); err != nil {
			return errorResult(err.Error()), nil
		}
		// validate reserved fields and optional _meta.id
		if v, ok := args.Patch["_meta"]; ok {
			meta, okm := v.(map[string]any)
			if !okm {
				return errorResult("_meta must be an object"), nil
			}
			if rid, okid := meta["id"]; okid {
				sid, oks := rid.(string)
				if !oks || sid != args.ID {
					return errorResult("body _meta.id must match resource id"), nil
				}
			}
			delete(args.Patch, "_meta")
		}
		for k := range args.Patch {
			if len(k) > 0 && k[0] == '_' {
				return errorResult("fields starting with '_' are reserved"), nil
			}
		}
		// load existing
		var dataStr string
		err := db.QueryRow("SELECT data FROM "+tableName(args.Set)+" WHERE id = ? AND collection = ?", args.ID, args.Collection).Scan(&dataStr)
		if err == sql.ErrNoRows {
			return errorResult("not found"), nil
		}
		if err != nil {
			return errorResult(err.Error()), nil
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(dataStr), &m)
		if m == nil {
			m = map[string]any{}
		}
		for k, v := range args.Patch {
			m[k] = v
		}
		now := time.Now().Unix()
		_, err = db.Exec("UPDATE "+tableName(args.Set)+" SET data = ?, updated_at = ? WHERE id = ? AND collection = ?", mustJSON(m), now, args.ID, args.Collection)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		var created, updated int64
		err = db.QueryRow("SELECT created_at, updated_at FROM "+tableName(args.Set)+" WHERE id = ? AND collection = ?", args.ID, args.Collection).Scan(&created, &updated)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		m["_meta"] = map[string]any{"id": args.ID, "created_at": created, "updated_at": updated}
		return &mcp.CallToolResultFor[any]{StructuredContent: m}, nil
	}
}

func deleteDocumentTool(db *sql.DB) func(context.Context, *mcp.ServerSession, *mcp.CallToolParamsFor[DeleteDocumentArgs]) (*mcp.CallToolResultFor[any], error) {
	return func(ctx context.Context, _ *mcp.ServerSession, params *mcp.CallToolParamsFor[DeleteDocumentArgs]) (*mcp.CallToolResultFor[any], error) {
		args := params.Arguments
		if args.Set == "" || args.Collection == "" || args.ID == "" {
			return errorResult("set, collection and id are required"), nil
		}
		if err := middleware.ValidateNames(args.Set, args.Collection); err != nil {
			return errorResult(err.Error()), nil
		}
		_, _ = db.Exec("DELETE FROM "+tableName(args.Set)+" WHERE id = ? AND collection = ?", args.ID, args.Collection)
		return &mcp.CallToolResultFor[any]{StructuredContent: map[string]any{"deleted": args.ID}}, nil
	}
}

func queryCollectionTool(db *sql.DB) func(context.Context, *mcp.ServerSession, *mcp.CallToolParamsFor[QueryCollectionArgs]) (*mcp.CallToolResultFor[any], error) {
	return func(ctx context.Context, _ *mcp.ServerSession, params *mcp.CallToolParamsFor[QueryCollectionArgs]) (*mcp.CallToolResultFor[any], error) {
		args := params.Arguments
		if args.Set == "" || args.Collection == "" {
			return errorResult("set and collection are required"), nil
		}
		if err := middleware.ValidateNames(args.Set, args.Collection); err != nil {
			return errorResult(err.Error()), nil
		}
		if err := database.EnsureSetTable(db, args.Set); err != nil {
			return errorResult(err.Error()), nil
		}
		pw, err := query.ParseWhere(args.Where)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		// total count (ignore limit/offset)
		countSQL, countArgs := query.BuildCount(query.BuildOpts{Set: args.Set, Collection: args.Collection, Where: pw})
		var total int64
		_ = db.QueryRow(countSQL, countArgs...).Scan(&total)
		sqlStr, sqlArgs := query.BuildSelect(query.BuildOpts{Set: args.Set, Collection: args.Collection, Where: pw, OrderBy: args.OrderBy, Limit: args.Limit, Offset: args.Offset})
		rows, err := db.Query(sqlStr, sqlArgs...)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		defer rows.Close()
		var results []map[string]any
		for rows.Next() {
			var id, dataStr string
			var created, updated int64
			if err := rows.Scan(&id, &dataStr, &created, &updated); err == nil {
				var m map[string]any
				_ = json.Unmarshal([]byte(dataStr), &m)
				includeMeta := true
				if args.IncludeMeta != nil && !*args.IncludeMeta {
					includeMeta = false
				}
				if includeMeta {
					if m == nil {
						m = map[string]any{}
					}
					m["_meta"] = map[string]any{"id": id, "created_at": created, "updated_at": updated}
				}
				results = append(results, m)
			}
		}
		// Return both results and total so clients can page
		return &mcp.CallToolResultFor[any]{StructuredContent: map[string]any{"items": results, "total": total}}, nil
	}
}

func errorResult(msg string) *mcp.CallToolResultFor[any] {
	return &mcp.CallToolResultFor[any]{StructuredContent: map[string]any{"error": msg}, IsError: true}
}

func tableName(set string) string { return "data_" + set }

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}
