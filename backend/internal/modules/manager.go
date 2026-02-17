package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SupperTomato/openclaw_dashboard/backend/internal/config"
)

// ModuleState represents the current state of a module
type ModuleState string

const (
	StateRunning  ModuleState = "running"
	StateStopped  ModuleState = "stopped"
	StateCrashed  ModuleState = "crashed"
	StateStarting ModuleState = "starting"
	StateStopping ModuleState = "stopping"
)

// ModuleInfo holds metadata about a module
type ModuleInfo struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Icon        string      `json:"icon"`
	State       ModuleState `json:"state"`
	Port        int         `json:"port"`
	Enabled     bool        `json:"enabled"`
	PID         int         `json:"pid,omitempty"`
	Restarts    int         `json:"restarts"`
	LastStarted *time.Time  `json:"last_started,omitempty"`
	LastError   string      `json:"last_error,omitempty"`
	Uptime      string      `json:"uptime,omitempty"`
}

// ModuleDefinition defines a module that can be loaded
type ModuleDefinition struct {
	ID          string
	Name        string
	Description string
	Icon        string
	ConfigKey   string // key in modules config
}

// Manager handles the lifecycle of all modules
type Manager struct {
	mu          sync.RWMutex
	modules     map[string]*moduleInstance
	definitions []ModuleDefinition
	configMgr   *config.ConfigManager
	binDir      string
	ctx         context.Context
	cancel      context.CancelFunc
}

type moduleInstance struct {
	def        ModuleDefinition
	cmd        *exec.Cmd
	state      ModuleState
	port       int
	enabled    bool
	pid        int
	restarts   int
	maxRestart int
	backoff    int
	started    *time.Time
	lastError  string
	cancel     context.CancelFunc
}

// NewManager creates a new module manager
func NewManager(configMgr *config.ConfigManager, binDir string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		modules:   make(map[string]*moduleInstance),
		configMgr: configMgr,
		binDir:    binDir,
		ctx:       ctx,
		cancel:    cancel,
	}
	m.registerDefaults()
	return m
}

func (m *Manager) registerDefaults() {
	m.definitions = []ModuleDefinition{
		{ID: "system_health", Name: "System Health", Description: "Monitor CPU, RAM, disk, and temperature", Icon: "activity", ConfigKey: "system_health"},
		{ID: "session_manager", Name: "Session Manager", Description: "View and manage AI agent sessions", Icon: "users", ConfigKey: "session_manager"},
		{ID: "live_feed", Name: "Live Feed", Description: "Real-time stream of agent messages", Icon: "zap", ConfigKey: "live_feed"},
		{ID: "log_viewer", Name: "Log Viewer", Description: "View system and application logs", Icon: "file-text", ConfigKey: "log_viewer"},
		{ID: "file_manager", Name: "File Manager", Description: "Browse and edit workspace files", Icon: "folder", ConfigKey: "file_manager"},
		{ID: "cost_analyzer", Name: "Cost Analyzer", Description: "Track API spending and budgets", Icon: "dollar-sign", ConfigKey: "cost_analyzer"},
		{ID: "rate_limiter", Name: "Rate Limiter", Description: "Monitor API rate limit usage", Icon: "gauge", ConfigKey: "rate_limiter"},
		{ID: "memory_viewer", Name: "Memory Viewer", Description: "Browse agent memory files", Icon: "brain", ConfigKey: "memory_viewer"},
		{ID: "service_control", Name: "Service Control", Description: "Restart and manage system services", Icon: "settings", ConfigKey: "service_control"},
		{ID: "cron_manager", Name: "Cron Manager", Description: "View and manage scheduled jobs", Icon: "clock", ConfigKey: "cron_manager"},
	}
}

// StartAll starts all enabled modules
func (m *Manager) StartAll() {
	cfg := m.configMgr.Get()
	advanced := cfg.Advanced

	for _, def := range m.definitions {
		modCfg := m.getModuleConfig(cfg, def.ID)
		if modCfg == nil {
			continue
		}

		inst := &moduleInstance{
			def:        def,
			port:       modCfg.Port,
			enabled:    modCfg.Enabled,
			maxRestart: advanced.MaxModuleRestarts,
			backoff:    advanced.RestartBackoff,
		}

		m.mu.Lock()
		m.modules[def.ID] = inst
		m.mu.Unlock()

		if modCfg.Enabled {
			go m.startModule(def.ID)
		}
	}
}

