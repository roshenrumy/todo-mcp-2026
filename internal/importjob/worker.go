package importjob

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/todo-mcp-2026/internal/todo"
	"github.com/google/uuid"
)

type Item struct {
	Title       string `json:"title"`
	Completed   bool   `json:"completed"`
	Priority    string `json:"priority"`
	Description string `json:"description,omitempty"`
}
type Task struct {
	ID              string          `json:"taskId"`
	Status          string          `json:"status"`
	Message         string          `json:"statusMessage"`
	Result          json.RawMessage `json:"result,omitempty"`
	CreatedAt       string          `json:"createdAt"`
	LastUpdatedAt   string          `json:"lastUpdatedAt"`
	ExpiresAt       string          `json:"expiresAt"`
	CancelRequested bool            `json:"-"`
}
type Service struct {
	DB    *sql.DB
	Todos todo.Service
	Delay time.Duration
}

func (s Service) Enqueue(items []Item) (Task, error) {
	id := uuid.NewString()
	data, e := json.Marshal(items)
	if e != nil {
		return Task{}, e
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, e := s.DB.Begin()
	if e != nil {
		return Task{}, e
	}
	defer tx.Rollback()
	_, e = tx.Exec(`INSERT INTO tasks(task_id,type,status,status_message,created_at,updated_at,expires_at) VALUES(?,?,?,?,?,?,?)`, id, "import_todos", "working", "Queued", now, now, time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339))
	if e == nil {
		_, e = tx.Exec(`INSERT INTO import_jobs(id,task_id,items_json,status) VALUES(?,?,?,'queued')`, uuid.NewString(), id, string(data))
	}
	if e != nil {
		return Task{}, e
	}
	return Task{ID: id, Status: "working", Message: "Queued", CreatedAt: now, LastUpdatedAt: now, ExpiresAt: time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)}, tx.Commit()
}
func (s Service) Get(id string) (Task, error) {
	var t Task
	var r string
	e := s.DB.QueryRow(`SELECT task_id,status,COALESCE(status_message,''),COALESCE(result_json,''),cancel_requested,created_at,updated_at,expires_at FROM tasks WHERE task_id=?`, id).Scan(&t.ID, &t.Status, &t.Message, &r, &t.CancelRequested, &t.CreatedAt, &t.LastUpdatedAt, &t.ExpiresAt)
	t.Result = json.RawMessage(r)
	return t, e
}

func (s Service) ListActive() ([]Task, error) {
	rows, err := s.DB.Query(`SELECT task_id,status,COALESCE(status_message,''),COALESCE(result_json,''),cancel_requested,created_at,updated_at,expires_at FROM tasks WHERE status IN ('working','input_required') ORDER BY updated_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var task Task
		var result string
		if err := rows.Scan(&task.ID, &task.Status, &task.Message, &result, &task.CancelRequested, &task.CreatedAt, &task.LastUpdatedAt, &task.ExpiresAt); err != nil {
			return nil, err
		}
		task.Result = json.RawMessage(result)
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}
func (s Service) Cancel(id string) error {
	_, e := s.DB.Exec(`UPDATE tasks SET cancel_requested=1,status_message='Cancellation requested',updated_at=? WHERE task_id=? AND status='working'`, time.Now().UTC().Format(time.RFC3339), id)
	return e
}

func (s Service) RunOnce() (bool, error) {
	tx, e := s.DB.Begin()
	if e != nil {
		return false, e
	}
	var job, tid, data string
	e = tx.QueryRow(`SELECT id,task_id,items_json FROM import_jobs WHERE status='queued' LIMIT 1`).Scan(&job, &tid, &data)
	if e == sql.ErrNoRows {
		tx.Rollback()
		return false, nil
	}
	if e != nil {
		tx.Rollback()
		return false, e
	}
	r, e := tx.Exec(`UPDATE import_jobs SET status='working',claimed_at=? WHERE id=? AND status='queued'`, time.Now().UTC().Format(time.RFC3339), job)
	if e != nil {
		tx.Rollback()
		return false, e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		tx.Rollback()
		return false, nil
	}
	if e = tx.Commit(); e != nil {
		return false, e
	}
	var items []Item
	if e = json.Unmarshal([]byte(data), &items); e != nil {
		return false, e
	}
	imported := 0
	for _, x := range items {
		var cancelled bool
		if e = s.DB.QueryRow(`SELECT cancel_requested FROM tasks WHERE task_id=?`, tid).Scan(&cancelled); e != nil {
			return false, e
		}
		if cancelled {
			_, e = s.DB.Exec(`UPDATE tasks SET status='cancelled',status_message='Cancelled',updated_at=? WHERE task_id=?`, time.Now().UTC().Format(time.RFC3339), tid)
			return true, e
		}
		if _, e = s.Todos.Create(todo.Todo{Title: x.Title, Description: x.Description, Priority: x.Priority, Completed: x.Completed}); e == nil {
			imported++
		}
		_, e = s.DB.Exec(`UPDATE tasks SET status_message=?,updated_at=? WHERE task_id=?`, fmt.Sprintf("Imported %d / %d todos", imported, len(items)), time.Now().UTC().Format(time.RFC3339), tid)
		if e != nil {
			return false, e
		}
		if s.Delay > 0 {
			time.Sleep(s.Delay)
		}
	}
	result, _ := json.Marshal(map[string]any{"imported": imported, "skipped": 0, "failed": len(items) - imported})
	_, e = s.DB.Exec(`UPDATE tasks SET status='completed',status_message='Completed',result_json=?,updated_at=? WHERE task_id=?`, string(result), time.Now().UTC().Format(time.RFC3339), tid)
	if e == nil {
		_, e = s.DB.Exec(`UPDATE import_jobs SET status='completed' WHERE id=?`, job)
	}
	return true, e
}
