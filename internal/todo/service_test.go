package todo

import (
	"github.com/example/todo-mcp-2026/internal/database"
	"path/filepath"
	"testing"
)

func TestBulkUpdateReportsPartialFailures(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "todo.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(db); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO lists VALUES('inbox','Inbox')`); e != nil {
		t.Fatal(e)
	}
	s := Service{DB: db}
	x, e := s.Create(Todo{Title: "a"})
	if e != nil {
		t.Fatal(e)
	}
	bad := "wrong"
	r, e := s.BulkUpdate([]Update{{TodoID: x.ID}, {TodoID: "missing"}, {TodoID: x.ID, Changes: Change{Priority: &bad}}})
	if e != nil {
		t.Fatal(e)
	}
	if !r[0].Successful || r[1].Error == "" || r[2].Error == "" {
		t.Fatalf("unexpected results: %#v", r)
	}
}