// startModule starts a single module process
func (m *Manager) startModule(id string) {
	m.mu.Lock()
	inst, ok := m.modules[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	inst.state = StateStarting
	m.mu.Unlock()

	binName := strings.ReplaceAll(id, "_", "-")
	binPath := filepath.Join(m.binDir, "module-"+binName)
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		m.mu.Lock()
		inst.state = StateStopped
		inst.lastError = "binary not found: " + binPath
		m.mu.Unlock()
		log.Printf("[modules] Binary not found for %s: %s", id, binPath)
		return
	}

	ctx, cancel := context.WithCancel(m.ctx)

	m.mu.Lock()
	inst.cancel = cancel
	m.mu.Unlock()

	cfg := m.configMgr.Get()
	configJSON, _ := json.Marshal(m.getModuleConfig(cfg, id))

	cmd := exec.CommandContext(ctx, binPath)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("MODULE_ID=%s", id),
		fmt.Sprintf("MODULE_PORT=%d", inst.port),
		fmt.Sprintf("MODULE_CONFIG=%s", string(configJSON)),
		fmt.Sprintf("DASHBOARD_PORT=%d", cfg.Dashboard.Port),
		fmt.Sprintf("CONFIG_PATH=%s", m.configMgr.Path()),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		m.mu.Lock()
		inst.state = StateCrashed
		inst.lastError = err.Error()
		m.mu.Unlock()
		log.Printf("[modules] Failed to start %s: %v", id, err)
		m.handleCrash(id)
		return
	}

	now := time.Now()
	m.mu.Lock()
	inst.cmd = cmd
	inst.pid = cmd.Process.Pid
	inst.state = StateRunning
	inst.started = &now
	m.mu.Unlock()

	log.Printf("[modules] Started %s (PID: %d, Port: %d)", id, cmd.Process.Pid, inst.port)

	// Wait for process to exit
	err := cmd.Wait()
	if err != nil && ctx.Err() == nil {
		// Process crashed (not intentionally stopped)
		m.mu.Lock()
		inst.state = StateCrashed
		inst.lastError = err.Error()
		m.mu.Unlock()
		log.Printf("[modules] Module %s crashed: %v", id, err)
		m.handleCrash(id)
	} else {
		m.mu.Lock()
		inst.state = StateStopped
		inst.pid = 0
		m.mu.Unlock()
	}
}

func (m *Manager) handleCrash(id string) {
	m.mu.RLock()
	inst := m.modules[id]
	restarts := inst.restarts
	maxRestart := inst.maxRestart
	backoff := inst.backoff
	enabled := inst.enabled
	m.mu.RUnlock()

	if !enabled || restarts >= maxRestart {
		log.Printf("[modules] Module %s exceeded max restarts (%d), not restarting", id, maxRestart)
		return
	}

	delay := time.Duration(backoff*(1<<restarts)) * time.Second
	log.Printf("[modules] Restarting %s in %v (attempt %d/%d)", id, delay, restarts+1, maxRestart)

	select {
	case <-time.After(delay):
		m.mu.Lock()
		inst.restarts++
		m.mu.Unlock()
		go m.startModule(id)
	case <-m.ctx.Done():
		return
	}
}

// StopModule stops a running module
func (m *Manager) StopModule(id string) error {
	m.mu.Lock()
	inst, ok := m.modules[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("module %s not found", id)
	}
	if inst.state != StateRunning {
		m.mu.Unlock()
		return fmt.Errorf("module %s is not running (state: %s)", id, inst.state)
	}
	inst.state = StateStopping
	cancel := inst.cancel
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	// Wait a bit for graceful shutdown
	time.Sleep(2 * time.Second)

	m.mu.Lock()
	inst.state = StateStopped
	inst.pid = 0
	inst.enabled = false
	m.mu.Unlock()

	return nil
}

// StartModule starts a stopped module
func (m *Manager) StartModule(id string) error {
	m.mu.Lock()
	inst, ok := m.modules[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("module %s not found", id)
	}
	if inst.state == StateRunning {
		m.mu.Unlock()
		return fmt.Errorf("module %s is already running", id)
	}
	inst.enabled = true
	inst.restarts = 0
	m.mu.Unlock()

	go m.startModule(id)
	return nil
}

