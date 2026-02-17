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

type CronJob struct {
	Line     string `json:"line"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Enabled  bool   `json:"enabled"`
	Index    int    `json:"index"`
}

var (
	allowEdit    bool
	allowTrigger bool
)

func main() {
	port := os.Getenv("MODULE_PORT")
	if port == "" {
		port = "9010"
	}

	allowEdit = true
	allowTrigger = true

	if cfg := os.Getenv("MODULE_CONFIG"); cfg != "" {
		var c map[string]interface{}
		if json.Unmarshal([]byte(cfg), &c) == nil {
			if ae, ok := c["allow_edit"].(bool); ok {
				allowEdit = ae
			}
			if at, ok := c["allow_trigger"].(bool); ok {
				allowTrigger = at
			}
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/jobs", handleJobs)
	mux.HandleFunc("/trigger/", handleTrigger)
	mux.HandleFunc("/toggle/", handleToggle)

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

	log.Printf("[cron-manager] Starting on port %s", port)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[cron-manager] Server error: %v", err)
	}
}

func parseCrontab() []CronJob {
	out, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		return nil
	}

	var jobs []CronJob
	for i, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		job := CronJob{
			Line:  line,
			Index: i,
		}

		if strings.HasPrefix(line, "#") {
			job.Enabled = false
			// Try to parse disabled cron line
			stripped := strings.TrimPrefix(line, "#")
			stripped = strings.TrimSpace(stripped)
			parts := strings.Fields(stripped)
			if len(parts) >= 6 {
				job.Schedule = strings.Join(parts[:5], " ")
				job.Command = strings.Join(parts[5:], " ")
			} else {
				job.Command = stripped
			}
		} else {
			job.Enabled = true
			parts := strings.Fields(line)
			if len(parts) >= 6 {
				job.Schedule = strings.Join(parts[:5], " ")
				job.Command = strings.Join(parts[5:], " ")
			} else {
				job.Command = line
			}
		}

		jobs = append(jobs, job)
	}

	return jobs
}

func saveCrontab(lines []string) error {
	content := strings.Join(lines, "\n") + "\n"
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	return cmd.Run()
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	jobs := parseCrontab()
	enabled := 0
	for _, j := range jobs {
		if j.Enabled {
			enabled++
		}
	}
	writeJSON(w, map[string]interface{}{
		"total_jobs":   len(jobs),
		"enabled_jobs": enabled,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"module":        "cron_manager",
		"status":        "running",
		"allow_edit":    allowEdit,
		"allow_trigger": allowTrigger,
	})
}

func handleJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, parseCrontab())
}

func handleTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !allowTrigger {
		http.Error(w, "manual trigger not allowed", http.StatusForbidden)
		return
	}

	var req struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Verify the command exists in crontab (security)
	jobs := parseCrontab()
	found := false
	for _, j := range jobs {
		if j.Command == req.Command {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "command not found in crontab", http.StatusBadRequest)
		return
	}

	out, err := exec.Command("bash", "-c", req.Command).CombinedOutput()
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"success": false,
			"output":  string(out),
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"output":  string(out),
	})
}

func handleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !allowEdit {
		http.Error(w, "editing not allowed", http.StatusForbidden)
		return
	}

	var req struct {
		Index  int  `json:"index"`
		Enable bool `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	out, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		http.Error(w, "failed to read crontab", http.StatusInternalServerError)
		return
	}

	lines := strings.Split(string(out), "\n")
	if req.Index < 0 || req.Index >= len(lines) {
		http.Error(w, "invalid index", http.StatusBadRequest)
		return
	}

	line := strings.TrimSpace(lines[req.Index])
	if req.Enable && strings.HasPrefix(line, "#") {
		lines[req.Index] = strings.TrimPrefix(line, "#")
		lines[req.Index] = strings.TrimSpace(lines[req.Index])
	} else if !req.Enable && !strings.HasPrefix(line, "#") {
		lines[req.Index] = "# " + line
	}

	if err := saveCrontab(lines); err != nil {
		writeJSON(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
