package database

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// NormalizePaths ensures each path starts with $. and has no surrounding spaces.
func NormalizePaths(paths []string) []string {
	res := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "$") {
			if strings.HasPrefix(p, ".") {
				p = "$" + p
			} else {
				p = "$." + p
			}
		}
		res = append(res, p)
	}
	// stable order for hashing
	sort.Strings(res)
	return res
}

// IndexName returns deterministic index name for a collection and set of paths.
func IndexName(collection string, paths []string) string {
	joined := strings.Join(paths, "|")
	sum := sha1.Sum([]byte(joined))
	return fmt.Sprintf("idx_%s_%s", collection, hex.EncodeToString(sum[:])[:10])
}

// CreateIndexMetadata inserts idx_metadata row with status creating.
func CreateIndexMetadata(db *sql.DB, set, collection, idxName string, paths []string) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO idx_metadata (set_name, collection_name, idx_name, paths, status, created_at) VALUES (?, ?, ?, ?, 'creating', ?)`,
		set, collection, idxName, strings.Join(paths, ","), time.Now().Unix())
	return err
}

// SetIndexStatus updates status and optional error.
func SetIndexStatus(db *sql.DB, set, collection, idxName, status, errText string) error {
	var errArg any
	if strings.TrimSpace(errText) == "" {
		errArg = nil
	} else {
		errArg = errText
	}
	_, err := db.Exec(`UPDATE idx_metadata SET status = ?, error = ? WHERE set_name = ? AND collection_name = ? AND idx_name = ?`,
		status, errArg, set, collection, idxName)
	return err
}

func EnsurePathExists(db *sql.DB, set, collection, path string) (bool, error) {
	q := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE collection = ? AND json_extract(data, ?) IS NOT NULL LIMIT 1)", tableName(set))
	var exists int
	err := db.QueryRow(q, collection, path).Scan(&exists)
	return exists == 1, err
}

func CreateSQLIndex(db *sql.DB, set, idxName string, paths []string) error {
	exprs := make([]string, 0, len(paths))
	for _, p := range paths {
		exprs = append(exprs, fmt.Sprintf("(json_extract(data, '%s'))", strings.ReplaceAll(p, "'", "''")))
	}
	q := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s(%s)", idxName, tableName(set), strings.Join(exprs, ", "))
	_, err := db.Exec(q)
	return err
}

func DropSQLIndex(db *sql.DB, idxName string) error {
	_, err := db.Exec("DROP INDEX IF EXISTS " + idxName)
	return err
}

func ListIndexes(db *sql.DB, set, collection string) ([]map[string]any, error) {
	rows, err := db.Query(`SELECT idx_name, paths, status, error, usage_count, last_used_at, created_at FROM idx_metadata WHERE set_name = ? AND collection_name = ? ORDER BY created_at DESC`, set, collection)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var name, paths, status, errtxt sql.NullString
		var usage, last, created sql.NullInt64
		_ = rows.Scan(&name, &paths, &status, &errtxt, &usage, &last, &created)
		out = append(out, map[string]any{
			"name":         name.String,
			"paths":        strings.Split(paths.String, ","),
			"status":       status.String,
			"error":        errtxt.String,
			"usage_count":  usage.Int64,
			"last_used_at": last.Int64,
			"created_at":   created.Int64,
		})
	}
	return out, nil
}

func UpdateIndexUsage(db *sql.DB, set, collection string, usedPaths []string) {
	if len(usedPaths) == 0 { return }
	// Fetch existing indexes
	rows, err := db.Query(`SELECT idx_name, paths FROM idx_metadata WHERE set_name = ? AND collection_name = ? AND status = 'ready'`, set, collection)
	if err != nil { return }
	defer rows.Close()
	used := make(map[string]struct{}, len(usedPaths))
	for _, p := range usedPaths { used[p] = struct{}{} }
	now := time.Now().Unix()
	for rows.Next() {
		var name, paths string
		_ = rows.Scan(&name, &paths)
		pp := strings.Split(paths, ",")
		matchAll := true
		for _, p := range pp {
			if _, ok := used[p]; !ok { matchAll = false; break }
		}
		if matchAll {
			_, _ = db.Exec(`UPDATE idx_metadata SET usage_count = usage_count + 1, last_used_at = ? WHERE set_name = ? AND collection_name = ? AND idx_name = ?`, now, set, collection, name)
		}
	}
}
