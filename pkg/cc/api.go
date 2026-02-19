package cc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/git"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/tui"
	"github.com/sirupsen/logrus"
)

const gitCacheTTL = 2 * time.Minute

// gitCache holds the last successful git status response to avoid
// repeated expensive git operations on every poll.
type gitCache struct {
	resp      *gitResponse
	fetchedAt time.Time
	mu        sync.RWMutex
}

// stackProgressEvent is a single progress event emitted during boot/stop.
type stackProgressEvent struct {
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

// stackState tracks background stack operations to prevent concurrent boots/stops.
type stackState struct {
	status         string               // "idle", "starting", "stopping"
	lastError      string               // last boot/stop error, cleared on next operation
	cancelBoot     context.CancelFunc   // cancels the in-progress boot context; nil when not booting
	progressEvents []stackProgressEvent // accumulated progress events for the current operation
	mu             sync.Mutex
}

// apiHandler holds dependencies for REST API handlers.
type apiHandler struct {
	log      logrus.FieldLogger
	wrapper  *tui.OrchestratorWrapper
	health   *tui.HealthMonitor
	orch     *orchestrator.Orchestrator
	labCfg   *config.LabConfig
	cfgPath  string
	gitChk   *git.Checker
	sseHub   *SSEHub
	stack    stackState
	gitCache gitCache
	mu       sync.RWMutex
}

// statusResponse is the full dashboard snapshot.
type statusResponse struct {
	Services       []serviceResponse `json:"services"`
	Infrastructure []infraResponse   `json:"infrastructure"`
	Config         configResponse    `json:"config"`
	Timestamp      time.Time         `json:"timestamp"`
}

// serviceResponse represents a service with merged health info.
type serviceResponse struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	PID     int    `json:"pid"`
	Uptime  string `json:"uptime"`
	URL     string `json:"url"`
	Ports   []int  `json:"ports"`
	Health  string `json:"health"`
	LogFile string `json:"logFile"`
}

// infraResponse represents infrastructure status.
type infraResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

// configResponse is a sanitized view of the lab configuration.
type configResponse struct {
	Mode     string        `json:"mode"`
	Networks []networkInfo `json:"networks"`
	Ports    portsInfo     `json:"ports"`
	CfgPath  string        `json:"cfgPath"`
}

type networkInfo struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	PortOffset int    `json:"portOffset"`
}

type portsInfo struct {
	LabBackend      int `json:"labBackend"`
	LabFrontend     int `json:"labFrontend"`
	CBTBase         int `json:"cbtBase"`
	CBTAPIBase      int `json:"cbtApiBase"`
	CBTFrontendBase int `json:"cbtFrontendBase"`
	ClickHouseCBT   int `json:"clickhouseCbt"`
	ClickHouseXatu  int `json:"clickhouseXatu"`
	Redis           int `json:"redis"`
	Prometheus      int `json:"prometheus"`
	Grafana         int `json:"grafana"`
}

// gitResponse represents git status for all repos.
type gitResponse struct {
	Repos []repoInfo `json:"repos"`
}

type repoInfo struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	Branch           string `json:"branch"`
	AheadBy          int    `json:"aheadBy"`
	BehindBy         int    `json:"behindBy"`
	HasUncommitted   bool   `json:"hasUncommitted"`
	UncommittedCount int    `json:"uncommittedCount"`
	LatestTag        string `json:"latestTag"`
	CommitsSinceTag  int    `json:"commitsSinceTag"`
	IsUpToDate       bool   `json:"isUpToDate"`
	Error            string `json:"error,omitempty"`
}

// recreateOrchestrator rebuilds the orchestrator with the current config.
// This is needed after config changes (e.g. mode switch) so the orchestrator
// picks up the new mode, ports, etc. Must be called with a.mu held for writing.
func (a *apiHandler) recreateOrchestrator() error {
	newOrch, err := orchestrator.NewOrchestrator(a.log, a.labCfg, a.cfgPath)
	if err != nil {
		return fmt.Errorf("failed to recreate orchestrator: %w", err)
	}

	a.orch = newOrch
	a.wrapper.SetOrchestrator(newOrch)

	return nil
}

// handleGetStatus returns the full dashboard snapshot.
func (a *apiHandler) handleGetStatus(w http.ResponseWriter, _ *http.Request) {
	resp := statusResponse{
		Services:       a.getServicesData(),
		Infrastructure: a.getInfraData(),
		Config:         a.getConfigData(),
		Timestamp:      time.Now(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleGetServices returns all services with health info.
func (a *apiHandler) handleGetServices(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.getServicesData())
}

// handleGetInfrastructure returns infrastructure status.
func (a *apiHandler) handleGetInfrastructure(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.getInfraData())
}

// handleGetConfig returns sanitized configuration.
func (a *apiHandler) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.getConfigData())
}

