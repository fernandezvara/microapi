package utils

import (
	"encoding/json"
)

func MustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func CloneMap(m map[string]interface{}) map[string]interface{} {
	b, _ := json.Marshal(m)
	var out map[string]interface{}
	_ = json.Unmarshal(b, &out)
	return out
}
