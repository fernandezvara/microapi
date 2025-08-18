package query

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Condition struct {
	SQL  string
	Args []any
}

type ParsedWhere struct {
	Conds []Condition
	// Paths contains the normalized JSON paths (e.g. $.user.email) referenced in the where clause
	Paths []string
}

// ParseWhere expects a JSON object like {"field.path": {"$op": value}}
func ParseWhere(whereRaw string) (*ParsedWhere, error) {
	if strings.TrimSpace(whereRaw) == "" {
		return &ParsedWhere{Conds: []Condition{}, Paths: []string{}}, nil
	}
	var obj map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(whereRaw), &obj); err != nil {
		return nil, fmt.Errorf("malformed where clause: expected a JSON object where keys are field paths and values are operator objects")
	}
	pw := &ParsedWhere{Conds: []Condition{}, Paths: []string{}}
	for path, ops := range obj {
		jsonPath := toJSONPath(path)
		expr := fmt.Sprintf("json_extract(data, '%s')", jsonPath)
		for op, v := range ops {
			if !ValidOperator(op) {
				return nil, fmt.Errorf("unsupported operator: %s", op)
			}
			s, _ := ToSQL(op, expr)
			pw.Conds = append(pw.Conds, Condition{SQL: s, Args: []any{v}})
		}
		pw.Paths = append(pw.Paths, jsonPath)
	}
	return pw, nil
}

func toJSONPath(dot string) string {
	parts := strings.Split(dot, ".")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(p, "'", "''")
	}
	return "$." + strings.Join(parts, ".")
}