// RestartModule restarts a module
func (m *Manager) RestartModule(id string) error {
	m.mu.RLock()
	inst, ok := m.modules[id]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("module %s not found", id)
	}
	wasRunning := inst.state == StateRunning
	m.mu.RUnlock()

	if wasRunning {
		if err := m.StopModule(id); err != nil {
			log.Printf("[modules] Warning during restart stop: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	return m.StartModule(id)
}

// GetStatus returns status of all modules
func (m *Manager) GetStatus() []ModuleInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []ModuleInfo
	for _, def := range m.definitions {
		inst := m.modules[def.ID]
		info := ModuleInfo{
			ID:          def.ID,
			Name:        def.Name,
			Description: def.Description,
			Icon:        def.Icon,
		}
		if inst != nil {
			info.State = inst.state
			info.Port = inst.port
			info.Enabled = inst.enabled
			info.PID = inst.pid
			info.Restarts = inst.restarts
			info.LastStarted = inst.started
			info.LastError = inst.lastError
			if inst.started != nil && inst.state == StateRunning {
				info.Uptime = time.Since(*inst.started).Truncate(time.Second).String()
			}
		} else {
			info.State = StateStopped
		}
		result = append(result, info)
	}
	return result
}

// GetModuleStatus returns status of a single module
func (m *Manager) GetModuleStatus(id string) (*ModuleInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	inst, ok := m.modules[id]
	if !ok {
		return nil, fmt.Errorf("module %s not found", id)
	}

	var def ModuleDefinition
	for _, d := range m.definitions {
		if d.ID == id {
			def = d
			break
		}
	}

	info := &ModuleInfo{
		ID:          def.ID,
		Name:        def.Name,
		Description: def.Description,
		Icon:        def.Icon,
		State:       inst.state,
		Port:        inst.port,
		Enabled:     inst.enabled,
		PID:         inst.pid,
		Restarts:    inst.restarts,
		LastStarted: inst.started,
		LastError:   inst.lastError,
	}
	if inst.started != nil && inst.state == StateRunning {
		info.Uptime = time.Since(*inst.started).Truncate(time.Second).String()
	}
	return info, nil
}

// ProxyToModule proxies an HTTP request to a module
func (m *Manager) ProxyToModule(id string, path string, w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	inst, ok := m.modules[id]
	if !ok {
		m.mu.RUnlock()
		http.Error(w, "module not found", http.StatusNotFound)
		return
	}
	if inst.state != StateRunning {
		m.mu.RUnlock()
		http.Error(w, fmt.Sprintf("module %s is not running", id), http.StatusServiceUnavailable)
		return
	}
	port := inst.port
	m.mu.RUnlock()

	targetURL := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "proxy error", http.StatusInternalServerError)
		return
	}
	proxyReq.Header = r.Header

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("module %s unreachable: %v", id, err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// StopAll stops all modules
func (m *Manager) StopAll() {
	m.cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, inst := range m.modules {
		if inst.cmd != nil && inst.cmd.Process != nil {
			log.Printf("[modules] Stopping %s (PID: %d)", id, inst.pid)
			inst.cmd.Process.Kill()
		}
	}
}

// HealthCheck checks if a module is responsive
func (m *Manager) HealthCheck(id string) (bool, error) {
	m.mu.RLock()
	inst, ok := m.modules[id]
	if !ok {
		m.mu.RUnlock()
		return false, fmt.Errorf("module %s not found", id)
	}
	if inst.state != StateRunning {
		m.mu.RUnlock()
		return false, nil
	}
	port := inst.port
	m.mu.RUnlock()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// StartHealthChecker starts periodic health checks
func (m *Manager) StartHealthChecker(interval time.Duration) {
	go func() {
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-time.After(interval):
				m.mu.RLock()
				var runningModules []string
				for id, inst := range m.modules {
					if inst.state == StateRunning {
						runningModules = append(runningModules, id)
					}
				}
				m.mu.RUnlock()

				for _, id := range runningModules {
					healthy, _ := m.HealthCheck(id)
					if !healthy {
						log.Printf("[modules] Health check failed for %s", id)
					}
				}
			}
		}
	}()
}

func (m *Manager) getModuleConfig(cfg *config.Config, id string) *config.ModuleConfig {
	switch id {
	case "system_health":
		return &cfg.Modules.SystemHealth
	case "session_manager":
		return &cfg.Modules.SessionManager
	case "live_feed":
		return &cfg.Modules.LiveFeed
	case "log_viewer":
		return &cfg.Modules.LogViewer
	case "file_manager":
		return &cfg.Modules.FileManager
	case "cost_analyzer":
		return &cfg.Modules.CostAnalyzer
	case "rate_limiter":
		return &cfg.Modules.RateLimiter
	case "memory_viewer":
		return &cfg.Modules.MemoryViewer
	case "service_control":
		return &cfg.Modules.ServiceControl
	case "cron_manager":
		return &cfg.Modules.CronManager
	default:
		return nil
	}
}
