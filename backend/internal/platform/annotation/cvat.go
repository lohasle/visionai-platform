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

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Active    bool   `json:"is_active"`
}

type Job struct {
	ID       int64  `json:"id"`
	TaskID   int64  `json:"task_id"`
	Stage    string `json:"stage"`
	State    string `json:"state"`
	Assignee *User  `json:"assignee"`
}

type Provider interface {
	About(context.Context) error
	CreateTask(context.Context, string, []Label, int) (Task, error)
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

func (c *CVAT) CreateTask(ctx context.Context, name string, labels []Label, segmentSize int) (Task, error) {
	for index := range labels {
		if labels[index].Type == "" {
			labels[index].Type = "rectangle"
		}
	}
	if segmentSize < 1 {
		segmentSize = 50
	}
	body, _ := json.Marshal(map[string]any{"name": name, "labels": labels, "segment_size": segmentSize})
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
	return fmt.Sprintf("%s/tasks/%d", c.publicURL, taskID)
}

func (c *CVAT) JobURL(taskID, jobID int64) string {
	return fmt.Sprintf("%s/tasks/%d/jobs/%d", c.publicURL, taskID, jobID)
}

func (c *CVAT) AssignTask(ctx context.Context, taskID, assigneeID int64) error {
	body, _ := json.Marshal(map[string]int64{"assignee_id": assigneeID})
	resp, err := c.request(ctx, http.MethodPatch, fmt.Sprintf("/api/tasks/%d", taskID), "application/json", bytes.NewReader(body))
	if err == nil {
		resp.Body.Close()
	}
	return err
}

func (c *CVAT) RegisterUser(ctx context.Context, username, email, password, firstName, lastName string) error {
	body, _ := json.Marshal(map[string]string{
		"username": username, "email": email, "password1": password, "password2": password,
		"first_name": firstName, "last_name": lastName,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/auth/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.cvat+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		return fmt.Errorf("CVAT register user returned %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return nil
}

func (c *CVAT) FindUser(ctx context.Context, username string) (User, error) {
	resp, err := c.request(ctx, http.MethodGet, "/api/users?search="+url.QueryEscape(username)+"&page_size=100", "", nil)
	if err != nil {
		return User{}, err
	}
	defer resp.Body.Close()
	var page struct {
		Results []User `json:"results"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return User{}, err
	}
	for _, user := range page.Results {
		if user.Username == username {
			return user, nil
		}
	}
	return User{}, fmt.Errorf("CVAT user %q not found", username)
}

func (c *CVAT) Login(ctx context.Context, username, password string) ([]string, error) {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/auth/login", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.cvat+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		return nil, fmt.Errorf("CVAT login returned %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	cookies := resp.Header.Values("Set-Cookie")
	if len(cookies) == 0 {
		return nil, errors.New("CVAT login returned no session cookies")
	}
	return cookies, nil
}

func (c *CVAT) ListTaskJobs(ctx context.Context, taskID int64) ([]Job, error) {
	resp, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/jobs?task_id=%d&page_size=1000", taskID), "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var page struct {
		Results []Job `json:"results"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}
	return page.Results, nil
}

func (c *CVAT) AssignTaskJobs(ctx context.Context, taskID int64, assigneeIDs []int64) ([]Job, error) {
	if len(assigneeIDs) == 0 {
		return nil, errors.New("CVAT job assignment requires at least one assignee")
	}
	var jobs []Job
	var err error
	for attempt := 0; attempt < 60; attempt++ {
		jobs, err = c.ListTaskJobs(ctx, taskID)
		if err == nil && len(jobs) > 0 {
			break
		}
		if err != nil && attempt > 4 {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if len(jobs) == 0 {
		return nil, errors.New("CVAT did not create annotation jobs in time")
	}
	for index := range jobs {
		body, _ := json.Marshal(map[string]int64{"assignee": assigneeIDs[index%len(assigneeIDs)]})
		resp, patchErr := c.request(ctx, http.MethodPatch, fmt.Sprintf("/api/jobs/%d", jobs[index].ID), "application/json", bytes.NewReader(body))
		if patchErr != nil {
			return nil, patchErr
		}
		var updated Job
		patchErr = json.NewDecoder(resp.Body).Decode(&updated)
		resp.Body.Close()
		if patchErr != nil {
			return nil, patchErr
		}
		jobs[index] = updated
	}
	return jobs, nil
}
