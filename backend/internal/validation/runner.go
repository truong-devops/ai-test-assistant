package validation

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type SandboxRunner interface {
	Run(ctx context.Context, request SandboxRequest) (SandboxResult, error)
}

type DockerConfig struct {
	Binary         string
	Image          string
	Timeout        time.Duration
	MemoryMB       int
	CPUs           float64
	PIDsLimit      int
	MaxOutputBytes int
}

type CommandExecutor interface {
	Run(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error
}

type OSCommandExecutor struct{}

func (OSCommandExecutor) Run(ctx context.Context, stdout, stderr io.Writer,
	name string, args ...string,
) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

type DockerRunner struct {
	config   DockerConfig
	executor CommandExecutor
}

var dockerImageReference = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]*$`)

func NewDockerRunner(config DockerConfig) (*DockerRunner, error) {
	return newDockerRunner(config, OSCommandExecutor{})
}

func newDockerRunner(config DockerConfig, executor CommandExecutor) (*DockerRunner, error) {
	if config.Binary == "" {
		config.Binary = "docker"
	}
	if !dockerImageReference.MatchString(config.Image) {
		return nil, fmt.Errorf("sandbox image reference is invalid")
	}
	if config.Timeout <= 0 || config.Timeout > 15*time.Minute ||
		config.MemoryMB < 64 || config.MemoryMB > 16384 ||
		config.CPUs < .1 || config.CPUs > 32 ||
		config.PIDsLimit < 16 || config.PIDsLimit > 4096 {
		return nil, fmt.Errorf("invalid sandbox resource limits")
	}
	if config.MaxOutputBytes <= 0 {
		config.MaxOutputBytes = DefaultMaxOutputBytes
	}
	if executor == nil {
		return nil, fmt.Errorf("Docker command executor is required")
	}
	return &DockerRunner{config: config, executor: executor}, nil
}

func (r *DockerRunner) Run(ctx context.Context, request SandboxRequest) (SandboxResult, error) {
	workspace, err := filepath.Abs(request.Workspace)
	if err != nil || strings.TrimSpace(request.Workspace) == "" {
		return SandboxResult{}, fmt.Errorf("invalid sandbox workspace")
	}
	if len(request.Command) == 0 {
		return SandboxResult{}, fmt.Errorf("sandbox command is required")
	}
	for _, argument := range request.Command {
		if argument == "" || strings.ContainsRune(argument, '\x00') {
			return SandboxResult{}, fmt.Errorf("sandbox command contains an invalid argument")
		}
	}
	containerName, err := randomContainerName()
	if err != nil {
		return SandboxResult{}, err
	}
	created := false
	defer func() {
		if created {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			_ = r.executor.Run(cleanupCtx, io.Discard, io.Discard, r.config.Binary,
				"rm", "--force", "--volumes", containerName)
		}
	}()

	var createOutput, createError bytes.Buffer
	createErr := r.executor.Run(ctx, &createOutput, &createError, r.config.Binary,
		r.createArguments(containerName, request.Command)...)
	created = createErr == nil || strings.TrimSpace(createOutput.String()) != ""
	if createErr != nil {
		return SandboxResult{}, fmt.Errorf("create sandbox container: %w: %s", createErr,
			compactDockerError(createError.String()))
	}
	copySource := workspace + string(filepath.Separator) + "."
	var copyError bytes.Buffer
	if err := r.executor.Run(ctx, io.Discard, &copyError, r.config.Binary,
		"cp", copySource, containerName+":/workspace"); err != nil {
		return SandboxResult{}, fmt.Errorf("copy validation workspace into sandbox: %w: %s", err,
			compactDockerError(copyError.String()))
	}

	stdout := newLimitedBuffer(r.config.MaxOutputBytes)
	stderr := newLimitedBuffer(r.config.MaxOutputBytes)
	executionCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	started := time.Now()
	startErr := r.executor.Run(executionCtx, stdout, stderr, r.config.Binary,
		"start", "--attach", containerName)
	duration := time.Since(started)
	timedOut := errors.Is(executionCtx.Err(), context.DeadlineExceeded)
	cancel()
	if timedOut {
		return SandboxResult{ExitCode: -1, Duration: duration, Stdout: sanitizeOutput(stdout.String()),
			Stderr: sanitizeOutput(stderr.String()), TimedOut: true,
			OutputTruncated: stdout.Truncated() || stderr.Truncated()}, nil
	}
	if err := ctx.Err(); err != nil {
		return SandboxResult{}, err
	}
	exitCode, inspectErr := r.inspectExitCode(ctx, containerName)
	if inspectErr != nil {
		if startErr != nil {
			return SandboxResult{}, fmt.Errorf("start sandbox container: %w", startErr)
		}
		return SandboxResult{}, inspectErr
	}
	return SandboxResult{ExitCode: exitCode, Duration: duration,
		Stdout: sanitizeOutput(stdout.String()), Stderr: sanitizeOutput(stderr.String()),
		OutputTruncated: stdout.Truncated() || stderr.Truncated()}, nil
}

func (r *DockerRunner) createArguments(containerName string, command []string) []string {
	memory := strconv.Itoa(r.config.MemoryMB) + "m"
	args := []string{"create", "--name", containerName, "--hostname=sandbox", "--pull=never", "--network=none",
		"--memory=" + memory, "--memory-swap=" + memory,
		"--cpus=" + strconv.FormatFloat(r.config.CPUs, 'f', -1, 64),
		"--pids-limit=" + strconv.Itoa(r.config.PIDsLimit), "--cap-drop=ALL",
		"--security-opt=no-new-privileges", "--read-only", "--user=65532:65532",
		"--init", "--stop-timeout=1", "--shm-size=64m", "--ulimit=nofile=1024:1024", "--ulimit=core=0",
		"--mount=type=volume,destination=/workspace",
		"--tmpfs=/tmp:rw,nosuid,nodev,exec,size=256m,mode=1777", "--workdir=/workspace",
		"--env=HOME=/tmp", "--env=GOCACHE=/tmp/go-build", "--env=GOPATH=/tmp/go",
		"--env=GOTOOLCHAIN=local", "--env=GOPROXY=off", "--env=GOSUMDB=off",
		"--env=CGO_ENABLED=0", r.config.Image}
	return append(args, command...)
}

func (r *DockerRunner) inspectExitCode(ctx context.Context, containerName string) (int, error) {
	var output, errorOutput bytes.Buffer
	if err := r.executor.Run(ctx, &output, &errorOutput, r.config.Binary, "inspect",
		"--format={{.State.ExitCode}}", containerName); err != nil {
		return 0, fmt.Errorf("inspect sandbox exit code: %w: %s", err,
			compactDockerError(errorOutput.String()))
	}
	exitCode, err := strconv.Atoi(strings.TrimSpace(output.String()))
	if err != nil {
		return 0, fmt.Errorf("invalid sandbox exit code %q", strings.TrimSpace(output.String()))
	}
	return exitCode, nil
}

func randomContainerName() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate sandbox container name: %w", err)
	}
	return "ai-test-validation-" + hex.EncodeToString(value), nil
}

func compactDockerError(value string) string {
	value = strings.TrimSpace(sanitizeOutput(value))
	if len(value) > 4096 {
		value = value[:4096] + "...[truncated]"
	}
	return value
}

var (
	assignmentSecret = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|auth[_-]?token|password|secret)\s*[:=]\s*([^\s,;]+)`)
	bearerSecret     = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
)

func sanitizeOutput(value string) string {
	value = assignmentSecret.ReplaceAllString(value, "$1=[REDACTED]")
	return bearerSecret.ReplaceAllString(value, "Bearer [REDACTED]")
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	remaining int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer { return &limitedBuffer{remaining: limit} }

func (b *limitedBuffer) Write(value []byte) (int, error) {
	originalLength := len(value)
	if len(value) > b.remaining {
		value = value[:b.remaining]
		b.truncated = true
	}
	if len(value) > 0 {
		_, _ = b.buffer.Write(value)
		b.remaining -= len(value)
	}
	if originalLength > len(value) {
		b.truncated = true
	}
	return originalLength, nil
}

func (b *limitedBuffer) String() string  { return b.buffer.String() }
func (b *limitedBuffer) Truncated() bool { return b.truncated }
