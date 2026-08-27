//go:build sandbox

package validation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerSandboxPassingTest(t *testing.T) {
	result := runSandboxCase(t, `package sample
import "testing"
func TestAdd(t *testing.T) { if 1+1 != 2 { t.Fatal("bad math") } }
`, 60*time.Second)
	if result.TimedOut || result.ExitCode != 0 || !strings.Contains(result.Stdout, "ok") {
		t.Fatalf("result=%#v", result)
	}
}

func TestDockerSandboxCompilerFailure(t *testing.T) {
	result := runSandboxCase(t, `package sample
import "testing"
func TestBroken(t *testing.T) { missingSymbol() }
`, 60*time.Second)
	if result.TimedOut || result.ExitCode == 0 ||
		!strings.Contains(result.Stderr+result.Stdout, "undefined: missingSymbol") {
		t.Fatalf("result=%#v", result)
	}
}

func TestDockerSandboxAssertionFailure(t *testing.T) {
	result := runSandboxCase(t, `package sample
import "testing"
func TestAssertion(t *testing.T) { t.Fatal("deliberate assertion failure") }
`, 60*time.Second)
	if result.TimedOut || result.ExitCode == 0 ||
		!strings.Contains(result.Stderr+result.Stdout, "deliberate assertion failure") {
		t.Fatalf("result=%#v", result)
	}
}

func TestDockerSandboxTimeout(t *testing.T) {
	result := runSandboxCase(t, `package sample
import "testing"
func TestForever(t *testing.T) { select {} }
`, 300*time.Millisecond)
	if !result.TimedOut || result.ExitCode != -1 {
		t.Fatalf("result=%#v", result)
	}
}

func runSandboxCase(t *testing.T, testCode string, timeout time.Duration) SandboxResult {
	t.Helper()
	if os.Getenv("RUN_SANDBOX_TESTS") != "1" {
		t.Skip("RUN_SANDBOX_TESTS=1 is required")
	}
	image := os.Getenv("SANDBOX_TEST_IMAGE")
	if image == "" {
		image = "ai-test-assistant-sandbox:phase7"
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"),
		[]byte("module example.com/sandboxcase\n\ngo 1.25.0\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "sample_test.go"), []byte(testCode), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(workspace, 0o777); err != nil {
		t.Fatal(err)
	}
	runner, err := NewDockerRunner(DockerConfig{Image: image, Timeout: timeout,
		MemoryMB: 512, CPUs: 1, PIDsLimit: 64, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), SandboxRequest{Workspace: workspace,
		Command: []string{"go", "test", "-count=1", "-timeout=30s", "./..."}})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
