package cc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ethpandaops/xcli/pkg/ai"
	"github.com/ethpandaops/xcli/pkg/diagnostic"
)

const diagnoseTurnTimeout = 30 * time.Minute

type diagnoseSession struct {
	id          string
	service     string
	provider    ai.ProviderID
	session     ai.Session
	runMu       sync.Mutex
	stateMu     sync.Mutex
	interrupted bool
}

func (s *diagnoseSession) setInterrupted(v bool) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.interrupted = v
}

func (s *diagnoseSession) wasInterrupted() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	return s.interrupted
}

type diagnoseStartRequest struct {
	Provider  string `json:"provider"`
	RequestID string `json:"requestId"`
}

type diagnoseMessageRequest struct {
	SessionID string `json:"sessionId"`
	Provider  string `json:"provider"`
	Prompt    string `json:"prompt"`
	RequestID string `json:"requestId"`
}

type diagnoseInterruptRequest struct {
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
}

func (a *apiHandler) handleGetAIProviders(w http.ResponseWriter, r *http.Request) {
	providers := ai.ListProviderInfo(r.Context(), a.log, a.aiDefaultProvider)
	writeJSON(w, http.StatusOK, providers)
}

// handleGetDiagnoseAvailable returns whether the selected provider is available.
func (a *apiHandler) handleGetDiagnoseAvailable(w http.ResponseWriter, r *http.Request) {
	provider := a.providerFromString(r.URL.Query().Get("provider"))

	engine, err := ai.NewEngine(provider, a.log)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "provider": provider})

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"available": engine.IsAvailable(),
		"provider":  provider,
	})
}

// handlePostDiagnose is a synchronous compatibility endpoint.
func (a *apiHandler) handlePostDiagnose(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service name required"})

		return
	}

	provider := a.providerFromString(r.URL.Query().Get("provider"))

	engine, err := ai.NewEngine(provider, a.log)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})

		return
	}

	if !engine.IsAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("provider %s is not available", provider)})

		return
	}

	status, health, ok := a.getServiceStatus(name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found: " + name})

		return
	}

	logLines := a.collectServiceLogs(name)
	if len(logLines) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "no logs available for service: " + name})

		return
	}

	prompt := buildDiagnosePrompt(name, status, health, logLines)

	response, err := engine.Ask(r.Context(), prompt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("diagnosis failed: %v", err)})

		return
	}

	diagnosis := diagnostic.ParseDiagnosisResponse(response)
	writeJSON(w, http.StatusOK, diagnosis)
}

func (a *apiHandler) handlePostDiagnoseStart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service name required"})

		return
	}

	var req diagnoseStartRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})

		return
	}

	provider := a.providerFromString(req.Provider)

	engine, err := ai.NewEngine(provider, a.log)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})

		return
	}

	if !engine.IsAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("provider %s is not available", provider)})

		return
	}

	status, health, ok := a.getServiceStatus(name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found: " + name})

		return
	}

	logLines := a.collectServiceLogs(name)
	if len(logLines) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "no logs available for service: " + name})

		return
	}

	// Do not bind long-lived session startup to the HTTP request context,
	// which is cancelled as soon as this handler returns.
	sessionCtx, cancelSessionStart := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelSessionStart()

	session, err := engine.StartSession(sessionCtx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to start provider session: %v", err)})

		return
	}

	s := &diagnoseSession{
		id:       session.ID(),
		service:  name,
		provider: provider,
		session:  session,
	}
	a.storeDiagnoseSession(s)

	requestID := normalizeRequestID(req.RequestID)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"sessionId": s.id,
		"requestId": requestID,
		"provider":  string(provider),
	})

	prompt := buildDiagnosePrompt(name, status, health, logLines)
	go a.runDiagnoseTurn(s, requestID, prompt)
}

func (a *apiHandler) handlePostDiagnoseMessage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service name required"})

		return
	}

	var req diagnoseMessageRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})

		return
	}

	if strings.TrimSpace(req.SessionID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionId is required"})

		return
	}

	if strings.TrimSpace(req.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})

		return
	}

	s, ok := a.getDiagnoseSession(req.SessionID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "diagnose session not found"})

		return
	}

	if s.service != name {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session/service mismatch"})

		return
	}

	requestID := normalizeRequestID(req.RequestID)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"sessionId": s.id,
		"requestId": requestID,
		"provider":  string(s.provider),
	})

	go a.runDiagnoseTurn(s, requestID, buildFollowUpPrompt(req.Prompt))
}

