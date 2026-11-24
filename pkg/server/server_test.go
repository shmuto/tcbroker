package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"tcbroker/pkg/manager"
	"tcbroker/pkg/tc"
)

func newTestServer(t *testing.T, readOnly bool, token string) (*Server, *DemoExecutor) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.json")
	demoExec := NewDemoExecutor()
	runner := tc.NewRunnerWithExecutor(demoExec, false, false, &bytes.Buffer{})

	mgr := manager.New(runner, statePath)
	if _, err := mgr.Apply(DemoInitialConfig(), "demo.yaml"); err != nil {
		t.Fatalf("failed to seed demo config: %v", err)
	}

	srv := New(Options{
		StatePath: statePath,
		Version:   "2.0.0",
		ReadOnly:  readOnly,
		Token:     token,
		Demo:      true,
		Runner:    runner,
		DemoExec:  demoExec,
	})
	return srv, demoExec
}

func TestServer_EmbeddedUIAndStatusAndMetrics(t *testing.T) {
	srv, _ := newTestServer(t, false, "")
	h := srv.Handler()

	// 1. Embedded HTML
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "tcbroker") {
		t.Fatalf("GET / failed: code=%d", rec.Code)
	}

	// 2. Status JSON
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/status failed: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var status statusPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("invalid status JSON: %v", err)
	}
	if len(status.Rules) != 3 {
		t.Fatalf("expected 3 demo rules, got %d", len(status.Rules))
	}
	if !strings.Contains(status.ConfigYAML, "wan-web-inspect") {
		t.Errorf("expected reconstructed config_yaml to contain wan-web-inspect, got:\n%s", status.ConfigYAML)
	}

	// 3. Prometheus /metrics
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics failed: code=%d", rec.Code)
	}
	metrics := rec.Body.String()
	for _, want := range []string{
		"tcbroker_active_rules_total 3",
		`tcbroker_rule_health{rule="wan-web-inspect",src="eth0"} 1`,
		"tcbroker_filter_packets_total{",
	} {
		if !strings.Contains(metrics, want) {
			t.Errorf("metrics missing %q:\n%s", want, metrics)
		}
	}

	// 4. YAML export
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/config.yaml", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "dns-shadow-replay") {
		t.Fatalf("GET /api/v1/config.yaml failed: code=%d", rec.Code)
	}
}

func TestServer_ReadOnlyAndTokenGuards(t *testing.T) {
	// Read-only rejects writes with 403
	roSrv, _ := newTestServer(t, true, "")
	rec := httptest.NewRecorder()
	roSrv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/rules/wan-web-inspect", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 in read-only mode, got %d", rec.Code)
	}

	// Token auth rejects missing/wrong token with 401 and accepts valid Bearer token
	authSrv, _ := newTestServer(t, false, "secret-token")
	rec = httptest.NewRecorder()
	authSrv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/rules/wan-web-inspect", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/rules/wan-web-inspect", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	authSrv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid Bearer token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestServer_ApplyDriftAndReconcile(t *testing.T) {
	srv, _ := newTestServer(t, false, "")
	h := srv.Handler()

	// Dry-run apply with a new rule
	payload := map[string]any{
		"yaml": `rules:
  - name: new-ssh-tap
    src: eth0
    dst: tap-ids0
    filters:
      - proto: tcp
        dst_port: 22
`,
		"dry_run": true,
	}
	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/apply", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("dry-run apply failed: %d %s", rec.Code, rec.Body.String())
	}
	var dryRes applyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &dryRes); err != nil {
		t.Fatalf("invalid dry-run json: %v", err)
	}
	if !dryRes.DryRun || len(dryRes.ToAdd) != 1 || len(dryRes.ToDelete) == 0 {
		t.Fatalf("unexpected dry-run diff: %+v", dryRes)
	}

	// Simulate drift then reconcile
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/demo/drift", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("demo drift failed: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	var st statusPayload
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if !st.HasDrift {
		t.Fatal("expected has_drift=true after simulated drift")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/reconcile", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("reconcile failed: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.HasDrift {
		t.Fatal("expected has_drift=false after reconcile")
	}
}
