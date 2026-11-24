package server

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"

	"tcbroker/pkg/config"
	"tcbroker/pkg/manager"
	"tcbroker/pkg/state"
	"tcbroker/pkg/tc"
)

//go:embed web/*
var webAssets embed.FS

// InterfaceInfo describes a host network interface exposed to the WebUI.
type InterfaceInfo struct {
	Name  string `json:"name"`
	State string `json:"state"`
	MAC   string `json:"mac,omitempty"`
}

// Options configures the tcbroker HTTP server (WebUI + REST API + /metrics).
type Options struct {
	StatePath       string
	Version         string
	ReadOnly        bool
	Token           string
	Demo            bool
	Debug           bool
	Runner          *tc.Runner
	DemoExec        *DemoExecutor
	InterfaceLister func() []InterfaceInfo
}

// Server serves the embedded mini-NPB WebUI, JSON API, and Prometheus /metrics endpoint.
type Server struct {
	opts Options
	mgr  *manager.Manager
	mu   sync.Mutex // serializes mutating TC operations
}

// New creates a new Server instance.
func New(opts Options) *Server {
	runner := opts.Runner
	if runner == nil {
		runner = tc.NewRunner(opts.Debug, false)
	}
	if opts.InterfaceLister == nil {
		if opts.Demo {
			opts.InterfaceLister = DemoInterfaces
		} else {
			opts.InterfaceLister = listHostInterfaces
		}
	}
	return &Server{
		opts: opts,
		mgr:  manager.New(runner, opts.StatePath),
	}
}

// Handler returns the root http.Handler mounting the API, /metrics, and embedded WebUI.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Read-only endpoints
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/config.yaml", s.handleExportYAML)
	mux.HandleFunc("/metrics", s.handleMetrics)

	// Mutating endpoints (guarded by read-only & optional token auth)
	mux.HandleFunc("/api/v1/apply", s.guardWrite(s.handleApply))
	mux.HandleFunc("/api/v1/rules/", s.guardWrite(s.handleRuleAction))
	mux.HandleFunc("/api/v1/reconcile", s.guardWrite(s.handleReconcile))
	mux.HandleFunc("/api/v1/demo/drift", s.guardWrite(s.handleDemoDrift))

	// Embedded WebUI static files
	sub, err := fs.Sub(webAssets, "web")
	if err != nil {
		panic(fmt.Sprintf("failed to load embedded web assets: %v", err))
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("/", fileServer)

	return mux
}

type statusPayload struct {
	Version      string               `json:"version"`
	ReadOnly     bool                 `json:"read_only"`
	AuthRequired bool                 `json:"auth_required"`
	Demo         bool                 `json:"demo"`
	StatePath    string               `json:"state_path"`
	ConfigPath   string               `json:"config_path,omitempty"`
	HasDrift     bool                 `json:"has_drift"`
	Rules        []manager.RuleStatus `json:"rules"`
	Interfaces   []InterfaceInfo      `json:"interfaces"`
	ConfigYAML   string               `json:"config_yaml"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	report, err := s.mgr.Status("")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	st, _ := state.Load(s.opts.StatePath)
	yamlStr, _ := st.ExportYAML()

	writeJSON(w, http.StatusOK, statusPayload{
		Version:      s.opts.Version,
		ReadOnly:     s.opts.ReadOnly,
		AuthRequired: s.opts.Token != "",
		Demo:         s.opts.Demo,
		StatePath:    report.StatePath,
		ConfigPath:   report.ConfigPath,
		HasDrift:     report.HasDrift(),
		Rules:        report.Rules,
		Interfaces:   s.opts.InterfaceLister(),
		ConfigYAML:   yamlStr,
	})
}

func (s *Server) handleExportYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	st, err := state.Load(s.opts.StatePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	yamlStr, err := st.ExportYAML()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="tcbroker.yaml"`)
	_, _ = w.Write([]byte(yamlStr))
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	report, err := s.mgr.Status("")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	WritePrometheusMetrics(w, report)
}

type applyRequest struct {
	YAML   string `json:"yaml"`
	DryRun bool   `json:"dry_run"`
}

