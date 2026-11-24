package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"tcbroker/pkg/manager"
	"tcbroker/pkg/server"
	"tcbroker/pkg/tc"
)

const envAPIToken = "TCBROKER_API_TOKEN"

func printServeUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  tcbroker serve [flags]
  tcbroker ui [flags]

Starts the embedded mini-NPB WebUI, JSON API, and Prometheus /metrics server.
By default it binds to 127.0.0.1:8080 for safe local or SSH-forwarded access.

Flags:
      --listen string       Address to bind HTTP server (default "127.0.0.1:8080")
      --read-only           Disable mutating API endpoints (can run without root)
      --token string        Bearer token required for mutating API endpoints (or $TCBROKER_API_TOKEN)
      --demo                Run with simulated live TC traffic (no root or real NICs required)
      --state-file string   Override state file path
      --debug               Print all tc commands executed
  -h, --help                Show help
`)
}

func cmdServe(_ string, args []string) error {
	var (
		listenAddr string
		readOnly   bool
		token      string
		demo       bool
	)

	fs := newFlagSet("serve", printServeUsage)
	fs.StringVar(&listenAddr, "listen", "127.0.0.1:8080", "Address to bind HTTP server")
	fs.BoolVar(&readOnly, "read-only", false, "Disable mutating API endpoints")
	fs.StringVar(&token, "token", "", "Bearer token for mutating API endpoints")
	fs.BoolVar(&demo, "demo", false, "Run with simulated live TC traffic")

	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return fmt.Errorf("serve takes no positional arguments (run `tcbroker serve --help` for usage)")
	}

	if token == "" {
		token = os.Getenv(envAPIToken)
	}

	if !demo && !readOnly && os.Geteuid() != 0 {
		return fmt.Errorf("mutating WebUI mode requires root privileges (use sudo, --read-only for viewing, or --demo to try without root)")
	}

	statePath := resolvedStatePath()
	var runner *tc.Runner
	var demoExec *server.DemoExecutor

	if demo {
		if flagStateFile == "" && os.Getenv("TCBROKER_STATE_FILE") == "" {
			tmpDir, tmpErr := os.MkdirTemp("", "tcbroker-demo-*")
			if tmpErr != nil {
				return fmt.Errorf("failed to create demo temp dir: %w", tmpErr)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()
			statePath = filepath.Join(tmpDir, "state.json")
		}
		demoExec = server.NewDemoExecutor()
		runner = tc.NewRunnerWithExecutor(demoExec, flagDebug, false, os.Stdout)

		// Pre-populate realistic mini-NPB rules
		mgr := manager.New(runner, statePath)
		if _, err := mgr.Apply(server.DemoInitialConfig(), "demo.yaml"); err != nil {
			return fmt.Errorf("failed to initialize demo rules: %w", err)
		}
	} else {
		runner = tc.NewRunner(flagDebug, false)
	}

	if !readOnly && token == "" && !isLoopbackListen(listenAddr) {
		fmt.Fprintf(os.Stderr, "Warning: binding write-enabled server to non-loopback address %q without --token; consider using --token or --read-only\n", listenAddr)
	}

	srv := server.New(server.Options{
		StatePath: statePath,
		Version:   version,
		ReadOnly:  readOnly,
		Token:     token,
		Demo:      demo,
		Debug:     flagDebug,
		Runner:    runner,
		DemoExec:  demoExec,
	})

	httpSrv := &http.Server{
		Addr:              listenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}

	mode := "live"
	if demo {
		mode = "demo"
	} else if readOnly {
		mode = "read-only"
	}
	fmt.Printf("tcbroker mini-NPB server (%s) listening on http://%s\n", mode, ln.Addr().String())
	fmt.Printf("  WebUI:   http://%s/\n", ln.Addr().String())
	fmt.Printf("  Metrics: http://%s/metrics\n", ln.Addr().String())
	fmt.Printf("  State:   %s\n", statePath)

	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		fmt.Println("\nShutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(ctx)
	}
}

func isLoopbackListen(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	host = strings.TrimSpace(host)
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
