package trainer

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// TrainConfig holds the training parameters from the UI
type TrainConfig struct {
	Epochs    int    `json:"epochs"`
	BatchSize int    `json:"batch_size"`
	LR        string `json:"lr"` // e.g. "0.001"
	ImageSize int    `json:"img_size"`
	Seed      int    `json:"seed"`
	ValSplit  string `json:"val_split"` // e.g. "0.2"
}

// DefaultTrainConfig returns sensible defaults
func DefaultTrainConfig() TrainConfig {
	return TrainConfig{
		Epochs:    10,
		BatchSize: 16,
		LR:        "0.001",
		ImageSize: 224,
		Seed:      42,
		ValSplit:  "0.2",
	}
}

// TrainStatus represents the current state of a training job
type TrainStatus struct {
	Running      bool         `json:"running"`
	ContainerID  string       `json:"container_id,omitempty"`
	StartedAt    time.Time    `json:"started_at,omitempty"`
	ExitCode     int          `json:"exit_code,omitempty"`
	Error        string       `json:"error,omitempty"`
	Logs         string       `json:"logs,omitempty"`
	LastConfig   *TrainConfig `json:"last_config,omitempty"`
}

// Trainer manages the SkyClf-Trainer Docker container
type Trainer struct {
	mu           sync.RWMutex
	cli          *client.Client
	running      bool
	startedAt    time.Time
	lastExitCode int
	lastError    string
	lastLogs     string
	lastConfig   *TrainConfig

	// Config - container name from compose stack
	containerName string // e.g. "skyclf-trainer"
}

// NewTrainer creates a new Trainer instance
// containerName is the name of the trainer container defined in docker-compose
func NewTrainer(containerName string) (*Trainer, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}

	return &Trainer{
		cli:           cli,
		containerName: containerName,
	}, nil
}

// Close cleans up the Docker client
func (t *Trainer) Close() error {
	if t.cli != nil {
		return t.cli.Close()
	}
	return nil
}

// Status returns the current training status
func (t *Trainer) Status(ctx context.Context) TrainStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Check actual container state
	containerID, isRunning := t.getContainerState(ctx)

	status := TrainStatus{
		Running:     isRunning,
		ContainerID: containerID,
		StartedAt:   t.startedAt,
		ExitCode:    t.lastExitCode,
		Error:       t.lastError,
		Logs:        t.lastLogs,
		LastConfig:  t.lastConfig,
	}

	// If running, get current logs
	if isRunning && containerID != "" {
		logs, err := t.getLogs(ctx, containerID, 100)
		if err == nil {
			status.Logs = logs
		}
	}

	return status
}

// getContainerState checks if the trainer container exists and is running
func (t *Trainer) getContainerState(ctx context.Context) (containerID string, running bool) {
	info, err := t.cli.ContainerInspect(ctx, t.containerName)
	if err != nil {
		return "", false
	}
	return info.ID, info.State.Running
}

// Start starts a training job with the given config
// It recreates the trainer container with the new command arguments
func (t *Trainer) Start(ctx context.Context, cfg TrainConfig) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if already running
	_, isRunning := t.getContainerState(ctx)
	if isRunning {
		return fmt.Errorf("training already in progress")
	}

	// Get the existing container config to preserve settings
	existingInfo, err := t.cli.ContainerInspect(ctx, t.containerName)
	if err != nil {
		return fmt.Errorf("trainer container not found (is docker-compose up?): %w", err)
	}

	// Build new command with training params
	cmd := []string{
		"python", "-m", "trainer.train",
		"--epochs", fmt.Sprintf("%d", cfg.Epochs),
		"--batch", fmt.Sprintf("%d", cfg.BatchSize),
		"--lr", cfg.LR,
		"--img", fmt.Sprintf("%d", cfg.ImageSize),
		"--seed", fmt.Sprintf("%d", cfg.Seed),
		"--val", cfg.ValSplit,
	}

	// Remove old container
	if err := t.cli.ContainerRemove(ctx, t.containerName, container.RemoveOptions{Force: true}); err != nil {
		log.Printf("trainer: warning removing old container: %v", err)
	}

	// Recreate with new command but same config (volumes, env, etc.)
	newConfig := existingInfo.Config
	newConfig.Cmd = cmd

	resp, err := t.cli.ContainerCreate(ctx, newConfig, existingInfo.HostConfig, nil, nil, t.containerName)
	if err != nil {
		return fmt.Errorf("recreate container: %w", err)
	}

	// Start the container
	if err := t.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	t.running = true
	t.startedAt = time.Now()
	t.lastError = ""
	t.lastLogs = ""
	t.lastConfig = &cfg

	// Monitor in background
	go t.monitor(resp.ID)

	log.Printf("trainer: started %s with epochs=%d batch=%d lr=%s", t.containerName, cfg.Epochs, cfg.BatchSize, cfg.LR)
	return nil
}

// Stop stops the running training job
func (t *Trainer) Stop(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	containerID, isRunning := t.getContainerState(ctx)
	if !isRunning {
		return fmt.Errorf("no training in progress")
	}

	timeout := 10
	if err := t.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}

	log.Printf("trainer: stopped %s", t.containerName)
	return nil
}

// monitor watches the container and updates status when it exits
func (t *Trainer) monitor(containerID string) {
	ctx := context.Background()

	statusCh, errCh := t.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	select {
	case err := <-errCh:
		t.mu.Lock()
		t.running = false
		t.lastError = err.Error()
		t.mu.Unlock()
		log.Printf("trainer: wait error: %v", err)

	case result := <-statusCh:
		logs, _ := t.getLogs(ctx, containerID, 500)

		t.mu.Lock()
		t.running = false
		t.lastExitCode = int(result.StatusCode)
		t.lastLogs = logs
		if result.Error != nil {
			t.lastError = result.Error.Message
		} else if result.StatusCode != 0 {
			t.lastError = fmt.Sprintf("training failed with exit code %d", result.StatusCode)
		}
		t.mu.Unlock()

		if result.StatusCode == 0 {
			log.Printf("trainer: completed successfully")
		} else {
			log.Printf("trainer: exited with code %d", result.StatusCode)
		}
	}
}

// getLogs retrieves the last N lines of container logs
func (t *Trainer) getLogs(ctx context.Context, containerID string, tail int) (string, error) {
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", tail),
	}

	reader, err := t.cli.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return stripDockerLogHeaders(data), nil
}

// stripDockerLogHeaders removes the 8-byte header from each docker log line
func stripDockerLogHeaders(data []byte) string {
	var lines []string
	for len(data) >= 8 {
		// Header: [stream_type, 0, 0, 0, size(4 bytes big-endian)]
		size := int(data[4])<<24 | int(data[5])<<16 | int(data[6])<<8 | int(data[7])
		data = data[8:]

		if size > len(data) {
			size = len(data)
		}
		if size > 0 {
			lines = append(lines, string(data[:size]))
		}
		data = data[size:]
	}
	return strings.Join(lines, "")
}
