package api

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/SupperTomato/openclaw_dashboard/backend/internal/config"
	"github.com/SupperTomato/openclaw_dashboard/backend/internal/modules"
)

// Handlers contains all HTTP handlers
type Handlers struct {
	configMgr *config.ConfigManager
	modMgr    *modules.Manager
	tmplDir   string
	startTime time.Time
}

// NewHandlers creates new handlers
func NewHandlers(configMgr *config.ConfigManager, modMgr *modules.Manager, tmplDir string) *Handlers {
	return &Handlers{
		configMgr: configMgr,
		modMgr:    modMgr,
		tmplDir:   tmplDir,
		startTime: time.Now(),
	}
}

// DashboardPage serves the main dashboard HTML
func (h *Handlers) DashboardPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	cfg := h.configMgr.Get()
	mods := h.modMgr.GetStatus()

	data := map[string]any{
		"Title":       cfg.Dashboard.Title,
		"Theme":       cfg.Dashboard.Theme,
		"Refresh":     cfg.Dashboard.RefreshInterval,
		"Modules":     mods,
		"Uptime":      time.Since(h.startTime).Truncate(time.Second).String(),
		"CurrentPage": "dashboard",
	}

	h.renderTemplate(w, "dashboard.html", data)
}

// ConfigPage serves the config editor HTML
func (h *Handlers) ConfigPage(w http.ResponseWriter, r *http.Request) {
	cfg := h.configMgr.Get()
	raw, err := h.configMgr.GetRaw()
	if err != nil {
		http.Error(w, "Failed to read config", http.StatusInternalServerError)
		return
	}

	descriptions := config.GetDescriptions()

	// Group descriptions by category
	categories := make(map[string][]config.FieldDescription)
	var categoryOrder []string
	seen := make(map[string]bool)
	for _, d := range descriptions {
		if !seen[d.Category] {
			categoryOrder = append(categoryOrder, d.Category)
			seen[d.Category] = true
		}
		categories[d.Category] = append(categories[d.Category], d)
	}

	data := map[string]any{
		"Title":         cfg.Dashboard.Title,
		"Theme":         cfg.Dashboard.Theme,
		"ConfigRaw":     string(raw),
		"Descriptions":  descriptions,
		"Categories":    categories,
		"CategoryOrder": categoryOrder,
		"ReadOnly":      cfg.Security.ReadonlyMode,
		"CurrentPage":   "config",
	}

	h.renderTemplate(w, "config.html", data)
}

// APIStatus returns dashboard status
func (h *Handlers) APIStatus(w http.ResponseWriter, r *http.Request) {
	mods := h.modMgr.GetStatus()

	running := 0
	total := len(mods)
	for _, m := range mods {
		if m.State == modules.StateRunning {
			running++
		}
	}

	writeJSON(w, map[string]any{
		"status":          "ok",
		"uptime":          time.Since(h.startTime).Truncate(time.Second).String(),
		"modules_running": running,
		"modules_total":   total,
		"go_version":      runtime.Version(),
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
	})
}

// APIModules returns all module statuses
func (h *Handlers) APIModules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.modMgr.GetStatus())
}

