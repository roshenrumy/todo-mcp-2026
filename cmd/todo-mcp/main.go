package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/example/todo-mcp-2026/internal/database"
	"github.com/example/todo-mcp-2026/internal/importjob"
	"github.com/example/todo-mcp-2026/internal/requeststate"
	"github.com/example/todo-mcp-2026/internal/todo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

var createSchema = json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false,"required":["title","schedule"],"properties":{"title":{"type":"string"},"description":{"type":"string"},"listId":{"type":"string"},"parentId":{"type":"string","description":"Optional parent Todo ID for a subtask"},"priority":{"enum":["low","medium","high"]},"schedule":{"type":"object","additionalProperties":true,"properties":{"kind":{"enum":["one_time","recurring"]},"dueAt":{"type":"string"},"recurrence":{"type":"object"}}}}}`)

type taskParams struct {
	mcp.ParamsBase
	TaskID string `json:"taskId"`
}
type taskResult struct {
	mcp.ResultBase
	Task importjob.Task `json:"task"`
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type taskToolCall struct {
	Name      string `json:"name"`
	Arguments struct {
		Items []importjob.Item `json:"items"`
	} `json:"arguments"`
}

func text(v any) *mcp.CallToolResult {
	b, _ := json.Marshal(v)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
}
func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: todo-mcp server|worker|migrate|seed|reset")
	}
	db, err := database.Open(env("DATABASE_PATH", "/data/todo.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(db); err != nil {
		log.Fatal(err)
	}
	if err = database.EnsureSystemLists(db); err != nil {
		log.Fatal(err)
	}
	svc := todo.Service{DB: db}
	jobs := importjob.Service{DB: db, Todos: svc, Delay: time.Duration(envInt("IMPORT_DELAY_MS", 75)) * time.Millisecond}
	switch os.Args[1] {
	case "migrate":
		return
	case "seed":
		if err := svc.Seed(); err != nil {
			log.Fatal(err)
		}
		return
	case "reset":
		if err := reset(db); err != nil {
			log.Fatal(err)
		}
		return
	case "server":
		serve(svc, jobs)
	case "worker":
		for {
			_, err := jobs.RunOnce()
			if err != nil {
				log.Print(err)
			}
			time.Sleep(250 * time.Millisecond)
		}
	default:
		log.Fatal("unknown command")
	}
}

// reset removes only application records so a local demo can start clean.
func reset(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"import_jobs", "tasks", "todos"} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func serve(s todo.Service, jobs importjob.Service) {
	secret := env("REQUEST_STATE_SECRET", "development-only-change-me")
	codec := requeststate.New(secret)
	capabilities := &mcp.ServerCapabilities{}
	capabilities.AddExtension("io.modelcontextprotocol/tasks", nil)
	server := mcp.NewServer(&mcp.Implementation{Name: "todo-mcp-2026", Version: "0.1.0"}, &mcp.ServerOptions{Capabilities: capabilities})
	mcp.AddTool(server, &mcp.Tool{Name: "list_todos", Description: "List todos"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		x, e := s.List()
		return text(x), nil, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "create_todo", Description: "Create one-time or recurring todo", InputSchema: createSchema}, func(_ context.Context, _ *mcp.CallToolRequest, a map[string]any) (*mcp.CallToolResult, any, error) {
		t := todo.Todo{Title: argumentString(a, "title"), Description: argumentString(a, "description"), ListID: argumentString(a, "listId"), ParentID: argumentString(a, "parentId"), Priority: argumentString(a, "priority")}
		if x, ok := a["schedule"].(map[string]any); ok && x["kind"] == "one_time" {
			t.DueAt = fmt.Sprint(x["dueAt"])
		}
		z, e := s.Create(t)
		return text(z), nil, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "move_todo", Description: "Move or complete a todo. Always call this tool when a user asks to move or complete a todo; do not ask a manual follow-up first. If moving a parent to done with incomplete subtasks, this tool returns input_required with a signed requestState and options. Present that server response to the user, then retry this same tool with their choice and requestState."}, func(_ context.Context, _ *mcp.CallToolRequest, a struct {
		TodoID       string `json:"todoId"`
		Target       string `json:"targetListId"`
		RequestState string `json:"requestState,omitempty"`
		Choice       string `json:"choice,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		if a.RequestState != "" {
			st, e := codec.Decode(a.RequestState)
			if e != nil {
				return nil, nil, e
			}
			if st.TodoID != a.TodoID || st.TargetListID != a.Target {
				return nil, nil, fmt.Errorf("requestState arguments do not match")
			}
			_, v, e := s.IncompleteChildren(a.TodoID)
			if e != nil {
				return nil, nil, e
			}
			if v != st.TodoVersion {
				return nil, nil, fmt.Errorf("todo changed while awaiting input")
			}
			if a.Choice == "cancel" {
				return text(map[string]any{"cancelled": true}), nil, nil
			}
			e = s.Move(a.TodoID, a.Target, a.Choice == "move_with_subtasks")
			return text(map[string]any{"moved": e == nil}), nil, e
		}
		kids, v, e := s.IncompleteChildren(a.TodoID)
		if e != nil {
			return nil, nil, e
		}
		if a.Target == "done" && len(kids) > 0 {
			token, e := codec.Encode(requeststate.State{Operation: "move_todo", TodoID: a.TodoID, TargetListID: a.Target, TodoVersion: v, IncompleteSubtaskIDs: kids, ExpiresAt: time.Now().Add(10 * time.Minute)})
			return text(map[string]any{"resultType": "input_required", "message": fmt.Sprintf("This todo has %d incomplete subtasks. What should happen?", len(kids)), "options": []string{"move_parent_only", "move_with_subtasks", "cancel"}, "requestState": token}), nil, e
		}
		e = s.Move(a.TodoID, a.Target, false)
		return text(map[string]any{"moved": e == nil}), nil, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "bulk_update_todos", Description: "Apply independent updates in a bounded SQLite transaction"}, func(_ context.Context, _ *mcp.CallToolRequest, a struct {
		Updates []todo.Update `json:"updates"`
	}) (*mcp.CallToolResult, any, error) {
		r, e := s.BulkUpdate(a.Updates)
		return text(map[string]any{"successful": filterUpdates(r, true), "failed": filterUpdates(r, false)}), nil, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "import_todos", Description: "Queue a durable asynchronous Todo import"}, func(_ context.Context, _ *mcp.CallToolRequest, a struct {
		Items []importjob.Item `json:"items"`
	}) (*mcp.CallToolResult, any, error) {
		t, e := jobs.Enqueue(a.Items)
		return text(map[string]any{"resultType": "task", "taskId": t.ID, "status": t.Status, "pollIntervalMs": 500, "ttlMs": 600000}), nil, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "tasks_get", Description: "Get a durable import task"}, func(_ context.Context, _ *mcp.CallToolRequest, a struct {
		TaskID string `json:"taskId"`
	}) (*mcp.CallToolResult, any, error) {
		t, e := jobs.Get(a.TaskID)
		return text(t), nil, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "tasks_cancel", Description: "Request cancellation of an import task"}, func(_ context.Context, _ *mcp.CallToolRequest, a struct {
		TaskID string `json:"taskId"`
	}) (*mcp.CallToolResult, any, error) {
		e := jobs.Cancel(a.TaskID)
		return text(map[string]any{"taskId": a.TaskID, "cancelRequested": e == nil}), nil, e
	})
	server.AddPrompt(&mcp.Prompt{Name: "plan_my_day", Description: "Host-side daily planning without sampling", Arguments: []*mcp.PromptArgument{{Name: "date", Required: true}, {Name: "availableMinutes", Required: true}}}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		date := req.Params.Arguments["date"]
		minutes := req.Params.Arguments["availableMinutes"]
		prompt := fmt.Sprintf("Plan my day for %s using exactly %s available minutes. Call list_todos; inspect overdue and due-today work; prioritize high priority work; fit it into the provided time; show the proposed plan before changing data; then make exactly one bulk_update_todos call after approval.", date, minutes)
		return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: prompt}}}}, nil
	})
	for _, method := range []string{"tasks/get", "tasks/update"} {
		method := method
		if err := mcp.AddReceivingCustomMethod(server, method, func(_ context.Context, _ *mcp.ServerSession, p *taskParams) (*taskResult, error) {
			t, e := jobs.Get(p.TaskID)
			return &taskResult{Task: t}, e
		}); err != nil {
			log.Fatal(err)
		}
	}
	if err := mcp.AddReceivingCustomMethod(server, "tasks/cancel", func(_ context.Context, _ *mcp.ServerSession, p *taskParams) (*taskResult, error) {
		if e := jobs.Cancel(p.TaskID); e != nil {
			return nil, e
		}
		t, e := jobs.Get(p.TaskID)
		return &taskResult{Task: t}, e
	}); err != nil {
		log.Fatal(err)
	}
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	http.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Demo-Instance", env("HOSTNAME", "local"))
		if serveTaskExtension(w, r, jobs) {
			return
		}
		h.ServeHTTP(w, r)
	}))
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// serveTaskExtension emits the extension's top-level task result. The SDK's
// normal tool helper only emits CallToolResult, which Inspector cannot track.
func serveTaskExtension(w http.ResponseWriter, r *http.Request, jobs importjob.Service) bool {
	if r.Method != http.MethodPost {
		return false
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var rpc rpcEnvelope
	if json.Unmarshal(body, &rpc) != nil || !taskCapable(rpc.Params) {
		return false
	}
	if rpc.Method == "tools/call" {
		var call taskToolCall
		if json.Unmarshal(rpc.Params, &call) != nil || call.Name != "import_todos" {
			return false
		}
		task, err := jobs.Enqueue(call.Arguments.Items)
		if err != nil {
			writeTaskJSON(w, rpc.ID, map[string]any{"resultType": "complete", "isError": true, "content": []map[string]string{{"type": "text", "text": err.Error()}}})
			return true
		}
		writeTaskJSON(w, rpc.ID, map[string]any{"resultType": "task", "taskId": task.ID, "status": task.Status, "createdAt": task.CreatedAt, "lastUpdatedAt": task.LastUpdatedAt, "ttlMs": 600000, "pollIntervalMs": 500})
		return true
	}
	if rpc.Method == "tasks/get" {
		var params taskParams
		if json.Unmarshal(rpc.Params, &params) != nil {
			return false
		}
		task, err := jobs.Get(params.TaskID)
		if err != nil {
			return false
		}
		writeTaskJSON(w, rpc.ID, taskPayload(task))
		return true
	}
	if rpc.Method == "tasks/list" {
		tasks, err := jobs.ListActive()
		if err != nil {
			return false
		}
		writeTaskJSON(w, rpc.ID, map[string]any{"tasks": tasks})
		return true
	}
	return false
}

func taskPayload(task importjob.Task) map[string]any {
	payload := map[string]any{
		"resultType":     "complete",
		"taskId":         task.ID,
		"status":         task.Status,
		"statusMessage":  task.Message,
		"createdAt":      task.CreatedAt,
		"lastUpdatedAt":  task.LastUpdatedAt,
		"ttlMs":          600000,
		"pollIntervalMs": 500,
	}
	if task.Status == "completed" {
		payload["result"] = map[string]any{
			"content": []map[string]string{{
				"type": "text",
				"text": "Todo import completed.",
			}},
			"structuredContent": json.RawMessage(task.Result),
		}
	}
	return payload
}

func taskCapable(params json.RawMessage) bool {
	var value map[string]any
	if json.Unmarshal(params, &value) != nil {
		return false
	}
	meta, _ := value["_meta"].(map[string]any)
	capabilities, _ := meta["io.modelcontextprotocol/clientCapabilities"].(map[string]any)
	extensions, _ := capabilities["extensions"].(map[string]any)
	_, ok := extensions["io.modelcontextprotocol/tasks"]
	return ok
}

func writeTaskJSON(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
}

func argumentString(arguments map[string]any, key string) string {
	value, ok := arguments[key]
	if !ok || value == nil {
		return ""
	}
	stringValue, _ := value.(string)
	return stringValue
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	var n int
	if _, e := fmt.Sscanf(env(k, fmt.Sprint(d)), "%d", &n); e != nil {
		return d
	}
	return n
}
func filterUpdates(items []todo.UpdateResult, ok bool) []todo.UpdateResult {
	out := []todo.UpdateResult{}
	for _, i := range items {
		if i.Successful == ok {
			out = append(out, i)
		}
	}
	return out
}
