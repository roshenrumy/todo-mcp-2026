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
