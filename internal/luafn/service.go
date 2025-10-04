package luafn

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"
	lua "github.com/yuin/gopher-lua"
)

// ExecutionContext holds context for a Lua function execution
type ExecutionContext struct {
	FunctionID  string
	ExecutionID string
	Timestamp   string
	Set         string
	DB          *sql.DB
	Tx          *sql.Tx
	Logs        []string
}

// ExecutionResult holds the result of a Lua function execution
type ExecutionResult struct {
	HTTPStatus int
	Output     map[string]any
	Logs       []string
	Duration   time.Duration
	Error      error
}

// Service manages Lua VM pool and function execution
type Service struct {
	vmPool sync.Pool
}

// NewService creates a new Lua service
func NewService() *Service {
	s := &Service{}
	s.vmPool.New = func() interface{} {
		return lua.NewState()
	}
	return s
}

// getVM retrieves a VM from the pool
func (s *Service) getVM() *lua.LState {
	return s.vmPool.Get().(*lua.LState)
}

// putVM returns a VM to the pool
func (s *Service) putVM(L *lua.LState) {
	// Reset the state
	L.SetTop(0)
	s.vmPool.Put(L)
}

// ExecuteFunction executes a Lua function with the given input
func (s *Service) ExecuteFunction(ctx context.Context, execCtx *ExecutionContext, code string, input map[string]any, timeout time.Duration) *ExecutionResult {
	start := time.Now()
	result := &ExecutionResult{
		HTTPStatus: 200,
		Output:     make(map[string]any),
		Logs:       []string{},
	}

	// Create a context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute in a goroutine to handle timeout
	done := make(chan bool)
	go func() {
		L := s.getVM()
		defer s.putVM(L)

		// Setup sandboxed environment
		s.setupSandbox(L, execCtx, &result.Logs)

		// Set global variables
		s.setGlobals(L, execCtx, input)

		// Execute the Lua code
		if err := L.DoString(code); err != nil {
			result.Error = fmt.Errorf("lua execution error: %w", err)
			result.HTTPStatus = 500
			done <- true
			return
		}

		// Extract results
		result.HTTPStatus = s.getHTTPStatus(L)
		result.Output = s.getOutput(L)
		result.Logs = append(result.Logs, execCtx.Logs...)

		done <- true
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		result.Duration = time.Since(start)
		return result
	case <-timeoutCtx.Done():
		result.Error = fmt.Errorf("function execution timeout after %v", timeout)
		result.HTTPStatus = 504
		result.Duration = time.Since(start)
		return result
	}
}

// setupSandbox creates a sandboxed Lua environment
func (s *Service) setupSandbox(L *lua.LState, execCtx *ExecutionContext, logs *[]string) {
	// Disable dangerous functions
	L.SetGlobal("require", lua.LNil)
	L.SetGlobal("dofile", lua.LNil)
	L.SetGlobal("loadfile", lua.LNil)
	L.SetGlobal("load", lua.LNil)
	L.SetGlobal("loadstring", lua.LNil)

	// Remove io, os, debug, package modules
	L.SetGlobal("io", lua.LNil)
	L.SetGlobal("os", lua.LNil)
	L.SetGlobal("debug", lua.LNil)
	L.SetGlobal("package", lua.LNil)

	// Add safe utility functions
	s.setupJSONModule(L)
	s.setupLogModule(L, logs)
	s.setupMicroAPIModule(L, execCtx)
}

// setupJSONModule adds json.encode and json.decode functions
func (s *Service) setupJSONModule(L *lua.LState) {
	jsonTable := L.NewTable()

	// json.encode
	jsonTable.RawSetString("encode", L.NewFunction(func(L *lua.LState) int {
		val := L.Get(1)
		goVal := luaToGo(val)
		jsonBytes, err := json.Marshal(goVal)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(jsonBytes)))
		return 1
	}))

	// json.decode
	jsonTable.RawSetString("decode", L.NewFunction(func(L *lua.LState) int {
		jsonStr := L.CheckString(1)
		var result interface{}
		err := json.Unmarshal([]byte(jsonStr), &result)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLua(L, result))
		return 1
	}))

	L.SetGlobal("json", jsonTable)
}

