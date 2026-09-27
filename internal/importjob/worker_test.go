package importjob

import (
	"github.com/example/todo-mcp-2026/internal/database"
	"github.com/example/todo-mcp-2026/internal/todo"
	"path/filepath"
	"testing"
)

func TestCancellationStopsQueuedImport(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "todo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lists VALUES('inbox','Inbox')`); err != nil {
		t.Fatal(err)
	}
	s := Service{DB: db, Todos: todo.Service{DB: db}}
	task, err := s.Enqueue([]Item{{Title: "one", Priority: "medium"}, {Title: "two", Priority: "medium"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel(task.ID); err != nil {
		t.Fatal(err)
	}
	worked, err := s.RunOnce()
	if err != nil || !worked {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	got, err := s.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "cancelled" {
		t.Fatalf("want cancelled, got %s", got.Status)
	}
}
