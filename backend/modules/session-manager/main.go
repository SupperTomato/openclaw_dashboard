package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type Session struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"` // active, idle, completed
	StartedAt   time.Time `json:"started_at"`
	LastActive  time.Time `json:"last_active"`
	MessageCount int      `json:"message_count"`
	Model       string    `json:"model"`
	Directory   string    `json:"directory"`
}

var sessionsDir string

func main() {
	port := os.Getenv("MODULE_PORT")
	if port == "" {
		port = "9002"
	}

	sessionsDir = os.ExpandEnv("$HOME/.claude/projects")
	if cfg := os.Getenv("MODULE_CONFIG"); cfg != "" {
		var c map[string]interface{}
		if json.Unmarshal([]byte(cfg), &c) == nil {
			if sd, ok := c["sessions_dir"].(string); ok {
				sessionsDir = expandPath(sd)
			}
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/sessions", handleSessions)
	mux.HandleFunc("/session/", handleSessionDetail)

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

	log.Printf("[session-manager] Starting on port %s, watching: %s", port, sessionsDir)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[session-manager] Server error: %v", err)
	}
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return os.ExpandEnv(p)
}

func scanSessions() []Session {
	var sessions []Session

	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		return sessions
	}

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		log.Printf("[session-manager] Error reading sessions dir: %v", err)
		return sessions
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessPath := filepath.Join(sessionsDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		session := Session{
			ID:        entry.Name(),
			Name:      entry.Name(),
			Directory: sessPath,
			StartedAt: info.ModTime(),
		}

		// Check for activity - look for recent files
		session.LastActive = info.ModTime()
		session.Status = "completed"

		// Check if session has recent activity (within last 5 minutes)
		filepath.Walk(sessPath, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.ModTime().After(session.LastActive) {
				session.LastActive = fi.ModTime()
			}
			if !fi.IsDir() {
				session.MessageCount++
			}
			return nil
		})

		if time.Since(session.LastActive) < 5*time.Minute {
			session.Status = "active"
		} else if time.Since(session.LastActive) < 1*time.Hour {
			session.Status = "idle"
		}

		sessions = append(sessions, session)
	}

	// Sort by last active (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActive.After(sessions[j].LastActive)
	})

	return sessions
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	sessions := scanSessions()
	active := 0
	for _, s := range sessions {
		if s.Status == "active" {
			active++
		}
	}
	writeJSON(w, map[string]interface{}{
		"total_sessions":  len(sessions),
		"active_sessions": active,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	sessions := scanSessions()
	writeJSON(w, map[string]interface{}{
		"module":   "session_manager",
		"status":   "running",
		"sessions": len(sessions),
		"dir":      sessionsDir,
	})
}

func handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions := scanSessions()

	// Filter by status if requested
	status := r.URL.Query().Get("status")
	if status != "" {
		var filtered []Session
		for _, s := range sessions {
			if s.Status == status {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}

	// Search
	search := strings.ToLower(r.URL.Query().Get("q"))
	if search != "" {
		var filtered []Session
		for _, s := range sessions {
			if strings.Contains(strings.ToLower(s.Name), search) ||
				strings.Contains(strings.ToLower(s.ID), search) {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}

	writeJSON(w, sessions)
}

func handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/session/")
	sessions := scanSessions()
	for _, s := range sessions {
		if s.ID == id {
			writeJSON(w, s)
			return
		}
	}
	http.Error(w, "session not found", http.StatusNotFound)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
