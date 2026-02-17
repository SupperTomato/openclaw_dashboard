package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/SupperTomato/openclaw_dashboard/backend/internal/api"
	"github.com/SupperTomato/openclaw_dashboard/backend/internal/config"
	"github.com/SupperTomato/openclaw_dashboard/backend/internal/modules"
)

func main() {
	configPath := flag.String("config", "", "Path to openclaw.config.json")
	flag.Parse()

	// Find config file
	cfgPath := findConfigPath(*configPath)
	log.Printf("[dashboard] Using config: %s", cfgPath)

	// Load config
	configMgr, err := config.NewConfigManager(cfgPath)
	if err != nil {
		log.Fatalf("[dashboard] Failed to load config: %v", err)
	}

	cfg := configMgr.Get()

	// Watch for config file changes
	configMgr.StartFileWatcher(5 * time.Second)

	// Determine paths
	execPath, _ := os.Executable()
	baseDir := filepath.Dir(filepath.Dir(filepath.Dir(execPath)))
	if baseDir == "" || baseDir == "." {
		baseDir, _ = os.Getwd()
	}

	// Check for frontend dirs relative to working directory
	staticDir := findDir(baseDir, "frontend/public")
	tmplDir := findDir(baseDir, "frontend/templates")
	binDir := findDir(baseDir, "backend/bin")

	log.Printf("[dashboard] Static dir: %s", staticDir)
	log.Printf("[dashboard] Template dir: %s", tmplDir)
	log.Printf("[dashboard] Module bin dir: %s", binDir)

	// Create module manager
	modMgr := modules.NewManager(configMgr, binDir)

	// Set up router
	router := api.NewRouter(configMgr, modMgr, staticDir, tmplDir)

	// Start modules
	modMgr.StartAll()
	modMgr.StartHealthChecker(time.Duration(cfg.Advanced.ModuleHealthCheck) * time.Second)

	// Start HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Dashboard.Host, cfg.Dashboard.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      router.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Disabled for SSE
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("[dashboard] Shutting down...")
		modMgr.StopAll()
		server.Close()
	}()

	log.Printf("[dashboard] OpenClaw Dashboard starting on http://%s", addr)
	log.Printf("[dashboard] Access from LAN: http://<your-ip>:%d", cfg.Dashboard.Port)

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[dashboard] Server error: %v", err)
	}

	log.Println("[dashboard] Shutdown complete")
}

func findConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}

	// Check common locations
	candidates := []string{
		"config/openclaw.config.json",
		"../config/openclaw.config.json",
		"../../config/openclaw.config.json",
		filepath.Join(os.Getenv("HOME"), ".config/openclaw/openclaw.config.json"),
		"/etc/openclaw/openclaw.config.json",
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	log.Fatal("[dashboard] Config file not found. Use --config flag to specify path.")
	return ""
}

func findDir(baseDir, relPath string) string {
	// Try relative to base dir
	p := filepath.Join(baseDir, relPath)
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p
	}

	// Try relative to cwd
	cwd, _ := os.Getwd()
	p = filepath.Join(cwd, relPath)
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p
	}

	// Try going up
	for _, prefix := range []string{".", "..", "../.."} {
		p = filepath.Join(prefix, relPath)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}

	// Return relative path as fallback
	return filepath.Join(baseDir, relPath)
}
