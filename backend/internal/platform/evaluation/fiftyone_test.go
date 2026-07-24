package evaluation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSyncDataset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/datasets/tenant-1-project-2-evaluation-3" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var request DatasetRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Samples) != 1 || request.Samples[0].AssetID != 9 {
			t.Fatalf("unexpected payload: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(DatasetResponse{Name: request.Name, SampleCount: len(request.Samples)})
	}))
	defer server.Close()

	client := NewClient(server.URL, time.Second)
	result, err := client.SyncDataset(context.Background(), DatasetRequest{
		Name:    "tenant-1-project-2-evaluation-3",
		Samples: []Sample{{AssetID: 9}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SampleCount != 1 {
		t.Fatalf("unexpected response: %#v", result)
	}
}
