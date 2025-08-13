package query

import "fmt"

// Supported operators and their SQL counterparts
var opMap = map[string]string{
	"$eq":  "=",
	"$ne":  "!=",
	"$gt":  ">",
	"$gte": ">=",
	"$lt":  "<",
	"$lte": "<=",
}

func ValidOperator(op string) bool { _, ok := opMap[op]; return ok }

func ToSQL(op string, expr string) (string, error) {
	if sqlop, ok := opMap[op]; ok {
		return fmt.Sprintf("%s %s ?", expr, sqlop), nil
	}
	return "", fmt.Errorf("unsupported operator: %s", op)
}