// setupLogModule adds log.info and log.error functions
func (s *Service) setupLogModule(L *lua.LState, logs *[]string) {
	logTable := L.NewTable()

	logTable.RawSetString("info", L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		*logs = append(*logs, fmt.Sprintf("[INFO] %s", msg))
		return 0
	}))

	logTable.RawSetString("error", L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		*logs = append(*logs, fmt.Sprintf("[ERROR] %s", msg))
		return 0
	}))

	L.SetGlobal("log", logTable)
}

// setupMicroAPIModule adds microapi.* functions
func (s *Service) setupMicroAPIModule(L *lua.LState, execCtx *ExecutionContext) {
	microapiTable := L.NewTable()

	// microapi.query(collection, filters)
	microapiTable.RawSetString("query", L.NewFunction(func(L *lua.LState) int {
		return s.luaQuery(L, execCtx)
	}))

	// microapi.get(collection, id)
	microapiTable.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		return s.luaGet(L, execCtx)
	}))

	// microapi.create(collection, data)
	microapiTable.RawSetString("create", L.NewFunction(func(L *lua.LState) int {
		return s.luaCreate(L, execCtx)
	}))

	// microapi.update(collection, id, data)
	microapiTable.RawSetString("update", L.NewFunction(func(L *lua.LState) int {
		return s.luaUpdate(L, execCtx)
	}))

	// microapi.patch(collection, id, changes)
	microapiTable.RawSetString("patch", L.NewFunction(func(L *lua.LState) int {
		return s.luaPatch(L, execCtx)
	}))

	// microapi.delete(collection, id)
	microapiTable.RawSetString("delete", L.NewFunction(func(L *lua.LState) int {
		return s.luaDelete(L, execCtx)
	}))

	L.SetGlobal("microapi", microapiTable)
}

// setGlobals sets the global variables for the Lua script
func (s *Service) setGlobals(L *lua.LState, execCtx *ExecutionContext, input map[string]any) {
	// Set input
	L.SetGlobal("input", goToLua(L, input))

	// Set set name
	L.SetGlobal("set", lua.LString(execCtx.Set))

	// Set ctx
	ctxTable := L.NewTable()
	ctxTable.RawSetString("function_id", lua.LString(execCtx.FunctionID))
	ctxTable.RawSetString("execution_id", lua.LString(execCtx.ExecutionID))
	ctxTable.RawSetString("timestamp", lua.LString(execCtx.Timestamp))
	L.SetGlobal("ctx", ctxTable)

	// Set default http_status and output
	L.SetGlobal("http_status", lua.LNumber(200))
	L.SetGlobal("output", L.NewTable())
}

// getHTTPStatus extracts the http_status variable from Lua
func (s *Service) getHTTPStatus(L *lua.LState) int {
	statusVal := L.GetGlobal("http_status")
	if num, ok := statusVal.(lua.LNumber); ok {
		return int(num)
	}
	return 200
}

// getOutput extracts the output variable from Lua
func (s *Service) getOutput(L *lua.LState) map[string]any {
	outputVal := L.GetGlobal("output")
	result := luaToGo(outputVal)
	if m, ok := result.(map[string]any); ok {
		return m
	}
	return map[string]any{"value": result}
}

// Helper function to get the database executor (transaction or DB)
func (s *Service) getExecutor(execCtx *ExecutionContext) interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
} {
	if execCtx.Tx != nil {
		return execCtx.Tx
	}
	return execCtx.DB
}

// tableName returns the table name for a set
func tableName(set string) string {
	return fmt.Sprintf("data_%s", set)
}