// handleGetGit returns git status for all repos, caching successful
// responses for 2 minutes to avoid repeated expensive git operations.
func (a *apiHandler) handleGetGit(w http.ResponseWriter, r *http.Request) {
	a.gitCache.mu.RLock()

	if a.gitCache.resp != nil && time.Since(a.gitCache.fetchedAt) < gitCacheTTL {
		cached := *a.gitCache.resp
		a.gitCache.mu.RUnlock()

		writeJSON(w, http.StatusOK, cached)

		return
	}

	a.gitCache.mu.RUnlock()

	repos := map[string]string{
		"cbt":         a.labCfg.Repos.CBT,
		"xatu-cbt":    a.labCfg.Repos.XatuCBT,
		"cbt-api":     a.labCfg.Repos.CBTAPI,
		"lab-backend": a.labCfg.Repos.LabBackend,
		"lab":         a.labCfg.Repos.Lab,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	statuses := a.gitChk.CheckRepositories(ctx, repos)

	resp := gitResponse{
		Repos: make([]repoInfo, 0, len(statuses)),
	}

	for _, s := range statuses {
		info := repoInfo{
			Name:             s.Name,
			Path:             s.Path,
			Branch:           s.CurrentBranch,
			AheadBy:          s.AheadBy,
			BehindBy:         s.BehindBy,
			HasUncommitted:   s.HasUncommitted,
			UncommittedCount: s.UncommittedCount,
			LatestTag:        s.LatestTag,
			CommitsSinceTag:  s.CommitsSinceTag,
			IsUpToDate:       s.IsUpToDate,
		}

		if s.Error != nil {
			info.Error = s.Error.Error()
		}

		resp.Repos = append(resp.Repos, info)
	}

	slices.SortFunc(resp.Repos, func(a, b repoInfo) int {
		return strings.Compare(a.Name, b.Name)
	})

	// Cache successful responses (no repos had errors).
	hasErrors := false

	for _, ri := range resp.Repos {
		if ri.Error != "" {
			hasErrors = true

			break
		}
	}

	if !hasErrors {
		a.gitCache.mu.Lock()
		a.gitCache.resp = &resp
		a.gitCache.fetchedAt = time.Now()
		a.gitCache.mu.Unlock()
	}

	writeJSON(w, http.StatusOK, resp)
}

// handlePostServiceAction handles start/stop/restart/rebuild actions.
func (a *apiHandler) handlePostServiceAction(
	w http.ResponseWriter,
	r *http.Request,
	action string,
) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "service name required",
		})

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	var err error

	switch action {
	case "start":
		err = a.wrapper.StartService(ctx, name)
	case "stop":
		err = a.wrapper.StopService(ctx, name)
	case "restart":
		err = a.wrapper.RestartService(ctx, name)
	case "rebuild":
		err = a.wrapper.RebuildService(ctx, name)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "unknown action: " + action,
		})

		return
	}

	if err != nil {
		a.log.WithError(err).WithFields(logrus.Fields{
			"service": name,
			"action":  action,
		}).Error("Service action failed")

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})

		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// handleGetServiceLogs reads the log file for a service directly from disk
// and returns its parsed lines. Used by the frontend to show crash logs for
// stopped services without relying on the SSE/ring-buffer pipeline.
func (a *apiHandler) handleGetServiceLogs(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "service name required",
		})

		return
	}

	logPath := filepath.Clean(a.orch.LogFilePath(name))

	f, err := os.Open(logPath) //nolint:gosec // path is constructed by LogFilePath from internal config
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "log file not found",
		})

		return
	}
	defer f.Close()

	var lines []tui.LogLine

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		lines = append(lines, tui.ParseLine(name, scanner.Text()))
	}

	if lines == nil {
		lines = make([]tui.LogLine, 0)
	}

	writeJSON(w, http.StatusOK, lines)
}

// getServicesData builds the services response slice.
func (a *apiHandler) getServicesData() []serviceResponse {
	services := a.wrapper.GetServices()
	result := make([]serviceResponse, 0, len(services))

	for _, svc := range services {
		resp := serviceResponse{
			Name:    svc.Name,
			Status:  svc.Status,
			PID:     svc.PID,
			URL:     svc.URL,
			Ports:   svc.Ports,
			Health:  svc.Health,
			LogFile: svc.LogFile,
		}

		if svc.Uptime > 0 {
			resp.Uptime = formatDuration(svc.Uptime)
		}

		result = append(result, resp)
	}

	return result
}

// getInfraData builds the infrastructure response slice.
func (a *apiHandler) getInfraData() []infraResponse {
	infra := a.wrapper.GetInfrastructure()
	result := make([]infraResponse, 0, len(infra))

	for _, i := range infra {
		result = append(result, infraResponse{
			Name:   i.Name,
			Status: i.Status,
			Type:   i.Type,
		})
	}

	return result
}

// getConfigData builds the sanitized config response.
func (a *apiHandler) getConfigData() configResponse {
	networks := make([]networkInfo, 0, len(a.labCfg.Networks))
	for _, n := range a.labCfg.Networks {
		networks = append(networks, networkInfo{
			Name:       n.Name,
			Enabled:    n.Enabled,
			PortOffset: n.PortOffset,
		})
	}

	return configResponse{
		Mode:     a.labCfg.Mode,
		Networks: networks,
		Ports: portsInfo{
			LabBackend:      a.labCfg.Ports.LabBackend,
			LabFrontend:     a.labCfg.Ports.LabFrontend,
			CBTBase:         a.labCfg.Ports.CBTBase,
			CBTAPIBase:      a.labCfg.Ports.CBTAPIBase,
			CBTFrontendBase: a.labCfg.Ports.CBTFrontendBase,
			ClickHouseCBT:   a.labCfg.Infrastructure.ClickHouseCBTPort,
			ClickHouseXatu:  a.labCfg.Infrastructure.ClickHouseXatuPort,
			Redis:           a.labCfg.Infrastructure.RedisPort,
			Prometheus:      a.labCfg.Infrastructure.Observability.PrometheusPort,
			Grafana:         a.labCfg.Infrastructure.Observability.GrafanaPort,
		},
		CfgPath: a.cfgPath,
	}
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	d = d.Truncate(time.Second)

	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}

	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}

	return fmt.Sprintf("%ds", s)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Already started writing, can't change status
		return
	}
}