type applyResponse struct {
	DryRun        bool                  `json:"dry_run"`
	HasChanges    bool                  `json:"has_changes"`
	ToAdd         []state.ManagedFilter `json:"to_add"`
	ToDelete      []state.ManagedFilter `json:"to_delete"`
	KeptCount     int                   `json:"kept_count"`
	DriftedCount  int                   `json:"drifted_count"`
	Warnings      []string              `json:"warnings"`
	CreatedQdiscs []string              `json:"created_qdiscs,omitempty"`
	RemovedQdiscs []string              `json:"removed_qdiscs,omitempty"`
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req applyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON body: %v", err))
		return
	}

	cfg, err := config.Parse([]byte(req.YAML))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !req.DryRun && !s.opts.Demo {
		var missing []string
		for _, iface := range cfg.AllInterfaces() {
			if _, errIface := net.InterfaceByName(iface); errIface != nil {
				missing = append(missing, iface)
			}
		}
		if len(missing) > 0 {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("network interface(s) not found: %s", strings.Join(missing, ", ")))
			return
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if req.DryRun {
		plan, err := s.mgr.PlanApply(cfg, "webui")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, applyResponse{
			DryRun:       true,
			HasChanges:   plan.HasChanges(),
			ToAdd:        plan.ToAdd,
			ToDelete:     plan.ToDelete,
			KeptCount:    len(plan.Kept),
			DriftedCount: plan.DriftedCount,
			Warnings:     cfg.Warnings(),
		})
		return
	}

	res, err := s.mgr.Apply(cfg, "webui")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, applyResponse{
		DryRun:        false,
		HasChanges:    res.Plan.HasChanges(),
		ToAdd:         res.Plan.ToAdd,
		ToDelete:      res.Plan.ToDelete,
		KeptCount:     len(res.Plan.Kept),
		DriftedCount:  res.Plan.DriftedCount,
		Warnings:      cfg.Warnings(),
		CreatedQdiscs: res.CreatedQdiscs,
		RemovedQdiscs: res.RemovedQdiscs,
	})
}

func (s *Server) handleRuleAction(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/rules/")
	if suffix == "" {
		writeError(w, http.StatusBadRequest, "rule name is required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// POST /api/v1/rules/delete-all
	if suffix == "delete-all" && r.Method == http.MethodPost {
		res, err := s.mgr.Delete(nil, true)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}

	// DELETE /api/v1/rules/{name}
	if r.Method == http.MethodDelete {
		res, err := s.mgr.Delete([]string{suffix}, false)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}

	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) handleReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := state.Load(s.opts.StatePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg := st.ExportConfig()
	if len(cfg.Rules) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"reconciled": 0})
		return
	}
	res, err := s.mgr.Apply(cfg, st.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reconciled": len(res.Plan.ToAdd),
		"drifted":    res.Plan.DriftedCount,
	})
}

func (s *Server) handleDemoDrift(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.opts.Demo || s.opts.DemoExec == nil {
		writeError(w, http.StatusNotFound, "only available in --demo mode")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ok := s.opts.DemoExec.SimulateDrift()
	writeJSON(w, http.StatusOK, map[string]bool{"drifted": ok})
}

func (s *Server) guardWrite(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.opts.ReadOnly {
			writeError(w, http.StatusForbidden, "server is running in --read-only mode")
			return
		}
		if s.opts.Token != "" {
			provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if provided == r.Header.Get("Authorization") {
				provided = r.Header.Get("X-TCBroker-Token")
			}
			if subtle.ConstantTimeCompare([]byte(provided), []byte(s.opts.Token)) != 1 {
				writeError(w, http.StatusUnauthorized, "invalid or missing API token")
				return
			}
		}
		next(w, r)
	}
}

func listHostInterfaces() []InterfaceInfo {
	ifaces, err := net.Interfaces()
	if err != nil {
		return []InterfaceInfo{}
	}
	result := make([]InterfaceInfo, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		st := "DOWN"
		if iface.Flags&net.FlagUp != 0 {
			st = "UP"
		}
		result = append(result, InterfaceInfo{
			Name:  iface.Name,
			State: st,
			MAC:   iface.HardwareAddr.String(),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
