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
			if payload["segment_size"] != float64(25) {
				t.Fatalf("unexpected segment size: %#v", payload)
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
	task, err := provider.CreateTask(ctx, "quality task", []Label{{Name: "defect"}}, 25)
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

func TestCVATIdentitySessionAndJobAssignment(t *testing.T) {
	var assigned []int64
	var taskAssignee int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/auth/register":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"username":"va_user"}`)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "personal-session", Path: "/", HttpOnly: true})
			_, _ = io.WriteString(w, `{"key":"token"}`)
			return
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "visionai" || password != "secret" {
			t.Fatalf("missing provider credentials")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/users":
			_, _ = io.WriteString(w, `{"results":[{"id":7,"username":"va_user","is_active":true}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/jobs":
			_, _ = io.WriteString(w, `{"results":[{"id":101,"task_id":42},{"id":102,"task_id":42},{"id":103,"task_id":42}]}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/tasks/42":
			var payload map[string]int64
			_ = json.NewDecoder(r.Body).Decode(&payload)
			taskAssignee = payload["assignee_id"]
			_, _ = io.WriteString(w, `{"id":42}`)
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/jobs/"):
			var payload map[string]int64
			_ = json.NewDecoder(r.Body).Decode(&payload)
			assigned = append(assigned, payload["assignee"])
			_, _ = io.WriteString(w, `{"id":101,"task_id":42}`)
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
	if err = provider.RegisterUser(ctx, "va_user", "va@example.test", "secret-value", "VA", "User"); err != nil {
		t.Fatal(err)
	}
	user, err := provider.FindUser(ctx, "va_user")
	if err != nil || user.ID != 7 {
		t.Fatalf("find user: %+v %v", user, err)
	}
	cookies, err := provider.Login(ctx, "va_user", "secret-value")
	if err != nil || len(cookies) != 1 || !strings.Contains(cookies[0], "sessionid=personal-session") {
		t.Fatalf("login cookies: %#v %v", cookies, err)
	}
	if _, err = provider.AssignTaskJobs(ctx, 42, []int64{7, 8}); err != nil {
		t.Fatal(err)
	}
	if err = provider.AssignTask(ctx, 42, 9); err != nil || taskAssignee != 9 {
		t.Fatalf("task assignment = %d, %v", taskAssignee, err)
	}
	want := []int64{7, 8, 7}
	if len(assigned) != len(want) {
		t.Fatalf("assignments: %#v", assigned)
	}
	for index := range want {
		if assigned[index] != want[index] {
			t.Fatalf("assignments: %#v", assigned)
		}
	}
	if got := provider.JobURL(42, 101); got != "https://cvat.example.test/tasks/42/jobs/101" {
		t.Fatalf("job URL = %s", got)
	}
}
