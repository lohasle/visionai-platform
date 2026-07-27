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
				"last_worker": "gpu-1",
				"execution":   map[string]any{"progress": 100},
			}}})
		case "/workers.get_all":
			_, _ = w.Write([]byte(`{"data":{"workers":[{"id":"gpu-1","last_report_time":"2026-07-27T06:23:28.130000+00:00","queues":[{"name":"gpu"}]}]}}`))
		case "/tasks.dequeue":
			_, _ = w.Write([]byte(`{"data":{"updated":1}}`))
		case "/events.get_task_log":
			_, _ = w.Write([]byte(`{"data":{"events":[{"timestamp":1,"level":"info","worker":"gpu-1","msg":"epoch complete"}]}}`))
		case "/events.add":
			_, _ = w.Write([]byte(`{"data":{"added":1,"errors":0}}`))
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
	if err != nil || task.Status != "completed" || task.Progress != 100 || task.WorkerID != "gpu-1" {
		t.Fatalf("get: %#v %v", task, err)
	}
	workers, err := client.Workers(context.Background())
	if err != nil || len(workers) != 1 || workers[0].ID != "gpu-1" || len(workers[0].Queues) != 1 {
		t.Fatalf("workers: %#v %v", workers, err)
	}
	if err = client.Cancel(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	log, err := client.TaskLog(context.Background(), task.ID)
	if err != nil || !strings.Contains(string(log), "epoch complete") {
		t.Fatalf("log: %q %v", log, err)
	}
	if err = client.ReportMetrics(context.Background(), task.ID, []ResultMetric{{Name: "train/loss", Step: 1, Value: 0.5}}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 14 {
		t.Fatalf("unexpected call count: %v", calls)
	}
}

func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"--env", `VISIONAI_PARAMETERS_JSON={"epochs": 1}`, "plain"})
	want := `--env 'VISIONAI_PARAMETERS_JSON={"epochs": 1}' plain`
	if got != want {
		t.Fatalf("shellJoin = %q, want %q", got, want)
	}
}

func TestClearMLHyperParameters(t *testing.T) {
	got := clearMLHyperParameters("VisionAI", map[string]any{"epochs": 2, "pretrained": true})
	epochs, ok := got["epochs"].(map[string]any)
	if !ok || epochs["section"] != "VisionAI" || epochs["value"] != "2" || epochs["type"] != "int" {
		t.Fatalf("unexpected epochs parameter: %#v", got["epochs"])
	}
	pretrained, ok := got["pretrained"].(map[string]any)
	if !ok || pretrained["value"] != "true" || pretrained["type"] != "bool" {
		t.Fatalf("unexpected pretrained parameter: %#v", got["pretrained"])
	}
}
