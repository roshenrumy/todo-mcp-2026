package main

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/example/todo-mcp-2026/internal/database"
	"github.com/example/todo-mcp-2026/internal/requeststate"
	"github.com/example/todo-mcp-2026/internal/todo"
)

func TestCreateTodoSchemaIsValidJSON(t *testing.T) {
	if !json.Valid(createSchema) {
		t.Fatal("create_todo schema must be valid JSON")
	}
	if _, err := json.Marshal(createSchema); err != nil {
		t.Fatalf("create_todo schema must marshal: %v", err)
	}
}

func TestMoveTodoUsesWireLevelMRTR(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "todo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureSystemLists(db); err != nil {
		t.Fatal(err)
	}
	service := todo.Service{DB: db}
	parent, err := service.Create(todo.Todo{Title: "Parent", ListID: "today"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(todo.Todo{Title: "Child", ListID: "today", ParentID: parent.ID}); err != nil {
		t.Fatal(err)
	}

	params, _ := json.Marshal(map[string]any{
		"name":      "move_todo",
		"arguments": map[string]any{"todoId": parent.ID, "targetListId": "done"},
		"_meta":     map[string]any{"io.modelcontextprotocol/clientCapabilities": map[string]any{"elicitation": map[string]any{}}},
	})
	call := moveTodoCall{Name: "move_todo"}
	call.Arguments.TodoID = parent.ID
	call.Arguments.TargetListID = "done"
	response := httptest.NewRecorder()
	serveMoveTodoMRTR(response, rpcEnvelope{ID: json.RawMessage(`1`), Params: params}, call, service, requeststate.New("test-secret"))

	var first struct {
		Result struct {
			ResultType    string         `json:"resultType"`
			InputRequests map[string]any `json:"inputRequests"`
			RequestState  string         `json:"requestState"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Result.ResultType != "input_required" || first.Result.RequestState == "" || first.Result.InputRequests["subtask_decision"] == nil {
		t.Fatalf("expected wire-level input_required response, got %s", response.Body.String())
	}

	call.RequestState = first.Result.RequestState
	call.InputResponses = map[string]struct {
		Action  string `json:"action"`
		Content struct {
			Choice string `json:"choice"`
		} `json:"content"`
	}{"subtask_decision": {Action: "accept", Content: struct {
		Choice string `json:"choice"`
	}{Choice: "move_with_subtasks"}}}
	retry := httptest.NewRecorder()
	serveMoveTodoMRTR(retry, rpcEnvelope{ID: json.RawMessage(`2`), Params: params}, call, service, requeststate.New("test-secret"))
	var completed struct {
		Result struct {
			ResultType string `json:"resultType"`
		} `json:"result"`
	}
	if err := json.Unmarshal(retry.Body.Bytes(), &completed); err != nil {
		t.Fatal(err)
	}
	if completed.Result.ResultType != "complete" {
		t.Fatalf("expected completed retry, got %s", retry.Body.String())
	}
}
