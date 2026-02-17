package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config represents the entire openclaw.config.json structure
type Config struct {
	Dashboard     DashboardConfig     `json:"dashboard"`
	Modules       ModulesConfig       `json:"modules"`
	Notifications NotificationsConfig `json:"notifications"`
	Security      SecurityConfig      `json:"security"`
	Advanced      AdvancedConfig      `json:"advanced"`
}

type DashboardConfig struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Title           string `json:"title"`
	RefreshInterval int    `json:"refresh_interval"`
	Theme           string `json:"theme"`
	Language        string `json:"language"`
}

type ModuleConfig struct {
	Enabled         bool     `json:"enabled"`
	Port            int      `json:"port"`
	RefreshInterval int      `json:"refresh_interval,omitempty"`
	HistoryHours    int      `json:"history_hours,omitempty"`
	TempWarning     int      `json:"temp_warning_celsius,omitempty"`
	TempCritical    int      `json:"temp_critical_celsius,omitempty"`
	SessionsDir     string   `json:"sessions_dir,omitempty"`
	MaxSessions     int      `json:"max_sessions_display,omitempty"`
	MaxMessages     int      `json:"max_messages,omitempty"`
	AutoScroll      bool     `json:"auto_scroll,omitempty"`
	LogPaths        []string `json:"log_paths,omitempty"`
	MaxLines        int      `json:"max_lines,omitempty"`
	Follow          bool     `json:"follow,omitempty"`
	RootDir         string   `json:"root_dir,omitempty"`
	AllowedExts     []string `json:"allowed_extensions,omitempty"`
	MaxFileSizeKB   int      `json:"max_file_size_kb,omitempty"`
	ShowHidden      bool     `json:"show_hidden,omitempty"`
	Currency        string   `json:"currency,omitempty"`
	BudgetWarning   float64  `json:"budget_warning,omitempty"`
	BudgetCritical  float64  `json:"budget_critical,omitempty"`
	WindowHours     int      `json:"window_hours,omitempty"`
	WarningPercent  int      `json:"warning_percent,omitempty"`
	MemoryDir       string   `json:"memory_dir,omitempty"`
	WatchFiles      []string `json:"watch_files,omitempty"`
	Services        []string `json:"services,omitempty"`
	AllowRestart    bool     `json:"allow_restart,omitempty"`
	AllowStop       bool     `json:"allow_stop,omitempty"`
	AllowEdit       bool     `json:"allow_edit,omitempty"`
	AllowTrigger    bool     `json:"allow_trigger,omitempty"`
}

type ModulesConfig struct {
	SystemHealth   ModuleConfig `json:"system_health"`
	SessionManager ModuleConfig `json:"session_manager"`
	LiveFeed       ModuleConfig `json:"live_feed"`
	LogViewer      ModuleConfig `json:"log_viewer"`
	FileManager    ModuleConfig `json:"file_manager"`
	CostAnalyzer   ModuleConfig `json:"cost_analyzer"`
	RateLimiter    ModuleConfig `json:"rate_limiter"`
	MemoryViewer   ModuleConfig `json:"memory_viewer"`
	ServiceControl ModuleConfig `json:"service_control"`
	CronManager    ModuleConfig `json:"cron_manager"`
}

type NotificationsConfig struct {
	Enabled            bool `json:"enabled"`
	BrowserNotifs      bool `json:"browser_notifications"`
	RateLimitWarning   bool `json:"rate_limit_warning"`
	BudgetWarning      bool `json:"budget_warning"`
	ServiceDownAlert   bool `json:"service_down_alert"`
}

type SecurityConfig struct {
	LANOnly         bool     `json:"lan_only"`
	AllowedNetworks []string `json:"allowed_networks"`
	ReadonlyMode    bool     `json:"readonly_mode"`
}

type AdvancedConfig struct {
	LogLevel             string `json:"log_level"`
	MaxModuleRestarts    int    `json:"max_module_restarts"`
	RestartBackoff       int    `json:"restart_backoff_seconds"`
	SSEKeepalive         int    `json:"sse_keepalive_seconds"`
	ModuleHealthCheck    int    `json:"module_health_check_seconds"`
	DataRetentionDays    int    `json:"data_retention_days"`
}

// ConfigManager handles loading, watching, and updating config
type ConfigManager struct {
	mu       sync.RWMutex
	config   *Config
	path     string
	watchers []func(*Config)
	modTime  time.Time
}

// NewConfigManager creates a new config manager
func NewConfigManager(configPath string) (*ConfigManager, error) {
	cm := &ConfigManager{
		path: configPath,
	}
	if err := cm.Load(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return cm, nil
}

// Load reads the config file from disk
func (cm *ConfigManager) Load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	absPath, err := filepath.Abs(cm.path)
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}
	cm.path = absPath

	data, err := os.ReadFile(cm.path)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", cm.path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	info, err := os.Stat(cm.path)
	if err == nil {
		cm.modTime = info.ModTime()
	}

	cm.config = &cfg
	return nil
}

// Get returns the current config (read-only)
func (cm *ConfigManager) Get() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
}

// GetRaw returns the raw JSON of the config file
func (cm *ConfigManager) GetRaw() (json.RawMessage, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	data, err := os.ReadFile(cm.path)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// Save writes updated config to disk
func (cm *ConfigManager) Save(data json.RawMessage) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Validate JSON by parsing it
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("invalid config JSON: %w", err)
	}

	// Pretty-print the JSON
	formatted, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format config: %w", err)
	}

	// Write to temp file first, then rename (atomic write)
	tmpPath := cm.path + ".tmp"
	if err := os.WriteFile(tmpPath, append(formatted, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}
	if err := os.Rename(tmpPath, cm.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to replace config: %w", err)
	}

	cm.config = &cfg

	info, err := os.Stat(cm.path)
	if err == nil {
		cm.modTime = info.ModTime()
	}

	// Notify watchers
	for _, w := range cm.watchers {
		go w(&cfg)
	}

	return nil
}

// Watch registers a callback for config changes
func (cm *ConfigManager) Watch(callback func(*Config)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.watchers = append(cm.watchers, callback)
}

// StartFileWatcher polls for config file changes
func (cm *ConfigManager) StartFileWatcher(interval time.Duration) {
	go func() {
		for {
			time.Sleep(interval)
			info, err := os.Stat(cm.path)
			if err != nil {
				continue
			}
			cm.mu.RLock()
			changed := info.ModTime().After(cm.modTime)
			cm.mu.RUnlock()

			if changed {
				log.Println("[config] Config file changed, reloading...")
				if err := cm.Load(); err != nil {
					log.Printf("[config] Failed to reload config: %v", err)
					continue
				}
				cm.mu.RLock()
				cfg := cm.config
				watchers := cm.watchers
				cm.mu.RUnlock()
				for _, w := range watchers {
					go w(cfg)
				}
			}
		}
	}()
}

// Path returns the config file path
func (cm *ConfigManager) Path() string {
	return cm.path
}
