package main

import (
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParseFlags_FlexibleOrdering(t *testing.T) {
	var dryRun bool
	var interval time.Duration
	fs := newFlagSet("test", func(io.Writer) {})
	fs.BoolVar(&dryRun, "dry-run", false, "")
	fs.DurationVar(&interval, "interval", time.Second, "")

	pos, err := parseFlags(fs, []string{"config.yaml", "--dry-run", "--state-file", "/tmp/s.json", "--interval", "2s"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if !dryRun {
		t.Errorf("expected dryRun=true")
	}
	if flagStateFile != "/tmp/s.json" {
		t.Errorf("expected flagStateFile=/tmp/s.json, got %q", flagStateFile)
	}
	if interval != 2*time.Second {
		t.Errorf("expected interval=2s, got %v", interval)
	}
	if !reflect.DeepEqual(pos, []string{"config.yaml"}) {
		t.Errorf("unexpected positional args: %v", pos)
	}
}

func TestParseFlags_BareDashAndDoubleDash(t *testing.T) {
	var force bool
	fs := newFlagSet("init", func(io.Writer) {})
	fs.BoolVar(&force, "force", false, "")
	fs.BoolVar(&force, "f", false, "")

	// Bare '-' must stay positional (stdout)
	pos, err := parseFlags(fs, []string{"-", "-f"})
	if err != nil {
		t.Fatalf("parseFlags failed: %v", err)
	}
	if !force || !reflect.DeepEqual(pos, []string{"-"}) {
		t.Errorf("expected force=true pos=[-], got force=%v pos=%v", force, pos)
	}

	// Arguments after '--' must not be parsed as flags
	fs2 := newFlagSet("delete", func(io.Writer) {})
	pos2, err := parseFlags(fs2, []string{"--", "--weird-rule-name"})
	if err != nil {
		t.Fatalf("parseFlags with -- failed: %v", err)
	}
	if !reflect.DeepEqual(pos2, []string{"--weird-rule-name"}) {
		t.Errorf("expected [--weird-rule-name], got %v", pos2)
	}
}

func TestRunCLI_AliasesAndHelp(t *testing.T) {
	// Version command
	if err := runCLI([]string{"version"}); err != nil {
		t.Fatalf("version failed: %v", err)
	}

	// Unknown command returns error
	if err := runCLI([]string{"nonexistent"}); err == nil {
		t.Fatal("expected error for unknown command")
	}

	// Help returns errHelpRequested
	if err := runCLI([]string{"apply", "--help"}); !errors.Is(err, errHelpRequested) {
		t.Fatalf("expected errHelpRequested, got %v", err)
	}

	// Init to temp file + validate via runCLI
	cfgPath := filepath.Join(t.TempDir(), "tcbroker.yaml")
	if err := runCLI([]string{"init", cfgPath}); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := runCLI([]string{"validate", cfgPath}); err != nil {
		t.Fatalf("validate failed: %v", err)
	}

	// Alias 'start' with --dry-run after positional arg
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := runCLI([]string{"start", cfgPath, "--dry-run", "--state-file", statePath}); err != nil {
		t.Fatalf("start alias --dry-run failed: %v", err)
	}

	// Alias 'stop' with empty state succeeds cleanly
	if err := runCLI([]string{"stop", "--dry-run", "--state-file", statePath}); err != nil {
		t.Fatalf("stop alias failed: %v", err)
	}
}
