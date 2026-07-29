package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestActivateAndPredict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/revisions/7":
			if r.Method != http.MethodPut {
				t.Fatal("unexpected method")
			}
			w.WriteHeader(http.StatusOK)
		case "/v1/predict":
			if r.Method != http.MethodPost {
				t.Fatal("unexpected method")
			}
			_ = json.NewEncoder(w).Encode(Prediction{ModelVersionID: 4, RevisionID: 7})
		case "/admin/evaluations/11":
			if r.Method != http.MethodPut {
				t.Fatal("unexpected method")
			}
			w.WriteHeader(http.StatusOK)
		case "/v1/evaluations/11/predict":
			if r.URL.Query().Get("training_run_id") != "13" || r.URL.Query().Get("artifact_sha256") != "abc" {
				t.Fatalf("unexpected evaluation query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(Prediction{ModelVersionID: 13, RevisionID: 11})
		case "/v1/predict-video":
			if r.Method != http.MethodPost {
				t.Fatal("unexpected method")
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			_ = file.Close()
			_ = json.NewEncoder(w).Encode(VideoPrediction{
				ModelVersionID: 4,
				RevisionID:     7,
				Frames:         []json.RawMessage{json.RawMessage(`{"frameIndex":0}`)},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, time.Second)
	if err := client.Activate(context.Background(), Revision{RevisionID: 7, ModelVersionID: 4}); err != nil {
		t.Fatal(err)
	}
	result, err := client.Predict(context.Background(), "sample.png", bytes.NewBufferString("png"))
	if err != nil || result.RevisionID != 7 {
		t.Fatalf("unexpected result %#v err=%v", result, err)
	}
	if err = client.LoadEvaluation(context.Background(), 11, EvaluationModel{TrainingRunID: 13, ArtifactSHA256: "abc"}); err != nil {
		t.Fatal(err)
	}
	evaluated, err := client.PredictEvaluation(context.Background(), 11, 13, "abc", "sample.png", bytes.NewBufferString("png"))
	if err != nil || evaluated.ModelVersionID != 13 || evaluated.RevisionID != 11 {
		t.Fatalf("unexpected evaluation result %#v err=%v", evaluated, err)
	}
	video, err := client.PredictVideo(context.Background(), "sample.mp4", bytes.NewBufferString("video"))
	if err != nil || video.RevisionID != 7 || len(video.Frames) != 1 {
		t.Fatalf("unexpected video result %#v err=%v", video, err)
	}
}
