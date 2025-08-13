package query

import (
	"fmt"
	"strings"
)

type BuildOpts struct {
	Set        string
	Collection string
	Where      *ParsedWhere
	OrderBy    string
	Limit      int
	Offset     int
}

func BuildSelect(opts BuildOpts) (string, []any) {
	table := fmt.Sprintf("data_%s", opts.Set)
	base := fmt.Sprintf("SELECT id, data, created_at, updated_at FROM %s WHERE collection = ?", table)
	args := []any{opts.Collection}
	if opts.Where != nil {
		for _, c := range opts.Where.Conds {
			base += " AND " + c.SQL
			args = append(args, c.Args...)
		}
	}
	if opts.OrderBy != "" {
		if opts.OrderBy == "created_at" || opts.OrderBy == "updated_at" {
			base += " ORDER BY " + opts.OrderBy
		} else {
			// treat as JSON path
			base += " ORDER BY json_extract(data, '" + strings.ReplaceAll(opts.OrderBy, "'", "''") + "')"
		}
	}
	if opts.Limit > 0 {
		base += fmt.Sprintf(" LIMIT %d", opts.Limit)
		if opts.Offset > -1 {
			base += fmt.Sprintf(" OFFSET %d", opts.Offset)
		}
	}
	return base, args
}

// BuildCount builds a COUNT(*) query that matches the same WHERE conditions as BuildSelect.
func BuildCount(opts BuildOpts) (string, []any) {
	table := fmt.Sprintf("data_%s", opts.Set)
	base := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE collection = ?", table)
	args := []any{opts.Collection}
	if opts.Where != nil {
		for _, c := range opts.Where.Conds {
			base += " AND " + c.SQL
			args = append(args, c.Args...)
		}
	}
	return base, args
}
