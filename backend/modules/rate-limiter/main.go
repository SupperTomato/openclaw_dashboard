package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type RateEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Tokens    int64     `json:"tokens"`
	Model     string    `json:"model"`
}

type RateStatus struct {
	WindowHours    int     `json:"window_hours"`
	TotalTokens    int64   `json:"total_tokens"`
	RequestCount   int     `json:"request_count"`
	UsagePercent   float64 `json:"usage_percent"`
	WarningPercent int     `json:"warning_percent"`
	Status         string  `json:"status"` // ok, warning, critical
	WindowStart    string  `json:"window_start"`
	WindowEnd      string  `json:"window_end"`
}

var (
	mu             sync.RWMutex
	rateEntries    []RateEntry
	windowHours    int
	warningPercent int
	// Approximate rate limits (tokens per window)
	rateLimits = map[string]int64{
		"claude-3-opus":     500000,
		"claude-3-sonnet":   1000000,
		"claude-3-haiku":    2000000,
		"claude-3.5-sonnet": 1000000,
		"claude-4-opus":     500000,
		"claude-4-sonnet":   1000000,
		"default":           1000000,
	}
)

func main() {
	port := os.Getenv("MODULE_PORT")
	if port == "" {
		port = "9007"
	}

	windowHours = 5
	warningPercent = 80

	if cfg := os.Getenv("MODULE_CONFIG"); cfg != "" {
		var c map[string]interface{}
		if json.Unmarshal([]byte(cfg), &c) == nil {
			if wh, ok := c["window_hours"].(float64); ok {
				windowHours = int(wh)
			}
			if wp, ok := c["warning_percent"].(float64); ok {
				warningPercent = int(wp)
			}
		}
	}

	// Cleanup old entries periodically
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			cleanupOldEntries()
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/record", handleRecord)
	mux.HandleFunc("/reset", handleReset)

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

	log.Printf("[rate-limiter] Starting on port %s, window: %dh", port, windowHours)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[rate-limiter] Server error: %v", err)
	}
}

func cleanupOldEntries() {
	mu.Lock()
	defer mu.Unlock()
	cutoff := time.Now().Add(-time.Duration(windowHours) * time.Hour)
	i := 0
	for i < len(rateEntries) && rateEntries[i].Timestamp.Before(cutoff) {
		i++
	}
	if i > 0 {
		rateEntries = rateEntries[i:]
	}
}

func getStatus() RateStatus {
	mu.RLock()
	defer mu.RUnlock()

	now := time.Now()
	windowStart := now.Add(-time.Duration(windowHours) * time.Hour)

	status := RateStatus{
		WindowHours:    windowHours,
		WarningPercent: warningPercent,
		WindowStart:    windowStart.Format(time.RFC3339),
		WindowEnd:      now.Format(time.RFC3339),
	}

	for _, e := range rateEntries {
		if e.Timestamp.After(windowStart) {
			status.TotalTokens += e.Tokens
			status.RequestCount++
		}
	}

	limit := int64(rateLimits["default"])
	if status.TotalTokens > 0 {
		status.UsagePercent = float64(status.TotalTokens) / float64(limit) * 100.0
	}

	if status.UsagePercent >= 95 {
		status.Status = "critical"
	} else if status.UsagePercent >= float64(warningPercent) {
		status.Status = "warning"
	} else {
		status.Status = "ok"
	}

	return status
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	s := getStatus()
	writeJSON(w, map[string]interface{}{
		"usage_percent": s.UsagePercent,
		"status":        s.Status,
		"total_tokens":  s.TotalTokens,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, getStatus())
}

func handleRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var entry RateEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	mu.Lock()
	rateEntries = append(rateEntries, entry)
	mu.Unlock()

	writeJSON(w, map[string]interface{}{"success": true})
}

func handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mu.Lock()
	rateEntries = nil
	mu.Unlock()

	writeJSON(w, map[string]interface{}{"success": true, "message": "rate data cleared"})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
