package annotation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type Label struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Type  string `json:"type,omitempty"`
}

type Media struct {
	Name   string
	Reader io.Reader
}

type Task struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Size       int64  `json:"size"`
	Progress   int    `json:"progress"`
	AssigneeID int64  `json:"assignee_id"`
}

type Provider interface {
	About(context.Context) error
	CreateTask(context.Context, string, []Label) (Task, error)
	AttachData(context.Context, int64, []Media) error
	GetTask(context.Context, int64) (Task, error)
	GetAnnotations(context.Context, int64) ([]byte, error)
	PutAnnotations(context.Context, int64, []byte) error
	TaskURL(int64) string
}

type CVAT struct {
	baseURL   string
	publicURL string
	username  string
	password  string
	client    *http.Client
}

func NewCVAT(baseURL, publicURL, username, password string, timeout time.Duration) (*CVAT, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	publicURL = strings.TrimRight(strings.TrimSpace(publicURL), "/")
	if _, err := url.ParseRequestURI(baseURL); err != nil || username == "" || password == "" {
		return nil, errors.New("invalid CVAT provider configuration")
	}
	if publicURL == "" {
		publicURL = baseURL
	}
	return &CVAT{
		baseURL: baseURL, publicURL: publicURL, username: username, password: password,
		client: &http.Client{Timeout: timeout},
	}, nil
}

func (c *CVAT) request(ctx context.Context, method, endpoint string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/vnd.cvat+json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		return nil, fmt.Errorf("CVAT %s %s returned %d: %s", method, endpoint, resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return resp, nil
}

func (c *CVAT) About(ctx context.Context) error {
	resp, err := c.request(ctx, http.MethodGet, "/api/server/about", "", nil)
	if err == nil {
		resp.Body.Close()
	}
	return err
}

func (c *CVAT) CreateTask(ctx context.Context, name string, labels []Label) (Task, error) {
	for index := range labels {
		if labels[index].Type == "" {
			labels[index].Type = "rectangle"
		}
	}
	body, _ := json.Marshal(map[string]any{"name": name, "labels": labels, "segment_size": 50})
	resp, err := c.request(ctx, http.MethodPost, "/api/tasks", "application/json", bytes.NewReader(body))
	if err != nil {
		return Task{}, err
	}
	defer resp.Body.Close()
	var task Task
	err = json.NewDecoder(resp.Body).Decode(&task)
	return task, err
}

func (c *CVAT) AttachData(ctx context.Context, taskID int64, media []Media) error {
	if len(media) == 0 {
		return errors.New("CVAT task requires at least one media file")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("image_quality", "70")
	for index, item := range media {
		part, err := writer.CreateFormFile("client_files["+strconv.Itoa(index)+"]", path.Base(item.Name))
		if err != nil {
			return err
		}
		if _, err = io.Copy(part, item.Reader); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	resp, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/tasks/%d/data", taskID), writer.FormDataContentType(), &body)
	if err == nil {
		resp.Body.Close()
	}
	return err
}

func (c *CVAT) GetTask(ctx context.Context, taskID int64) (Task, error) {
	resp, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/tasks/%d", taskID), "", nil)
	if err != nil {
		return Task{}, err
	}
	defer resp.Body.Close()
	var task Task
	err = json.NewDecoder(resp.Body).Decode(&task)
	return task, err
}

func (c *CVAT) GetAnnotations(ctx context.Context, taskID int64) ([]byte, error) {
	resp, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/tasks/%d/annotations", taskID), "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 512<<20))
}

func (c *CVAT) PutAnnotations(ctx context.Context, taskID int64, annotations []byte) error {
	resp, err := c.request(ctx, http.MethodPut, fmt.Sprintf("/api/tasks/%d/annotations", taskID), "application/json", bytes.NewReader(annotations))
	if err == nil {
		resp.Body.Close()
	}
	return err
}

func (c *CVAT) TaskURL(taskID int64) string {
	return fmt.Sprintf("%s/tasks/%d/jobs", c.publicURL, taskID)
}
