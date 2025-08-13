package validation

import (
	"database/sql"
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// GetSchemaJSON returns the raw JSON schema bytes for a collection, or nil if none.
func GetSchemaJSON(db *sql.DB, set, collection string) ([]byte, error) {
	var raw sql.NullString
	err := db.QueryRow(`SELECT schema FROM schemas WHERE set_name = ? AND collection_name = ?`, set, collection).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !raw.Valid || raw.String == "" || raw.String == "null" {
		return nil, nil
	}
	return []byte(raw.String), nil
}

// SetSchemaJSON upserts the schema JSON for a collection.
func SetSchemaJSON(db *sql.DB, set, collection string, schemaBytes []byte) error {
	// validate schema is syntactically correct JSON
	var tmp any
	if err := json.Unmarshal(schemaBytes, &tmp); err != nil {
		return fmt.Errorf("invalid JSON schema: %w", err)
	}
	_, err := db.Exec(`INSERT INTO schemas (set_name, collection_name, schema, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(set_name, collection_name) DO UPDATE SET schema = excluded.schema, updated_at = excluded.updated_at`,
		set, collection, string(schemaBytes), time.Now().Unix())
	return err
}

// DeleteSchema removes the schema for a collection.
func DeleteSchema(db *sql.DB, set, collection string) error {
	_, err := db.Exec(`DELETE FROM schemas WHERE set_name = ? AND collection_name = ?`, set, collection)
	return err
}

// ValidateDocument validates the document against the stored JSON schema, if any.
func ValidateDocument(db *sql.DB, set, collection string, doc map[string]any) error {
	schemaBytes, err := GetSchemaJSON(db, set, collection)
	if err != nil {
		return err
	}
	if schemaBytes == nil {
		return nil // no schema defined
	}
	c := jsonschema.NewCompiler()
	// add schema as an in-memory resource
	if err := c.AddResource("mem://schema.json", bytes.NewReader(schemaBytes)); err != nil {
		return fmt.Errorf("invalid JSON schema: %w", err)
	}
	s, err := c.Compile("mem://schema.json")
	if err != nil {
		return fmt.Errorf("invalid JSON schema: %w", err)
	}
	if err := s.Validate(doc); err != nil {
		return fmt.Errorf("schema validation failed: %v", err)
	}
	return nil
}

func joinStrings(ss []string, sep string) string {
	res := ""
	for i, s := range ss {
		if i > 0 {
			res += sep
		}
		res += s
	}
	return res
}
