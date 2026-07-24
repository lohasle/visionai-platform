package training

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ClearMLSpec struct {
	Name       string
	Queue      string
	ImageRef   string
	Entrypoint string
	Parameters map[string]any
	Tags       []string
}

type ClearMLTask struct {
	ID            string
	Status        string
	StatusMessage string
	Progress      int
}

type ClearML struct {
	apiURL    string
	webURL    string
	accessKey string
	secretKey string
	client    *http.Client
}

func NewClearML(apiURL, webURL, accessKey, secretKey string, timeout time.Duration) (*ClearML, error) {
	apiURL = strings.TrimRight(strings.TrimSpace(apiURL), "/")
	webURL = strings.TrimRight(strings.TrimSpace(webURL), "/")
	if _, err := url.ParseRequestURI(apiURL); err != nil || accessKey == "" || secretKey == "" {
		return nil, errors.New("invalid ClearML provider configuration")
	}
	if webURL == "" {
		webURL = apiURL
	}
	return &ClearML{apiURL: apiURL, webURL: webURL, accessKey: accessKey, secretKey: secretKey, client: &http.Client{Timeout: timeout}}, nil
}

func (c *ClearML) token(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+"/auth.login?expiration_sec=3600", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.accessKey+":"+c.secretKey)))
	data, err := c.do(req)
	if err != nil {
		return "", err
	}
	var result struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(data, &result) != nil || result.Token == "" {
		return "", errors.New("ClearML auth response missing token")
	}
	return result.Token, nil
}

func (c *ClearML) post(ctx context.Context, endpoint string, body any) (json.RawMessage, error) {
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

func (c *ClearML) do(req *http.Request) (json.RawMessage, error) {
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ClearML %s returned %d: %s", req.URL.Path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) == nil && len(envelope.Data) > 0 {
		return envelope.Data, nil
	}
	return raw, nil
}

func (c *ClearML) CreateAndEnqueue(ctx context.Context, spec ClearMLSpec) (ClearMLTask, error) {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Queue) == "" || !strings.HasPrefix(spec.ImageRef, "sha256:") {
		return ClearMLTask{}, errors.New("invalid ClearML task spec")
	}
	entrypoint := strings.TrimSpace(spec.Entrypoint)
	if entrypoint == "" {
		entrypoint = "run"
	}
	created, err := c.post(ctx, "/tasks.create", map[string]any{
		"name": spec.Name, "type": "training", "tags": spec.Tags,
		"container":   map[string]any{"image": spec.ImageRef},
		"script":      map[string]any{"repository": "visionai://managed-template", "entry_point": entrypoint},
		"hyperparams": map[string]any{"VisionAI": spec.Parameters},
	})
	if err != nil {
		return ClearMLTask{}, err
	}
	var result struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(created, &result) != nil || result.ID == "" {
		return ClearMLTask{}, errors.New("ClearML create response missing task id")
	}
	if _, err = c.post(ctx, "/tasks.enqueue", map[string]any{"task": result.ID, "queue_name": spec.Queue, "verify_watched_queue": false}); err != nil {
		return ClearMLTask{}, err
	}
	return ClearMLTask{ID: result.ID, Status: "queued"}, nil
}

func (c *ClearML) GetTask(ctx context.Context, id string) (ClearMLTask, error) {
	data, err := c.post(ctx, "/tasks.get_by_id", map[string]any{"task": id})
	if err != nil {
		return ClearMLTask{}, err
	}
	var response struct {
		Task struct {
			ID            string `json:"id"`
			Status        string `json:"status"`
			StatusMessage string `json:"status_message"`
			Execution     struct {
				Progress int `json:"progress"`
			} `json:"execution"`
		} `json:"task"`
	}
	if json.Unmarshal(data, &response) != nil || response.Task.ID == "" {
		return ClearMLTask{}, errors.New("ClearML task response invalid")
	}
	return ClearMLTask{ID: response.Task.ID, Status: response.Task.Status, StatusMessage: response.Task.StatusMessage, Progress: response.Task.Execution.Progress}, nil
}

func (c *ClearML) Cancel(ctx context.Context, id string) error {
	_, err := c.post(ctx, "/tasks.dequeue", map[string]any{"task": id, "remove_from_all_queues": true, "new_status": "stopped", "status_reason": "Cancelled by VisionAI"})
	return err
}

func (c *ClearML) TaskURL(id string) string {
	return c.webURL + "/projects/*/experiments/" + url.PathEscape(id)
}
