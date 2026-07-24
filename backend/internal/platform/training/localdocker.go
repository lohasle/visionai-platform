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
	outputDir, err := filepath.Abs(spec.OutputDir)
	if err != nil {
		return ResultManifest{}, nil, err
	}
	if err = os.MkdirAll(outputDir, 0o750); err != nil {
		return ResultManifest{}, nil, err
	}
	// MkdirAll can create the per-feature parent (for example
	// /training-work/template-smoke) with mode 0750. The trainer's fixed UID
	// needs execute permission on that parent before it can reach the
	// deliberately world-writable per-run directory below.
	if err = os.Chmod(filepath.Dir(outputDir), 0o755); err != nil {
		return ResultManifest{}, nil, err
	}
	// The container runs as an unprivileged fixed UID. The per-run directory
	// contains only generated artifacts and must be writable across host UID
	// mappings (Linux, WSL2, and Docker Desktop).
	if err = os.Chmod(outputDir, 0o777); err != nil {
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
	log, runErr := command.CombinedOutput()
	if runErr != nil {
		return ResultManifest{}, log, fmt.Errorf("LocalDocker training failed: %w", runErr)
	}
	manifestPath := filepath.Join(outputDir, "result-manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return ResultManifest{}, log, fmt.Errorf("result-manifest missing: %w", err)
	}
	var manifest ResultManifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return ResultManifest{}, log, fmt.Errorf("invalid result-manifest: %w", err)
	}
	if manifest.SchemaVersion != "visionai.result-manifest.v1" || manifest.Status != "SUCCEEDED" {
		return ResultManifest{}, log, errors.New("unsupported or unsuccessful result-manifest")
	}
	for _, artifact := range manifest.Artifacts {
		clean := filepath.Clean(filepath.FromSlash(artifact.Path))
		if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return ResultManifest{}, log, errors.New("result-manifest contains unsafe artifact path")
		}
		resolved := filepath.Join(outputDir, clean)
		relative, relErr := filepath.Rel(outputDir, resolved)
		if relErr != nil || strings.HasPrefix(relative, "..") {
			return ResultManifest{}, log, errors.New("artifact escapes output directory")
		}
		if info, statErr := os.Stat(resolved); statErr != nil || !info.Mode().IsRegular() {
			return ResultManifest{}, log, fmt.Errorf("artifact missing: %s", artifact.Path)
		}
	}
	return manifest, log, nil
}