// buildFollowUpPrompt wraps a user's follow-up question with formatting
// instructions and an optional structured format for genuine diagnoses.
func buildFollowUpPrompt(userPrompt string) string {
	var sb strings.Builder

	sb.WriteString("Respond using markdown formatting (headers, lists, code blocks, bold, etc). ")
	sb.WriteString("Be specific and actionable.\n\n")
	sb.WriteString("If your answer identifies a specific root cause, use the structured format below. ")
	sb.WriteString("Otherwise respond freely in markdown.\n\n")
	sb.WriteString("Structured format (only when a root cause is identified):\n")
	sb.WriteString("1. **## Root Cause** - A single sentence identifying the primary issue\n")
	sb.WriteString("2. **## Explanation** - 2-3 sentences explaining why this error occurred\n")
	sb.WriteString("3. **## Affected Files** - List of files that likely need changes (one per line, prefixed with -)\n")
	sb.WriteString("4. **## Suggestions** - Numbered list of specific actions to fix the issue\n")
	sb.WriteString("5. **## Fix Commands** - Shell commands that might help (prefixed with $)\n\n")
	sb.WriteString(userPrompt)

	return sb.String()
}

func (a *apiHandler) handlePostDiagnoseInterrupt(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service name required"})

		return
	}

	var req diagnoseInterruptRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})

		return
	}

	if strings.TrimSpace(req.SessionID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionId is required"})

		return
	}

	s, ok := a.getDiagnoseSession(req.SessionID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "diagnose session not found"})

		return
	}

	if s.service != name {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session/service mismatch"})

		return
	}

	s.setInterrupted(true)

	if err := s.session.Interrupt(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("interrupt failed: %v", err)})

		return
	}

	requestID := normalizeRequestID(req.RequestID)
	a.broadcastDiagnoseEvent("diagnose_interrupted", map[string]any{
		"sessionId": s.id,
		"requestId": requestID,
		"service":   s.service,
		"provider":  s.provider,
	})

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":    "interrupted",
		"sessionId": s.id,
		"requestId": requestID,
	})
}

func (a *apiHandler) handleDeleteDiagnoseSession(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sessionID := r.PathValue("session")

	if name == "" || sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service and session are required"})

		return
	}

	s, ok := a.deleteDiagnoseSession(sessionID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "diagnose session not found"})

		return
	}

	if s.service != name {
		_ = s.session.Close()

		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session/service mismatch"})

		return
	}

	if err := s.session.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to close session: %v", err)})

		return
	}

	a.broadcastDiagnoseEvent("diagnose_session_closed", map[string]any{
		"sessionId": s.id,
		"service":   s.service,
		"provider":  s.provider,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "closed", "sessionId": s.id})
}

func (a *apiHandler) runDiagnoseTurn(s *diagnoseSession, requestID, prompt string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	s.setInterrupted(false)
	a.broadcastDiagnoseEvent("diagnose_started", map[string]any{
		"sessionId": s.id,
		"requestId": requestID,
		"service":   s.service,
		"provider":  s.provider,
		"ts":        time.Now().UTC(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), diagnoseTurnTimeout)
	defer cancel()

	response, err := s.session.AskStream(ctx, prompt, func(chunk ai.StreamChunk) {
		a.broadcastDiagnoseEvent("diagnose_stream", map[string]any{
			"sessionId": s.id,
			"requestId": requestID,
			"service":   s.service,
			"provider":  s.provider,
			"kind":      chunk.Kind,
			"text":      chunk.Text,
			"eventType": chunk.EventType,
			"seq":       chunk.Seq,
		})
	})
	if err != nil {
		if s.wasInterrupted() || errors.Is(err, context.Canceled) {
			return
		}

		debugInfo := map[string]any{}

		var providerDebugErr *ai.ProviderDebugError
		if errors.As(err, &providerDebugErr) {
			debugInfo = providerDebugErr.DebugInfo()
		}

		a.log.WithError(err).WithFields(map[string]any{
			"service":    s.service,
			"session_id": s.id,
			"provider":   s.provider,
			"debug":      debugInfo,
		}).Warn("diagnose stream failed")

		a.broadcastDiagnoseEvent("diagnose_error", map[string]any{
			"sessionId": s.id,
			"requestId": requestID,
			"service":   s.service,
			"provider":  s.provider,
			"error":     err.Error(),
		})

		return
	}

	diagnosis := diagnostic.ParseDiagnosisResponse(response)
	a.broadcastDiagnoseEvent("diagnose_result", map[string]any{
		"sessionId": s.id,
		"requestId": requestID,
		"service":   s.service,
		"provider":  s.provider,
		"rawText":   response,
		"diagnosis": diagnosis,
	})
}

func (a *apiHandler) providerFromString(provider string) ai.ProviderID {
	p := strings.TrimSpace(provider)
	if p == "" {
		return a.aiDefaultProvider
	}

	return ai.ProviderID(strings.ToLower(p))
}

func (a *apiHandler) getServiceStatus(name string) (status, health string, ok bool) {
	for _, svc := range a.getServicesData() {
		if svc.Name == name {
			return svc.Status, svc.Health, true
		}
	}

	return "", "", false
}

func (a *apiHandler) broadcastDiagnoseEvent(name string, data any) {
	if a.sseHub == nil {
		return
	}

	a.sseHub.Broadcast(name, data)
}

func (a *apiHandler) storeDiagnoseSession(s *diagnoseSession) {
	a.diagnoseMu.Lock()
	defer a.diagnoseMu.Unlock()

	a.diagnoseSessions[s.id] = s
}

func (a *apiHandler) getDiagnoseSession(sessionID string) (*diagnoseSession, bool) {
	a.diagnoseMu.Lock()
	defer a.diagnoseMu.Unlock()

	s, ok := a.diagnoseSessions[sessionID]

	return s, ok
}

func (a *apiHandler) deleteDiagnoseSession(sessionID string) (*diagnoseSession, bool) {
	a.diagnoseMu.Lock()
	defer a.diagnoseMu.Unlock()

	s, ok := a.diagnoseSessions[sessionID]
	if !ok {
		return nil, false
	}

	delete(a.diagnoseSessions, sessionID)

	return s, true
}

func (a *apiHandler) closeDiagnoseSessions() {
	a.diagnoseMu.Lock()

	sessions := make([]*diagnoseSession, 0, len(a.diagnoseSessions))
	for id, s := range a.diagnoseSessions {
		sessions = append(sessions, s)

		delete(a.diagnoseSessions, id)
	}

	a.diagnoseMu.Unlock()

	for _, s := range sessions {
		_ = s.session.Close()
	}
}

func decodeJSONBody(r *http.Request, target any) error {
	if r.Body == nil {
		return nil
	}

	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}

		return fmt.Errorf("invalid JSON body: %w", err)
	}

	return nil
}

