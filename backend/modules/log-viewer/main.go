package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type LogEntry struct {
	Line      string    `json:"line"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"` // info, warn, error, debug
}

type LogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
}

func NewLogBuffer(maxSize int) *LogBuffer {
	return &LogBuffer{maxSize: maxSize}
}

func (b *LogBuffer) Add(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, entry)
	if len(b.entries) > b.maxSize {
		b.entries = b.entries[len(b.entries)-b.maxSize:]
	}
}

func (b *LogBuffer) GetAll() []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]LogEntry, len(b.entries))
	copy(result, b.entries)
	return result
}

func (b *LogBuffer) GetFiltered(level, search string, limit int) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []LogEntry
	for i := len(b.entries) - 1; i >= 0 && (limit <= 0 || len(result) < limit); i-- {
		e := b.entries[i]
		if level != "" && e.Level != level {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(e.Line), strings.ToLower(search)) {
			continue
		}
		result = append(result, e)
	}

	// Reverse to chronological order
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

var buffer *LogBuffer

func main() {
	port := os.Getenv("MODULE_PORT")
	if port == "" {
		port = "9004"
	}

	maxLines := 1000
	logPaths := []string{"/var/log/syslog"}

	if cfg := os.Getenv("MODULE_CONFIG"); cfg != "" {
		var c map[string]interface{}
		if json.Unmarshal([]byte(cfg), &c) == nil {
			if ml, ok := c["max_lines"].(float64); ok {
				maxLines = int(ml)
			}
			if lp, ok := c["log_paths"].([]interface{}); ok {
				logPaths = nil
				for _, p := range lp {
					if s, ok := p.(string); ok {
						logPaths = append(logPaths, s)
					}
				}
			}
		}
	}

	buffer = NewLogBuffer(maxLines)

	// Start watching log files
	for _, path := range logPaths {
		go watchLogFile(path)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/logs", handleLogs)
	mux.HandleFunc("/stream", handleStream)

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

	log.Printf("[log-viewer] Starting on port %s, watching: %v", port, logPaths)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[log-viewer] Server error: %v", err)
	}
}

func watchLogFile(path string) {
	for {
		file, err := os.Open(path)
		if err != nil {
			log.Printf("[log-viewer] Cannot open %s: %v, retrying in 10s", path, err)
			time.Sleep(10 * time.Second)
			continue
		}

		// Seek to end
		file.Seek(0, 2)

		scanner := bufio.NewScanner(file)
		for {
			for scanner.Scan() {
				line := scanner.Text()
				entry := LogEntry{
					Line:      line,
					Source:    path,
					Timestamp: time.Now(),
					Level:     detectLevel(line),
				}
				buffer.Add(entry)
			}
			// Wait for new data
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func detectLevel(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") || strings.Contains(lower, "panic") {
		return "error"
	}
	if strings.Contains(lower, "warn") {
		return "warn"
	}
	if strings.Contains(lower, "debug") {
		return "debug"
	}
	return "info"
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	entries := buffer.GetAll()
	errors := 0
	warnings := 0
	for _, e := range entries {
		if e.Level == "error" {
			errors++
		}
		if e.Level == "warn" {
			warnings++
		}
	}
	writeJSON(w, map[string]interface{}{
		"total_lines": len(entries),
		"errors":      errors,
		"warnings":    warnings,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"module":    "log_viewer",
		"status":    "running",
		"buffered":  len(buffer.GetAll()),
	})
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	level := r.URL.Query().Get("level")
	search := r.URL.Query().Get("q")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	entries := buffer.GetFiltered(level, search, limit)
	writeJSON(w, entries)
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	lastCount := len(buffer.GetAll())

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			entries := buffer.GetAll()
			if len(entries) > lastCount {
				newEntries := entries[lastCount:]
				data, _ := json.Marshal(newEntries)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
				lastCount = len(entries)
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