// luaQuery implements microapi.query(collection, filters)
func (s *Service) luaQuery(L *lua.LState, execCtx *ExecutionContext) int {
	collection := L.CheckString(1)
	filters := L.Get(2)

	executor := s.getExecutor(execCtx)
	table := tableName(execCtx.Set)

	// Build query
	sqlStr := fmt.Sprintf("SELECT id, data, created_at, updated_at FROM %s WHERE collection = ?", table)
	args := []interface{}{collection}

	// Add filters if provided
	if filters != lua.LNil {
		filterMap := luaToGo(filters)
		if fm, ok := filterMap.(map[string]any); ok {
			for key, value := range fm {
				sqlStr += fmt.Sprintf(" AND json_extract(data, '$.%s') = ?", strings.ReplaceAll(key, "'", "''"))
				args = append(args, value)
			}
		}
	}

	rows, err := executor.Query(sqlStr, args...)
	if err != nil {
		slog.Error("lua query error", "error", err)
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer rows.Close()

	results := L.NewTable()
	idx := 1
	for rows.Next() {
		var id, dataStr string
		var created, updated int64
		if err := rows.Scan(&id, &dataStr, &created, &updated); err != nil {
			continue
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
			continue
		}

		// Add metadata
		data["_meta"] = map[string]any{
			"id":         id,
			"created_at": created,
			"updated_at": updated,
		}

		results.RawSetInt(idx, goToLua(L, data))
		idx++
	}

	L.Push(results)
	return 1
}

// luaGet implements microapi.get(collection, id)
func (s *Service) luaGet(L *lua.LState, execCtx *ExecutionContext) int {
	collection := L.CheckString(1)
	id := L.CheckString(2)

	executor := s.getExecutor(execCtx)
	table := tableName(execCtx.Set)

	var dataStr string
	var created, updated int64
	err := executor.QueryRow(
		fmt.Sprintf("SELECT data, created_at, updated_at FROM %s WHERE id = ? AND collection = ?", table),
		id, collection,
	).Scan(&dataStr, &created, &updated)

	if err == sql.ErrNoRows {
		L.Push(lua.LNil)
		return 1
	}
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Add metadata
	data["_meta"] = map[string]any{
		"id":         id,
		"created_at": created,
		"updated_at": updated,
	}

	L.Push(goToLua(L, data))
	return 1
}

// luaCreate implements microapi.create(collection, data)
func (s *Service) luaCreate(L *lua.LState, execCtx *ExecutionContext) int {
	collection := L.CheckString(1)
	data := L.CheckTable(2)

	executor := s.getExecutor(execCtx)
	table := tableName(execCtx.Set)

	dataMap := luaToGo(data)
	if dm, ok := dataMap.(map[string]any); ok {
		// Remove any _meta field
		delete(dm, "_meta")

		// Generate ID and timestamps
		id := xid.New().String()
		now := time.Now().Unix()

		dataBytes, err := json.Marshal(dm)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		_, err = executor.Exec(
			fmt.Sprintf("INSERT INTO %s (id, collection, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", table),
			id, collection, string(dataBytes), now, now,
		)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// Return created document with metadata
		dm["_meta"] = map[string]any{
			"id":         id,
			"created_at": now,
			"updated_at": now,
		}

		L.Push(goToLua(L, dm))
		return 1
	}

	L.Push(lua.LNil)
	L.Push(lua.LString("data must be a table"))
	return 2
}

// luaUpdate implements microapi.update(collection, id, data)
func (s *Service) luaUpdate(L *lua.LState, execCtx *ExecutionContext) int {
	collection := L.CheckString(1)
	id := L.CheckString(2)
	data := L.CheckTable(3)

	executor := s.getExecutor(execCtx)
	table := tableName(execCtx.Set)

	dataMap := luaToGo(data)
	if dm, ok := dataMap.(map[string]any); ok {
		// Remove any _meta field
		delete(dm, "_meta")

		now := time.Now().Unix()
		dataBytes, err := json.Marshal(dm)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		_, err = executor.Exec(
			fmt.Sprintf("UPDATE %s SET data = ?, updated_at = ? WHERE id = ? AND collection = ?", table),
			string(dataBytes), now, id, collection,
		)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		// Get created_at
		var created int64
		err = executor.QueryRow(
			fmt.Sprintf("SELECT created_at FROM %s WHERE id = ? AND collection = ?", table),
			id, collection,
		).Scan(&created)
		if err != nil {
			created = now
		}

		// Return updated document with metadata
		dm["_meta"] = map[string]any{
			"id":         id,
			"created_at": created,
			"updated_at": now,
		}

		L.Push(goToLua(L, dm))
		return 1
	}

	L.Push(lua.LNil)
	L.Push(lua.LString("data must be a table"))
	return 2
}

// luaPatch implements microapi.patch(collection, id, changes)
func (s *Service) luaPatch(L *lua.LState, execCtx *ExecutionContext) int {
	collection := L.CheckString(1)
	id := L.CheckString(2)
	changes := L.CheckTable(3)

	executor := s.getExecutor(execCtx)
	table := tableName(execCtx.Set)

	// Get existing document
	var dataStr string
	var created int64
	err := executor.QueryRow(
		fmt.Sprintf("SELECT data, created_at FROM %s WHERE id = ? AND collection = ?", table),
		id, collection,
	).Scan(&dataStr, &created)

	if err == sql.ErrNoRows {
		L.Push(lua.LNil)
		L.Push(lua.LString("document not found"))
		return 2
	}
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	var existing map[string]any
	if err := json.Unmarshal([]byte(dataStr), &existing); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Apply changes
	changesMap := luaToGo(changes)
	if cm, ok := changesMap.(map[string]any); ok {
		delete(cm, "_meta")
		for k, v := range cm {
			existing[k] = v
		}
	}

	now := time.Now().Unix()
	dataBytes, err := json.Marshal(existing)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	_, err = executor.Exec(
		fmt.Sprintf("UPDATE %s SET data = ?, updated_at = ? WHERE id = ? AND collection = ?", table),
		string(dataBytes), now, id, collection,
	)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	// Return updated document with metadata
	existing["_meta"] = map[string]any{
		"id":         id,
		"created_at": created,
		"updated_at": now,
	}

	L.Push(goToLua(L, existing))
	return 1
}

// luaDelete implements microapi.delete(collection, id)
func (s *Service) luaDelete(L *lua.LState, execCtx *ExecutionContext) int {
	collection := L.CheckString(1)
	id := L.CheckString(2)

	executor := s.getExecutor(execCtx)
	table := tableName(execCtx.Set)

	result, err := executor.Exec(
		fmt.Sprintf("DELETE FROM %s WHERE id = ? AND collection = ?", table),
		id, collection,
	)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	affected, _ := result.RowsAffected()
	L.Push(lua.LBool(affected > 0))
	return 1
}

// luaToGo converts a Lua value to a Go value
func luaToGo(lv lua.LValue) interface{} {
	switch v := lv.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case lua.LString:
		return string(v)
	case *lua.LTable:
		maxn := v.MaxN()
		if maxn == 0 {
			// Treat as map/object
			ret := make(map[string]any)
			v.ForEach(func(key, value lua.LValue) {
				if keyStr, ok := key.(lua.LString); ok {
					ret[string(keyStr)] = luaToGo(value)
				}
			})
			return ret
		} else {
			// Treat as array
			ret := make([]any, 0, maxn)
			for i := 1; i <= maxn; i++ {
				ret = append(ret, luaToGo(v.RawGetInt(i)))
			}
			return ret
		}
	default:
		return nil
	}
}

// goToLua converts a Go value to a Lua value
func goToLua(L *lua.LState, v interface{}) lua.LValue {
	switch val := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(val)
	case int:
		return lua.LNumber(val)
	case int64:
		return lua.LNumber(val)
	case float64:
		return lua.LNumber(val)
	case string:
		return lua.LString(val)
	case map[string]interface{}:
		tbl := L.NewTable()
		for k, v := range val {
			tbl.RawSetString(k, goToLua(L, v))
		}
		return tbl
	case []any:
		tbl := L.NewTable()
		for i, v := range val {
			tbl.RawSetInt(i+1, goToLua(L, v))
		}
		return tbl
	default:
		return lua.LNil
	}
}
