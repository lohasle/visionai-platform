package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Revision struct {
	DeploymentID   uint64         `json:"deploymentId"`
	RevisionID     uint64         `json:"revisionId"`
	ModelVersionID uint64         `json:"modelVersionId"`
	ArtifactURI    string         `json:"artifactUri"`
	ArtifactSHA256 string         `json:"artifactSha256"`
	Config         map[string]any `json:"config"`
}

type Prediction struct {
	ModelVersionID uint64 `json:"modelVersionId"`
	RevisionID     uint64 `json:"revisionId"`
	Image          struct {
		Width  int    `json:"width"`
		Height int    `json:"height"`
		SHA256 string `json:"sha256"`
	} `json:"image"`
	Detections []struct {
		Label      string    `json:"label"`
		Confidence float64   `json:"confidence"`
		BBox       []float64 `json:"bbox"`
	} `json:"detections"`
	LatencyMS float64 `json:"latencyMs"`
	Engine    string  `json:"engine"`
}

type VideoPrediction struct {
	ModelVersionID uint64 `json:"modelVersionId"`
	RevisionID     uint64 `json:"revisionId"`
	Video          struct {
		Width         int    `json:"width"`
		Height        int    `json:"height"`
		SampledFrames int    `json:"sampledFrames"`
		SHA256        string `json:"sha256"`
	} `json:"video"`
	Frames    []json.RawMessage `json:"frames"`
	LatencyMS float64           `json:"latencyMs"`
	Engine    string            `json:"engine"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: timeout}}
}

func (c *Client) Activate(ctx context.Context, revision Revision) error {
	body, _ := json.Marshal(revision)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/admin/revisions/"+strconv.FormatUint(revision.RevisionID, 10), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("activate inference revision: HTTP %d", response.StatusCode)
	}
	return nil
}

func (c *Client) Stop(ctx context.Context, revisionID uint64) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/admin/revisions/"+strconv.FormatUint(revisionID, 10), nil)
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("stop inference revision: HTTP %d", response.StatusCode)
	}
	return nil
}

func (c *Client) Predict(ctx context.Context, filename string, input io.Reader) (Prediction, error) {
	var result Prediction
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return result, err
	}
	if _, err = io.Copy(part, input); err != nil {
		return result, err
	}
	_ = writer.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/predict", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := c.http.Do(req)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("inference request: HTTP %d", response.StatusCode)
	}
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

func (c *Client) PredictVideo(ctx context.Context, filename string, input io.Reader) (VideoPrediction, error) {
	var result VideoPrediction
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return result, err
	}
	if _, err = io.Copy(part, input); err != nil {
		return result, err
	}
	_ = writer.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/predict-video", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := c.http.Do(req)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("video inference request: HTTP %d", response.StatusCode)
	}
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}
