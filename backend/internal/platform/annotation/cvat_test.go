package annotation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCVATProviderContract(t *testing.T) {
	var attached, replaced bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "visionai" || password != "secret" {
			t.Fatalf("missing provider credentials")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/server/about":
			_, _ = io.WriteString(w, `{"version":"2.71.0"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/tasks":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["name"] != "quality task" {
				t.Fatalf("unexpected task body: %#v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":42,"name":"quality task","status":"annotation","size":0}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/tasks/42/data":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			file, _, err := r.FormFile("client_files[0]")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			raw, _ := io.ReadAll(file)
			attached = string(raw) == "image-bytes"
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && r.URL.Path == "/api/tasks/42":
			_, _ = io.WriteString(w, `{"id":42,"name":"quality task","status":"annotation","size":1,"progress":80}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/tasks/42/annotations":
			_, _ = io.WriteString(w, `{"shapes":[{"id":1}],"tags":[],"tracks":[]}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/tasks/42/annotations":
			raw, _ := io.ReadAll(r.Body)
			replaced = strings.Contains(string(raw), `"shapes"`)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewCVAT(server.URL, "https://cvat.example.test", "visionai", "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = provider.About(ctx); err != nil {
		t.Fatal(err)
	}
	task, err := provider.CreateTask(ctx, "quality task", []Label{{Name: "defect"}})
	if err != nil || task.ID != 42 {
		t.Fatalf("create: %#v %v", task, err)
	}
	if err = provider.AttachData(ctx, task.ID, []Media{{Name: "sample.png", Reader: strings.NewReader("image-bytes")}}); err != nil || !attached {
		t.Fatalf("attach data: %v attached=%v", err, attached)
	}
	task, err = provider.GetTask(ctx, task.ID)
	if err != nil || task.Size != 1 || task.Progress != 80 {
		t.Fatalf("retrieve: %#v %v", task, err)
	}
	raw, err := provider.GetAnnotations(ctx, task.ID)
	count := strings.Count(string(raw), `"id"`)
	if err != nil || count != 1 {
		t.Fatalf("annotations: %s %v", raw, err)
	}
	if err = provider.PutAnnotations(ctx, task.ID, raw); err != nil || !replaced {
		t.Fatalf("replace annotations: %v replaced=%v", err, replaced)
	}
	if got := provider.TaskURL(task.ID); got != "https://cvat.example.test/tasks/42" {
		t.Fatalf("task URL = %s", got)
	}
}
