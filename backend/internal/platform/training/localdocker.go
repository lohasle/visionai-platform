package training

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type LocalDockerSpec struct {
	RunID              uint64
	ImageRef           string
	Entrypoint         string
	OutputDir          string
	InputDir           string
	DatasetManifestURI string
	ParametersJSON     string
	GPUCount           int
	MemoryBytes        int64
	CPUs               float64
}

type ResultArtifact struct {
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
}

type ResultMetric struct {
	Name  string  `json:"name"`
	Step  int64   `json:"step"`
	Value float64 `json:"value"`
}

type ResultManifest struct {
	SchemaVersion string           `json:"schemaVersion"`
	Status        string           `json:"status"`
	Artifacts     []ResultArtifact `json:"artifacts"`
	Metrics       []ResultMetric   `json:"metrics"`
	Summary       map[string]any   `json:"summary"`
}

type LocalDocker struct {
	Binary      string
	VolumesFrom string
}

func (p LocalDocker) Run(ctx context.Context, spec LocalDockerSpec) (ResultManifest, []byte, error) {
	if strings.TrimSpace(spec.ImageRef) == "" || spec.RunID == 0 {
		return ResultManifest{}, nil, errors.New("invalid LocalDocker training spec")
	}
	outputDir, err := PrepareOutputDirectory(spec.OutputDir)
	if err != nil {
		return ResultManifest{}, nil, err
	}
	inputTarget := ""
	if strings.TrimSpace(spec.InputDir) != "" {
		inputDir, inputErr := filepath.Abs(spec.InputDir)
		if inputErr != nil {
			return ResultManifest{}, nil, inputErr
		}
		if info, statErr := os.Stat(inputDir); statErr != nil || !info.IsDir() {
			return ResultManifest{}, nil, errors.New("training input directory is unavailable")
		}
		inputTarget = "/input"
		if strings.TrimSpace(p.VolumesFrom) != "" {
			inputTarget = inputDir
		}
	}
	binary := strings.TrimSpace(p.Binary)
	if binary == "" {
		binary = "docker"
	}
	args := []string{
		"run", "--rm", "--name", "visionai-training-" + strconv.FormatUint(spec.RunID, 10),
		"--network", "none", "--read-only", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--pids-limit", "256",
	}
	outputTarget := "/output"
	if strings.TrimSpace(p.VolumesFrom) != "" {
		args = append(args, "--volumes-from", strings.TrimSpace(p.VolumesFrom))
		outputTarget = outputDir
	} else {
		args = append(args, "--mount", "type=bind,src="+outputDir+",dst=/output")
		if inputTarget != "" {
			args = append(args, "--mount", "type=bind,src="+spec.InputDir+",dst=/input,readonly")
		}
	}
	args = append(args,
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=256m",
		"-e", "VISIONAI_RUN_ID="+strconv.FormatUint(spec.RunID, 10),
		"-e", "VISIONAI_OUTPUT_DIR="+outputTarget,
		"-e", "VISIONAI_DATASET_MANIFEST_URI="+spec.DatasetManifestURI,
		"-e", "VISIONAI_PARAMETERS_JSON="+spec.ParametersJSON,
	)
	if inputTarget != "" {
		args = append(args, "-e", "VISIONAI_INPUT_DIR="+inputTarget)
	}
	if spec.MemoryBytes > 0 {
		args = append(args, "--memory", strconv.FormatInt(spec.MemoryBytes, 10))
	}
	if spec.CPUs > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(spec.CPUs, 'f', 2, 64))
	}
	if spec.GPUCount > 0 {
		args = append(args, "--gpus", strconv.Itoa(spec.GPUCount))
	}
	args = append(args, spec.ImageRef)
	if entrypoint := strings.TrimSpace(spec.Entrypoint); entrypoint != "" {
		args = append(args, strings.Fields(entrypoint)...)
	}
	command := exec.CommandContext(ctx, binary, args...)
	containerName := "visionai-training-" + strconv.FormatUint(spec.RunID, 10)
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanupCtx, binary, "rm", "-f", containerName).Run()
	}()
	log, runErr := command.CombinedOutput()
	if runErr != nil {
		return ResultManifest{}, log, fmt.Errorf("LocalDocker training failed: %w", runErr)
	}
	manifest, err := ReadResultManifest(outputDir)
	return manifest, log, err
}

// PrepareOutputDirectory creates a portable output directory for a fixed,
// unprivileged trainer UID. It is shared by synchronous LocalDocker execution
// and asynchronous ClearML Agent execution.
func PrepareOutputDirectory(value string) (string, error) {
	outputDir, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(outputDir, 0o750); err != nil {
		return "", err
	}
	// MkdirAll can create the per-feature parent (for example
	// /training-work/template-smoke) with mode 0750. The trainer's fixed UID
	// needs execute permission on that parent before it can reach the
	// deliberately world-writable per-run directory below.
	if err = os.Chmod(filepath.Dir(outputDir), 0o755); err != nil {
		return "", err
	}
	// The per-run directory contains only generated artifacts and must be
	// writable across Linux, WSL2, and Docker Desktop UID mappings.
	if err = os.Chmod(outputDir, 0o777); err != nil {
		return "", err
	}
	return outputDir, nil
}

// ReadResultManifest validates the framework-neutral result contract and every
// referenced artifact before callers persist any result.
func ReadResultManifest(outputDir string) (ResultManifest, error) {
	outputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return ResultManifest{}, err
	}
	raw, err := os.ReadFile(filepath.Join(outputDir, "result-manifest.json"))
	if err != nil {
		return ResultManifest{}, fmt.Errorf("result-manifest missing: %w", err)
	}
	var manifest ResultManifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return ResultManifest{}, fmt.Errorf("invalid result-manifest: %w", err)
	}
	if manifest.SchemaVersion != "visionai.result-manifest.v1" || manifest.Status != "SUCCEEDED" {
		return ResultManifest{}, errors.New("unsupported or unsuccessful result-manifest")
	}
	for _, artifact := range manifest.Artifacts {
		clean := filepath.Clean(filepath.FromSlash(artifact.Path))
		if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return ResultManifest{}, errors.New("result-manifest contains unsafe artifact path")
		}
		resolved := filepath.Join(outputDir, clean)
		relative, relErr := filepath.Rel(outputDir, resolved)
		if relErr != nil || strings.HasPrefix(relative, "..") {
			return ResultManifest{}, errors.New("artifact escapes output directory")
		}
		if info, statErr := os.Stat(resolved); statErr != nil || !info.Mode().IsRegular() {
			return ResultManifest{}, fmt.Errorf("artifact missing: %s", artifact.Path)
		}
	}
	return manifest, nil
}
