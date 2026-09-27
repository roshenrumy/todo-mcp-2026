package todo

import (
	"database/sql"
	"fmt"
	"github.com/google/uuid"
	"time"
)

type Todo struct {
	ID, Title, Description, ListID, Priority, DueAt, ParentID string
	Completed                                                 bool
	Version                                                   int
}
type Service struct{ DB *sql.DB }

type Change struct {
	Priority, ListID, DueAt *string `json:"priority,omitempty"`
}
type Update struct {
	TodoID  string `json:"todoId"`
	Changes Change `json:"changes"`
}
type UpdateResult struct {
	TodoID     string `json:"todoId"`
	Error      string `json:"error,omitempty"`
	Successful bool   `json:"successful"`
}

func (s Service) List() ([]Todo, error) {
	rows, e := s.DB.Query(`SELECT id,title,COALESCE(description,''),list_id,priority,COALESCE(due_at,''),COALESCE(parent_todo_id,''),completed,version FROM todos ORDER BY created_at`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Todo
	for rows.Next() {
		var t Todo
		if e = rows.Scan(&t.ID, &t.Title, &t.Description, &t.ListID, &t.Priority, &t.DueAt, &t.ParentID, &t.Completed, &t.Version); e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s Service) Create(t Todo) (Todo, error) {
	if t.Title == "" {
		return t, fmt.Errorf("title is required")
	}
	if t.Priority == "" {
		t.Priority = "medium"
	}
	if t.ListID == "" {
		t.ListID = "inbox"
	}
	t.ID = uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	_, e := s.DB.Exec(`INSERT INTO todos(id,title,description,list_id,priority,due_at,parent_todo_id,completed,version,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,1,?,?)`, t.ID, t.Title, t.Description, t.ListID, t.Priority, null(t.DueAt), null(t.ParentID), t.Completed, now, now)
	return t, e
}
func null(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func (s Service) Move(id, target string, include bool) error {
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, e = tx.Exec(`UPDATE todos SET list_id=?, completed=?,version=version+1,updated_at=? WHERE id=?`, target, target == "done", now, id); e != nil {
		return e
	}
	if include {
		_, e = tx.Exec(`UPDATE todos SET list_id=?,completed=?,version=version+1,updated_at=? WHERE parent_todo_id=?`, target, target == "done", now, id)
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}

// BulkUpdate deliberately serializes SQLite writes in a brief transaction.
func (s Service) BulkUpdate(updates []Update) ([]UpdateResult, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	results := make([]UpdateResult, 0, len(updates))
	now := time.Now().UTC().Format(time.RFC3339)
	for _, u := range updates {
		if u.TodoID == "" {
			results = append(results, UpdateResult{Error: "todoId is required"})
			continue
		}
		var exists int
		if err := tx.QueryRow(`SELECT count(*) FROM todos WHERE id=?`, u.TodoID).Scan(&exists); err != nil {
			return nil, err
		}
		if exists == 0 {
			results = append(results, UpdateResult{TodoID: u.TodoID, Error: "todo not found"})
			continue
		}
		if u.Changes.Priority != nil && *u.Changes.Priority != "low" && *u.Changes.Priority != "medium" && *u.Changes.Priority != "high" {
			results = append(results, UpdateResult{TodoID: u.TodoID, Error: "invalid priority"})
			continue
		}
		_, err := tx.Exec(`UPDATE todos SET priority=COALESCE(?,priority),list_id=COALESCE(?,list_id),due_at=COALESCE(?,due_at),version=version+1,updated_at=? WHERE id=?`, u.Changes.Priority, u.Changes.ListID, u.Changes.DueAt, now, u.TodoID)
		if err != nil {
			results = append(results, UpdateResult{TodoID: u.TodoID, Error: err.Error()})
			continue
		}
		results = append(results, UpdateResult{TodoID: u.TodoID, Successful: true})
	}
	return results, tx.Commit()
}
func (s Service) IncompleteChildren(id string) ([]string, int, error) {
	rows, e := s.DB.Query(`SELECT id FROM todos WHERE parent_todo_id=? AND completed=0`, id)
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	var x []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		x = append(x, id)
	}
	var v int
	e = s.DB.QueryRow(`SELECT version FROM todos WHERE id=?`, id).Scan(&v)
	return x, v, e
}
func (s Service) Seed() error {
	for _, l := range []struct{ id, n string }{{"inbox", "Inbox"}, {"today", "Today"}, {"done", "Done"}} {
		if _, e := s.DB.Exec(`INSERT OR IGNORE INTO lists VALUES(?,?)`, l.id, l.n); e != nil {
			return e
		}
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM todos`).Scan(&n)
	if n > 0 {
		return nil
	}
	p, e := s.Create(Todo{Title: "Publish MCP article", ListID: "today", Priority: "high"})
	if e != nil {
		return e
	}
	for _, x := range []string{"Finish MRTR section", "Verify code examples"} {
		if _, e = s.Create(Todo{Title: x, ListID: "today", Priority: "high", ParentID: p.ID}); e != nil {
			return e
		}
	}
	_, e = s.Create(Todo{Title: "Overdue high priority task", ListID: "inbox", Priority: "high", DueAt: time.Now().AddDate(0, 0, -1).Format(time.RFC3339)})
	return e
}