func normalizeRequestID(requestID string) string {
	req := strings.TrimSpace(requestID)
	if req != "" {
		return req
	}

	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}

// collectServiceLogs gathers recent log lines for a service from disk
// (last ~300 lines) with a fallback to the in-memory ring buffer.
func (a *apiHandler) collectServiceLogs(name string) []string {
	logPath := filepath.Clean(a.orch.LogFilePath(name))

	lines := readLastLines(logPath, 300)
	if len(lines) > 0 {
		return lines
	}

	if a.logHistoryFn != nil {
		return a.logHistoryFn(name)
	}

	return nil
}

// buildDiagnosePrompt creates the log-focused prompt for AI analysis.
func buildDiagnosePrompt(name, status, health string, logLines []string) string {
	var sb strings.Builder

	sb.WriteString("Analyze these service logs and provide a diagnosis.\n\n")
	sb.WriteString("## Instructions\n")
	sb.WriteString("You are analyzing logs from a local Ethereum data pipeline service. ")
	sb.WriteString("Provide a structured analysis with the following sections:\n\n")
	sb.WriteString("1. **## Root Cause** - A single sentence identifying the primary issue\n")
	sb.WriteString("2. **## Explanation** - 2-3 sentences explaining why this error occurred\n")
	sb.WriteString("3. **## Affected Files** - List of files that likely need changes (one per line, prefixed with -)\n")
	sb.WriteString("4. **## Suggestions** - Numbered list of specific actions to fix the issue\n")
	sb.WriteString("5. **## Fix Commands** - Shell commands that might help (prefixed with $)\n\n")
	sb.WriteString("Be specific and actionable. Focus on the root cause, not symptoms.\n\n")

	sb.WriteString("## Service Info\n\n")
	sb.WriteString("- **Service**: ")
	sb.WriteString(promptMarkdownField(name))
	sb.WriteString("\n- **Status**: ")
	sb.WriteString(promptMarkdownField(status))
	sb.WriteString("\n- **Health**: ")
	sb.WriteString(promptMarkdownField(health))
	sb.WriteString("\n\n")

	sb.WriteString("## Recent Logs\n\n```\n")

	for _, line := range logLines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString("```\n")

	return sb.String()
}

func promptMarkdownField(value string) string {
	safe := strings.ReplaceAll(value, "\r", " ")
	safe = strings.ReplaceAll(safe, "\n", " ")

	return strings.TrimSpace(safe)
}

// readLastLines reads the last n lines from a file.
func readLastLines(path string, n int) []string {
	f, err := os.Open(path) //nolint:gosec // path is constructed by LogFilePath from internal config
	if err != nil {
		return nil
	}
	defer f.Close()

	var all []string

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		all = append(all, scanner.Text())
	}

	if len(all) <= n {
		return all
	}

	return all[len(all)-n:]
}
