package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Detection struct {
	Label       string    `json:"label"`
	Confidence  float64   `json:"confidence,omitempty"`
	BoundingBox []float64 `json:"boundingBox"`
}

type Sample struct {
	AssetID     uint64      `json:"assetId"`
	SourceURL   string      `json:"sourceUrl"`
	Split       string      `json:"split"`
	Slice       string      `json:"slice"`
	ErrorType   string      `json:"errorType"`
	IoU         float64     `json:"iou"`
	LatencyMS   float64     `json:"latencyMs"`
	GroundTruth []Detection `json:"groundTruth"`
	Predictions []Detection `json:"predictions"`
}

type DatasetRequest struct {
	Name      string         `json:"name"`
	ProjectID uint64         `json:"projectId"`
	RunID     uint64         `json:"runId"`
	Metadata  map[string]any `json:"metadata"`
	Samples   []Sample       `json:"samples"`
}

type DatasetResponse struct {
	Name        string `json:"name"`
	SampleCount int    `json:"sampleCount"`
}

type SimilaritySample struct {
	AssetID   uint64 `json:"assetId"`
	Filename  string `json:"filename"`
	SourceURL string `json:"sourceUrl"`
}

type SimilarityRequest struct {
	Name              string             `json:"name"`
	ProjectID         uint64             `json:"projectId"`
	DistanceThreshold int                `json:"distanceThreshold"`
	Samples           []SimilaritySample `json:"samples"`
}

type SimilarityGroup struct {
	CanonicalAssetID uint64            `json:"canonicalAssetId"`
	AssetIDs         []uint64          `json:"assetIds"`
	Hashes           map[string]string `json:"hashes"`
}

type SimilarityResponse struct {
	Name          string            `json:"name"`
	AnalyzedCount int               `json:"analyzedCount"`
	Groups        []SimilarityGroup `json:"groups"`
	Hashes        map[string]string `json:"hashes"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) SyncDataset(ctx context.Context, payload DatasetRequest) (DatasetResponse, error) {
	var result DatasetResponse
	if c.baseURL == "" {
		return result, fmt.Errorf("FiftyOne API URL is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return result, fmt.Errorf("encode FiftyOne dataset: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/datasets/"+payload.Name, bytes.NewReader(body))
	if err != nil {
		return result, fmt.Errorf("create FiftyOne request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return result, fmt.Errorf("sync FiftyOne dataset: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("sync FiftyOne dataset: HTTP %d", response.StatusCode)
	}
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("decode FiftyOne response: %w", err)
	}
	return result, nil
}

func (c *Client) AnalyzeSimilarity(ctx context.Context, payload SimilarityRequest) (SimilarityResponse, error) {
	var result SimilarityResponse
	if c.baseURL == "" {
		return result, fmt.Errorf("FiftyOne API URL is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return result, fmt.Errorf("encode FiftyOne similarity request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/similarity", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return result, fmt.Errorf("analyze FiftyOne similarity: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("analyze FiftyOne similarity: HTTP %d", response.StatusCode)
	}
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("decode FiftyOne similarity response: %w", err)
	}
	return result, nil
}