// APIModuleAction handles module start/stop/restart
func (h *Handlers) APIModuleAction(w http.ResponseWriter, r *http.Request) {
	// Path: /api/modules/{id}/{action}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/modules/"), "/")
	if len(parts) < 1 {
		http.Error(w, "missing module id", http.StatusBadRequest)
		return
	}

	id := parts[0]

	// GET = status
	if r.Method == "GET" {
		status, err := h.modMgr.GetModuleStatus(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, status)
		return
	}

	// POST with action
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.configMgr.Get().Security.ReadonlyMode {
		http.Error(w, "dashboard is in read-only mode", http.StatusForbidden)
		return
	}

	action := ""
	if len(parts) >= 2 {
		action = parts[1]
	}

	var err error
	switch action {
	case "start":
		err = h.modMgr.StartModule(id)
	case "stop":
		err = h.modMgr.StopModule(id)
	case "restart":
		err = h.modMgr.RestartModule(id)
	default:
		http.Error(w, "unknown action: "+action, http.StatusBadRequest)
		return
	}

	if err != nil {
		writeJSON(w, map[string]any{"success": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"success": true, "action": action, "module": id})
}

// APIConfig handles config read/write
func (h *Handlers) APIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		raw, err := h.configMgr.GetRaw()
		if err != nil {
			http.Error(w, "failed to read config", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)

	case "PUT", "POST":
		if h.configMgr.Get().Security.ReadonlyMode {
			http.Error(w, "dashboard is in read-only mode", http.StatusForbidden)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		if err := h.configMgr.Save(json.RawMessage(body)); err != nil {
			writeJSON(w, map[string]any{"success": false, "error": err.Error()})
			return
		}

		writeJSON(w, map[string]any{"success": true, "message": "Configuration saved successfully"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// APIConfigDescriptions returns field descriptions
func (h *Handlers) APIConfigDescriptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, config.GetDescriptions())
}

// APISystem returns system information
func (h *Handlers) APISystem(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	writeJSON(w, map[string]any{
		"hostname":       hostname,
		"os":             runtime.GOOS,
		"arch":           runtime.GOARCH,
		"go_version":     runtime.Version(),
		"goroutines":     runtime.NumGoroutine(),
		"memory_alloc":   mem.Alloc,
		"memory_sys":     mem.Sys,
		"uptime":         time.Since(h.startTime).Truncate(time.Second).String(),
		"uptime_seconds": int(time.Since(h.startTime).Seconds()),
	})
}

// SSEHandler handles Server-Sent Events connections
func (h *Handlers) SSEHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	cfg := h.configMgr.Get()
	keepalive := time.Duration(cfg.Advanced.SSEKeepalive) * time.Second
	refresh := time.Duration(cfg.Dashboard.RefreshInterval) * time.Second

	ctx := r.Context()
	ticker := time.NewTicker(refresh)
	defer ticker.Stop()

	// Send initial data
	h.sendSSEModules(w, flusher)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.sendSSEModules(w, flusher)
		case <-time.After(keepalive):
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (h *Handlers) sendSSEModules(w http.ResponseWriter, flusher http.Flusher) {
	mods := h.modMgr.GetStatus()
	data, _ := json.Marshal(mods)
	fmt.Fprintf(w, "event: modules\ndata: %s\n\n", data)

	// System info
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	sysData, _ := json.Marshal(map[string]any{
		"goroutines": runtime.NumGoroutine(),
		"memory":     mem.Alloc,
		"uptime":     time.Since(h.startTime).Truncate(time.Second).String(),
	})
	fmt.Fprintf(w, "event: system\ndata: %s\n\n", sysData)

	flusher.Flush()
}

// ModuleProxy forwards requests to module processes
func (h *Handlers) ModuleProxy(w http.ResponseWriter, r *http.Request) {
	// Path: /module/{id}/...
	path := strings.TrimPrefix(r.URL.Path, "/module/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 1 {
		http.Error(w, "missing module id", http.StatusBadRequest)
		return
	}

	moduleID := parts[0]
	modulePath := "/"
	if len(parts) >= 2 {
		modulePath = "/" + parts[1]
	}

	h.modMgr.ProxyToModule(moduleID, modulePath, w, r)
}

// PartialModules returns just the module list HTML (for htmx)
func (h *Handlers) PartialModules(w http.ResponseWriter, r *http.Request) {
	mods := h.modMgr.GetStatus()
	h.renderTemplate(w, "partial_modules.html", map[string]any{
		"Modules": mods,
	})
}

// PartialSystem returns just the system info HTML (for htmx)
func (h *Handlers) PartialSystem(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	data := map[string]any{
		"Uptime":     time.Since(h.startTime).Truncate(time.Second).String(),
		"Goroutines": runtime.NumGoroutine(),
		"MemoryMB":   fmt.Sprintf("%.1f", float64(mem.Alloc)/1024/1024),
	}
	h.renderTemplate(w, "partial_system.html", data)
}

// PartialModuleContent returns module-specific content (for htmx)
func (h *Handlers) PartialModuleContent(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/partial/module/")
	status, err := h.modMgr.GetModuleStatus(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	h.renderTemplate(w, "partial_module_detail.html", map[string]any{
		"Module": status,
	})
}

func (h *Handlers) renderTemplate(w http.ResponseWriter, name string, data any) {
	tmplPath := filepath.Join(h.tmplDir, name)
	basePath := filepath.Join(h.tmplDir, "base.html")

	var tmpl *template.Template
	var err error

	// Check if base template exists
	if _, statErr := os.Stat(basePath); statErr == nil && name != "base.html" && !strings.HasPrefix(name, "partial_") {
		tmpl, err = template.ParseFiles(basePath, tmplPath)
	} else {
		tmpl, err = template.ParseFiles(tmplPath)
	}

	if err != nil {
		log.Printf("[api] Template error for %s: %v", name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[api] Template render error for %s: %v", name, err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
