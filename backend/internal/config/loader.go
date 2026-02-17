package config

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Config struct {
	OpencuRootDir string `json:"openclaw_root_dir"`
	OpencuUser    string `json:"openclaw_user"`
	Dashboard     struct {
		Host            string `json:"host"`
		Port            int    `json:"port"`
		Title           string `json:"title"`
		RefreshInterval int    `json:"refresh_interval"`
		Theme           string `json:"theme"`
	} `json:"dashboard"`
	Modules map[string]struct {
		Enabled bool `json:"enabled"`
		Port    int  `json:"port"`
	} `json:"modules"`
	Security struct {
		LANOnly         bool     `json:"lan_only"`
		AllowedNetworks []string `json:"allowed_networks"`
	} `json:"security"`
	Advanced struct {
		LogLevel             string `json:"log_level"`
		MaxModuleRestarts    int    `json:"max_module_restarts"`
		ModuleHealthCheckSec int    `json:"module_health_check_seconds"`
	} `json:"advanced"`
}

var (
	cfg     *Config
	cfgPath string
	mu      sync.RWMutex
	modTime time.Time
)

func Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}
	mu.Lock()
	cfg = &c
	cfgPath = path
	info, _ := os.Stat(path)
	if info != nil {
		modTime = info.ModTime()
	}
	mu.Unlock()
	return nil
}

func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return cfg
}

func GetRaw() (json.RawMessage, error) {
	data, err := os.ReadFile(cfgPath)
	return json.RawMessage(data), err
}

func Save(data json.RawMessage) error {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}
	formatted, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(cfgPath, append(formatted, '\n'), 0644); err != nil {
		return err
	}
	mu.Lock()
	cfg = &c
	info, _ := os.Stat(cfgPath)
	if info != nil {
		modTime = info.ModTime()
	}
	mu.Unlock()
	return nil
}

func IsSetup() bool {
	mu.RLock()
	defer mu.RUnlock()
	return cfg != nil && cfg.OpencuRootDir != ""
}

func SetupRoot(root string) error {
	mu.Lock()
	cfg.OpencuRootDir = root
	mu.Unlock()
	data, _ := json.Marshal(cfg)
	return Save(json.RawMessage(data))
}

func Path() string {
	mu.RLock()
	defer mu.RUnlock()
	return cfgPath
}

func Watch() {
	go func() {
		for {
			time.Sleep(5 * time.Second)
			info, err := os.Stat(cfgPath)
			if err != nil {
				continue
			}
			mu.RLock()
			changed := info.ModTime().After(modTime)
			mu.RUnlock()
			if changed {
				Load(cfgPath)
			}
		}
	}()
}
