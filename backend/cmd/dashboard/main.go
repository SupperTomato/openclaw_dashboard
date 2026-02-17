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
	cfgPath := flag.String("config", "", "Config file path")
	flag.Parse()

	path := findConfig(*cfgPath)
	if err := config.Load(path); err != nil {
		log.Fatalf("Config load failed: %v", err)
	}

	cfg := config.Get()
	dirs := findDirs()

	log.Printf("[dash] Config: %s", path)
	log.Printf("[dash] Static: %s, Templates: %s", dirs["static"], dirs["templates"])

	modMgr := modules.NewManager(config.Get(), dirs["bin"])
	router := api.NewRouter(dirs["static"], dirs["templates"])

	modMgr.StartAll()
	modMgr.StartHealthChecker(time.Duration(cfg.Advanced.ModuleHealthCheckSec) * time.Second)
	config.Watch()

	addr := fmt.Sprintf("%s:%d", cfg.Dashboard.Host, cfg.Dashboard.Port)
	srv := &http.Server{Addr: addr, Handler: router.Handler(modMgr, config.Get())}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		modMgr.StopAll()
		srv.Close()
	}()

	log.Printf("[dash] Starting http://%s", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[dash] Server error: %v", err)
	}
}

func findConfig(explicit string) string {
	if explicit != "" {
		return explicit
	}
	candidates := []string{
		"config/dashboard.json",
		"../config/dashboard.json",
		"../../config/dashboard.json",
		filepath.Join(os.Getenv("HOME"), ".config/openclaw/dashboard.json"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	log.Fatal("Config file not found. Use --config flag.")
	return ""
}

func findDirs() map[string]string {
	cwd, _ := os.Getwd()
	return map[string]string{
		"static":    findDir(cwd, "frontend/public"),
		"templates": findDir(cwd, "frontend/templates"),
		"bin":       findDir(cwd, "backend/bin"),
	}
}

func findDir(base, rel string) string {
	cwd, _ := os.Getwd()
	paths := []string{
		filepath.Join(base, rel),
		filepath.Join(cwd, rel),
		rel,
	}
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return rel
}
