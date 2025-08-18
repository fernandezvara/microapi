package query

import (
	"fmt"
	"strings"
)

var supportedOps = map[string]struct{}{
	"$eq":          {},
	"$ne":          {},
	"$gt":          {},
	"$gte":         {},
	"$lt":          {},
	"$lte":         {},
	"$like":        {},
	"$ilike":       {},
	"$startsWith":  {},
	"$endsWith":    {},
	"$contains":    {},
	"$icontains":   {},
	"$istartsWith": {},
	"$iendsWith":   {},
	"$in":          {},
	"$nin":         {},
	"$between":     {},
	"$isNull":      {},
	"$notNull":     {},
}

func ValidOperator(op string) bool {
	_, ok := supportedOps[op]
	return ok
}

// ToSQL translates an operator, an expression (e.g. json_extract(...)), and a value into a SQL fragment and its args.
// The returned SQL should be safe to concatenate in WHERE with AND.
func ToSQL(op string, expr string, val any) (string, []any, error) {
	switch op {
	// Comparations
	case "$eq", "$ne", "$gt", "$gte", "$lt", "$lte":
		cmp := map[string]string{"$eq": "=", "$ne": "!=", "$gt": ">", "$gte": ">=", "$lt": "<", "$lte": "<="}[op]
		return fmt.Sprintf("%s %s ?", expr, cmp), []any{val}, nil

	// Text operators
	case "$like":
		return fmt.Sprintf("CAST(%s AS TEXT) LIKE ?", expr), []any{val}, nil
	case "$ilike":
		return fmt.Sprintf("LOWER(CAST(%s AS TEXT)) LIKE LOWER(?)", expr), []any{val}, nil
	case "$startsWith":
		return fmt.Sprintf("CAST(%s AS TEXT) LIKE (? || '%s')", expr, "%"), []any{val}, nil
	case "$endsWith":
		return fmt.Sprintf("CAST(%s AS TEXT) LIKE ('%s' || ?)", expr, "%"), []any{val}, nil
	case "$contains":
		return fmt.Sprintf("CAST(%s AS TEXT) LIKE ('%s' || ? || '%s')", expr, "%", "%"), []any{val}, nil
	case "$istartsWith":
		return fmt.Sprintf("LOWER(CAST(%s AS TEXT)) LIKE (LOWER(?) || '%s')", expr, "%"), []any{val}, nil
	case "$iendsWith":
		return fmt.Sprintf("LOWER(CAST(%s AS TEXT)) LIKE ('%s' || LOWER(?))", expr, "%"), []any{val}, nil
	case "$icontains":
		return fmt.Sprintf("LOWER(CAST(%s AS TEXT)) LIKE ('%s' || LOWER(?) || '%s')", expr, "%", "%"), []any{val}, nil

	// Set operators
	case "$in", "$nin":
		arr, ok := toInterfaceSlice(val)
		if !ok {
			return "", nil, fmt.Errorf("operator %s expects an array value", op)
		}
		if len(arr) == 0 {
			// By convention: IN [] => no rows; NOT IN [] => all rows
			if op == "$in" {
				return "1=0", nil, nil
			}
			return "1=1", nil, nil
		}
		ph := placeholders(len(arr))
		kw := "IN"
		if op == "$nin" {
			kw = "NOT IN"
		}
		return fmt.Sprintf("%s %s (%s)", expr, kw, ph), arr, nil

	case "$between":
		arr, ok := toInterfaceSlice(val)
		if !ok || len(arr) != 2 {
			return "", nil, fmt.Errorf("operator $between expects a two-element array [min, max]")
		}
		return fmt.Sprintf("%s BETWEEN ? AND ?", expr), []any{arr[0], arr[1]}, nil

	case "$isNull":
		return fmt.Sprintf("%s IS NULL", expr), nil, nil
	case "$notNull":
		return fmt.Sprintf("%s IS NOT NULL", expr), nil, nil
	}
	return "", nil, fmt.Errorf("unsupported operator: %s", op)
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?, ", n), ", ")
}

func toInterfaceSlice(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	default:
		return nil, false
	}
}
