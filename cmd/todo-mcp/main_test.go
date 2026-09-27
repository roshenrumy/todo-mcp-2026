package main

import (
	"encoding/json"
	"testing"
)

func TestCreateTodoSchemaIsValidJSON(t *testing.T) {
	if !json.Valid(createSchema) {
		t.Fatal("create_todo schema must be valid JSON")
	}
	if _, err := json.Marshal(createSchema); err != nil {
		t.Fatalf("create_todo schema must marshal: %v", err)
	}
}
