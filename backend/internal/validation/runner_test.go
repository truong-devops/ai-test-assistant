package validation

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type commandCall struct {
	name string
	args []string
}

type executorStub struct {
	mu          sync.Mutex
	calls       []commandCall
	exitCode    string
	startErr    error
	startOut    string
	startErrOut string
	blockStart  bool
}

func (s *executorStub) Run(ctx context.Context, stdout, stderr io.Writer,
	name string, args ...string,
) error {
	s.mu.Lock()
	s.calls = append(s.calls, commandCall{name: name, args: append([]string(nil), args...)})
	s.mu.Unlock()
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "create":
		_, _ = io.WriteString(stdout, "container-id\n")
	case "start":
		if s.blockStart {
			<-ctx.Done()
			return ctx.Err()
		}
		_, _ = io.WriteString(stdout, s.startOut)
		_, _ = io.WriteString(stderr, s.startErrOut)
		return s.startErr
	case "inspect":
		value := s.exitCode
		if value == "" {
			value = "0"
		}
		_, _ = io.WriteString(stdout, value+"\n")
	}
	return nil
}

func TestDockerRunnerUsesHardenedResourceLimitedContainer(t *testing.T) {
	executor := &executorStub{exitCode: "1", startErr: errors.New("test command failed"),
		startOut: "password=hunter2\n", startErrOut: "assertion failed\n"}
	runner, err := newDockerRunner(DockerConfig{Image: "sandbox:test", Timeout: time.Second,
		MemoryMB: 512, CPUs: 1.5, PIDsLimit: 64, MaxOutputBytes: 1024}, executor)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	result, err := runner.Run(context.Background(), SandboxRequest{
		Workspace: workspace, Command: []string{"go", "test", "./..."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 1 || result.TimedOut || strings.Contains(result.Stdout, "hunter2") ||
		!strings.Contains(result.Stdout, "[REDACTED]") {
		t.Fatalf("Run()=%#v", result)
	}
	create := executor.calls[0]
	joined := strings.Join(create.args, " ")
	for _, required := range []string{"--pull=never", "--network=none", "--memory=512m",
		"--memory-swap=512m", "--cpus=1.5", "--pids-limit=64", "--cap-drop=ALL",
		"--security-opt=no-new-privileges", "--read-only", "--user=65532:65532",
		"--mount=type=volume,destination=/workspace", "--tmpfs=/tmp:", "GOPROXY=off"} {
		if !strings.Contains(joined, required) {
			t.Errorf("docker create args missing %q: %s", required, joined)
		}
	}
	if strings.Contains(joined, "docker.sock") || strings.Contains(joined, workspace) {
		t.Fatalf("docker create unexpectedly mounts host data: %s", joined)
	}
	last := executor.calls[len(executor.calls)-1]
	if len(last.args) == 0 || last.args[0] != "rm" {
		t.Fatalf("last Docker call=%#v, want cleanup", last)
	}
}

func TestDockerRunnerTimesOutTruncatesOutputAndCleansUp(t *testing.T) {
	executor := &executorStub{blockStart: true}
	runner, err := newDockerRunner(DockerConfig{Image: "sandbox:test", Timeout: 20 * time.Millisecond,
		MemoryMB: 64, CPUs: .5, PIDsLimit: 16, MaxOutputBytes: 8}, executor)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), SandboxRequest{
		Workspace: t.TempDir(), Command: []string{"go", "test", "./..."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.TimedOut || result.ExitCode != -1 {
		t.Fatalf("Run()=%#v, want timeout", result)
	}
	last := executor.calls[len(executor.calls)-1]
	if len(last.args) == 0 || last.args[0] != "rm" {
		t.Fatalf("last Docker call=%#v, want cleanup", last)
	}

	buffer := newLimitedBuffer(4)
	if n, err := buffer.Write([]byte("123456")); err != nil || n != 6 ||
		buffer.String() != "1234" || !buffer.Truncated() {
		t.Fatalf("limited buffer n=%d value=%q truncated=%v error=%v",
			n, buffer.String(), buffer.Truncated(), err)
	}
}

func TestDockerRunnerRejectsUnsafeConfigurationAndCommand(t *testing.T) {
	if _, err := newDockerRunner(DockerConfig{Image: "--privileged", Timeout: time.Second,
		MemoryMB: 512, CPUs: 1, PIDsLimit: 64}, &executorStub{}); err == nil {
		t.Fatal("newDockerRunner() error=nil, want unsafe image rejection")
	}
	runner, err := newDockerRunner(DockerConfig{Image: "sandbox:test", Timeout: time.Second,
		MemoryMB: 512, CPUs: 1, PIDsLimit: 64}, &executorStub{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(context.Background(), SandboxRequest{Workspace: t.TempDir(),
		Command: []string{"go", ""}}); err == nil {
		t.Fatal("Run() error=nil, want empty command argument rejection")
	}
}
