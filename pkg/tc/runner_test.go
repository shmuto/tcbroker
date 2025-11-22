package tc

import (
	"strings"
	"testing"
)

func TestRunner_Run(t *testing.T) {
	// This is a basic integration test that assumes 'tc' is in the PATH.
	runner := NewRunner(false, false)

	// Execute a simple, read-only command.
	stdout, stderr, err := runner.Run("-V")

	if err != nil {
		t.Fatalf("Run() returned an unexpected error: %v, stderr: %s", err, stderr)
	}

	if stderr != "" {
		t.Errorf("Expected empty stderr, got: %s", stderr)
	}

	if !strings.Contains(stdout, "tc") {
		t.Errorf("Expected stdout to contain 'tc', got: %s", stdout)
	}
}
