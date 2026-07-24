package training

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClearMLContract(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth.login":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
				t.Error("missing basic authentication")
			}
			_, _ = w.Write([]byte(`{"data":{"token":"jwt"}}`))
		case "/tasks.create":
			if r.Header.Get("Authorization") != "Bearer jwt" {
				t.Error("missing bearer authentication")
			}
			_, _ = w.Write([]byte(`{"data":{"id":"task-1"}}`))
		case "/tasks.enqueue":
			_, _ = w.Write([]byte(`{"data":{"queued":1}}`))
		case "/tasks.get_by_id":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"task": map[string]any{
				"id": "task-1", "status": "completed", "status_message": "done",
				"execution": map[string]any{"progress": 100},
			}}})
		case "/tasks.dequeue":
			_, _ = w.Write([]byte(`{"data":{"updated":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClearML(server.URL, server.URL, "key", "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	task, err := client.CreateAndEnqueue(context.Background(), ClearMLSpec{
		Name: "contract", Queue: "gpu", ImageRef: "sha256:abc", Parameters: map[string]any{"epochs": 1},
	})
	if err != nil || task.ID != "task-1" {
		t.Fatalf("create: %#v %v", task, err)
	}
	task, err = client.GetTask(context.Background(), task.ID)
	if err != nil || task.Status != "completed" || task.Progress != 100 {
		t.Fatalf("get: %#v %v", task, err)
	}
	if err = client.Cancel(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 8 {
		t.Fatalf("unexpected call count: %v", calls)
	}
}
