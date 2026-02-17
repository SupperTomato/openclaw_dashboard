package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

type ServiceStatus struct {
	Name    string `json:"name"`
	Active  bool   `json:"active"`
	Status  string `json:"status"`
	Enabled bool   `json:"enabled"`
}

var (
	services     []string
	allowRestart bool
	allowStop    bool
)

func main() {
	port := os.Getenv("MODULE_PORT")
	if port == "" {
		port = "9009"
	}

	services = []string{"openclaw", "openclaw-dashboard"}
	allowRestart = true
	allowStop = false

	if cfg := os.Getenv("MODULE_CONFIG"); cfg != "" {
		var c map[string]interface{}
		if json.Unmarshal([]byte(cfg), &c) == nil {
			if svcs, ok := c["services"].([]interface{}); ok {
				services = nil
				for _, s := range svcs {
					if str, ok := s.(string); ok {
						services = append(services, str)
					}
				}
			}
			if ar, ok := c["allow_restart"].(bool); ok {
				allowRestart = ar
			}
			if as, ok := c["allow_stop"].(bool); ok {
				allowStop = as
			}
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/services", handleServices)
	mux.HandleFunc("/restart/", handleRestart)
	mux.HandleFunc("/stop/", handleStop)

	server := &http.Server{
		Addr:    "127.0.0.1:" + port,
		Handler: mux,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		server.Close()
	}()

	log.Printf("[service-control] Starting on port %s", port)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[service-control] Server error: %v", err)
	}
}

func getServiceStatus(name string) ServiceStatus {
	ss := ServiceStatus{Name: name}

	// Check systemd service status
	out, err := exec.Command("systemctl", "is-active", name).Output()
	if err == nil {
		status := strings.TrimSpace(string(out))
		ss.Active = status == "active"
		ss.Status = status
	} else {
		ss.Status = "unknown"
	}

	// Check if enabled
	out, err = exec.Command("systemctl", "is-enabled", name).Output()
	if err == nil {
		ss.Enabled = strings.TrimSpace(string(out)) == "enabled"
	}

	return ss
}

func isAllowedService(name string) bool {
	for _, s := range services {
		if s == name {
			return true
		}
	}
	return false
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	active := 0
	for _, s := range services {
		if getServiceStatus(s).Active {
			active++
		}
	}
	writeJSON(w, map[string]interface{}{
		"total_services":  len(services),
		"active_services": active,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"module":        "service_control",
		"status":        "running",
		"allow_restart": allowRestart,
		"allow_stop":    allowStop,
	})
}

func handleServices(w http.ResponseWriter, r *http.Request) {
	var statuses []ServiceStatus
	for _, s := range services {
		statuses = append(statuses, getServiceStatus(s))
	}
	writeJSON(w, statuses)
}

func handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !allowRestart {
		http.Error(w, "restart not allowed", http.StatusForbidden)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/restart/")
	if !isAllowedService(name) {
		http.Error(w, "service not in allowed list", http.StatusForbidden)
		return
	}

	out, err := exec.Command("systemctl", "restart", name).CombinedOutput()
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   string(out),
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Service " + name + " restarted",
	})
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !allowStop {
		http.Error(w, "stop not allowed", http.StatusForbidden)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/stop/")
	if !isAllowedService(name) {
		http.Error(w, "service not in allowed list", http.StatusForbidden)
		return
	}

	out, err := exec.Command("systemctl", "stop", name).CombinedOutput()
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   string(out),
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Service " + name + " stopped",
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
