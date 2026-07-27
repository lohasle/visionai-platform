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
	"strconv"
	"strings"
	"time"
)

type ClearMLSpec struct {
	Name               string
	Queue              string
	ImageRef           string
	Entrypoint         string
	ScriptBinary       string
	ContainerArguments []string
	Parameters         map[string]any
	Tags               []string
}

type ClearMLTask struct {
	ID            string
	Status        string
	StatusMessage string
	Progress      int
	WorkerID      string
}

type ClearMLWorker struct {
	ID         string
	LastReport time.Time
	Queues     []string
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
	binary := strings.TrimSpace(spec.ScriptBinary)
	if binary == "" {
		binary = "python3"
	}
	created, err := c.post(ctx, "/tasks.create", map[string]any{
		"name": spec.Name, "type": "training", "tags": spec.Tags,
		"container": map[string]any{
			"image":     spec.ImageRef,
			"arguments": shellJoin(spec.ContainerArguments),
		},
		"script": map[string]any{
			"repository":  "",
			"entry_point": entrypoint,
			"working_dir": ".",
			"binary":      binary,
			"requirements": map[string]string{
				"pip": "",
			},
		},
		"hyperparams": map[string]any{"VisionAI": clearMLHyperParameters("VisionAI", spec.Parameters)},
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

func clearMLHyperParameters(section string, values map[string]any) map[string]any {
	result := make(map[string]any, len(values))
	for name, value := range values {
		valueType := "str"
		var serialized string
		switch typed := value.(type) {
		case bool:
			valueType, serialized = "bool", strconv.FormatBool(typed)
		case float64:
			valueType, serialized = "float", strconv.FormatFloat(typed, 'g', -1, 64)
		case float32:
			valueType, serialized = "float", strconv.FormatFloat(float64(typed), 'g', -1, 32)
		case int:
			valueType, serialized = "int", strconv.Itoa(typed)
		case int64:
			valueType, serialized = "int", strconv.FormatInt(typed, 10)
		case uint64:
			valueType, serialized = "int", strconv.FormatUint(typed, 10)
		case string:
			serialized = typed
		default:
			raw, err := json.Marshal(typed)
			if err != nil {
				serialized = fmt.Sprint(typed)
			} else {
				valueType, serialized = "json", string(raw)
			}
		}
		result[name] = map[string]any{
			"section": section,
			"name":    name,
			"value":   serialized,
			"type":    valueType,
		}
	}
	return result
}

func shellJoin(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			quoted = append(quoted, "''")
			continue
		}
		if !strings.ContainsAny(value, " \t\r\n'\"\\$`;&|<>*?()[]{}!") {
			quoted = append(quoted, value)
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(value, "'", "'\"'\"'")+"'")
	}
	return strings.Join(quoted, " ")
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
			LastWorker    string `json:"last_worker"`
			Execution     struct {
				Progress int `json:"progress"`
			} `json:"execution"`
		} `json:"task"`
	}
	if json.Unmarshal(data, &response) != nil || response.Task.ID == "" {
		return ClearMLTask{}, errors.New("ClearML task response invalid")
	}
	return ClearMLTask{
		ID: response.Task.ID, Status: response.Task.Status,
		StatusMessage: response.Task.StatusMessage, Progress: response.Task.Execution.Progress,
		WorkerID: response.Task.LastWorker,
	}, nil
}

func (c *ClearML) Workers(ctx context.Context) ([]ClearMLWorker, error) {
	data, err := c.post(ctx, "/workers.get_all", map[string]any{})
	if err != nil {
		return nil, err
	}
	var response struct {
		Workers []struct {
			ID             string `json:"id"`
			LastReportTime string `json:"last_report_time"`
			Queues         []struct {
				Name string `json:"name"`
			} `json:"queues"`
		} `json:"workers"`
	}
	if err = json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("invalid ClearML workers response: %w", err)
	}
	result := make([]ClearMLWorker, 0, len(response.Workers))
	for _, worker := range response.Workers {
		if strings.TrimSpace(worker.ID) == "" {
			continue
		}
		lastReport, _ := time.Parse(time.RFC3339Nano, worker.LastReportTime)
		item := ClearMLWorker{ID: worker.ID, LastReport: lastReport}
		for _, queue := range worker.Queues {
			if queue.Name != "" {
				item.Queues = append(item.Queues, queue.Name)
			}
		}
		result = append(result, item)
	}
	return result, nil
}

func (c *ClearML) Cancel(ctx context.Context, id string) error {
	_, err := c.post(ctx, "/tasks.dequeue", map[string]any{"task": id, "remove_from_all_queues": true, "new_status": "stopped", "status_reason": "Cancelled by VisionAI"})
	return err
}

// TaskLog returns the provider-captured console output. ClearML events are the
// canonical log source for externally scheduled tasks.
func (c *ClearML) TaskLog(ctx context.Context, id string) ([]byte, error) {
	data, err := c.post(ctx, "/events.get_task_log", map[string]any{
		"task": id, "order": "asc", "batch_size": 10000,
	})
	if err != nil {
		return nil, err
	}
	var response struct {
		Events []struct {
			Timestamp int64  `json:"timestamp"`
			Level     string `json:"level"`
			Worker    string `json:"worker"`
			Message   string `json:"msg"`
		} `json:"events"`
	}
	if err = json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("invalid ClearML task log response: %w", err)
	}
	var log strings.Builder
	for _, event := range response.Events {
		if event.Timestamp > 0 {
			fmt.Fprintf(&log, "%d ", event.Timestamp)
		}
		if event.Level != "" {
			fmt.Fprintf(&log, "[%s] ", strings.ToUpper(event.Level))
		}
		if event.Worker != "" {
			fmt.Fprintf(&log, "%s: ", event.Worker)
		}
		log.WriteString(event.Message)
		log.WriteByte('\n')
	}
	return []byte(log.String()), nil
}

// ReportMetrics mirrors the framework-neutral result metrics into ClearML so
// its Scalars view and VisionAI's own experiment curves show the same values.
func (c *ClearML) ReportMetrics(ctx context.Context, id string, metrics []ResultMetric) error {
	for _, item := range metrics {
		metric, variant := "VisionAI", strings.TrimSpace(item.Name)
		if before, after, found := strings.Cut(variant, "/"); found {
			metric, variant = before, after
		}
		if variant == "" {
			variant = "value"
		}
		data, err := c.post(ctx, "/events.add", map[string]any{
			"task": id, "type": "training_stats_scalar",
			"metric": metric, "variant": variant,
			"value": item.Value, "iter": item.Step,
			"timestamp": time.Now().UnixMilli(),
		})
		if err != nil {
			return err
		}
		var response struct {
			Added  int `json:"added"`
			Errors int `json:"errors"`
		}
		if json.Unmarshal(data, &response) != nil || response.Added != 1 || response.Errors != 0 {
			return errors.New("ClearML scalar metric was not accepted")
		}
	}
	return nil
}

func (c *ClearML) TaskURL(id string) string {
	return c.webURL + "/projects/*/tasks/" + url.PathEscape(id) + "/execution"
}
