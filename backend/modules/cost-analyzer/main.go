package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Pricing per 1M tokens (approximate Claude pricing)
var modelPricing = map[string]struct{ Input, Output float64 }{
	"claude-3-opus":   {15.0, 75.0},
	"claude-3-sonnet": {3.0, 15.0},
	"claude-3-haiku":  {0.25, 1.25},
	"claude-3.5-sonnet": {3.0, 15.0},
	"claude-4-opus":   {15.0, 75.0},
	"claude-4-sonnet": {3.0, 15.0},
	"default":         {3.0, 15.0},
}

type CostEntry struct {
	Date         string  `json:"date"`
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	Cost         float64 `json:"cost"`
	Session      string  `json:"session"`
}

type CostSummary struct {
	TotalCost      float64            `json:"total_cost"`
	TotalInput     int64              `json:"total_input_tokens"`
	TotalOutput    int64              `json:"total_output_tokens"`
	ByModel        map[string]float64 `json:"by_model"`
	ByDate         map[string]float64 `json:"by_date"`
	Currency       string             `json:"currency"`
	BudgetWarning  float64            `json:"budget_warning"`
	BudgetCritical float64            `json:"budget_critical"`
	BudgetStatus   string             `json:"budget_status"` // ok, warning, critical
}

var (
	mu       sync.RWMutex
	entries  []CostEntry
	currency string
	budgetW  float64
	budgetC  float64
)

func main() {
	port := os.Getenv("MODULE_PORT")
	if port == "" {
		port = "9006"
	}

	currency = "USD"
	budgetW = 50.0
	budgetC = 100.0

	if cfg := os.Getenv("MODULE_CONFIG"); cfg != "" {
		var c map[string]interface{}
		if json.Unmarshal([]byte(cfg), &c) == nil {
			if cur, ok := c["currency"].(string); ok {
				currency = cur
			}
			if bw, ok := c["budget_warning"].(float64); ok {
				budgetW = bw
			}
			if bc, ok := c["budget_critical"].(float64); ok {
				budgetC = bc
			}
		}
	}

	// Try to load existing data
	loadData()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/summary", handleSummary)
	mux.HandleFunc("/entries", handleEntries)
	mux.HandleFunc("/record", handleRecord)

	server := &http.Server{
		Addr:    "127.0.0.1:" + port,
		Handler: mux,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		saveData()
		server.Close()
	}()

	log.Printf("[cost-analyzer] Starting on port %s", port)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[cost-analyzer] Server error: %v", err)
	}
}

func dataPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "openclaw")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "cost_data.json")
}

func loadData() {
	data, err := os.ReadFile(dataPath())
	if err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	json.Unmarshal(data, &entries)
}

func saveData() {
	mu.RLock()
	defer mu.RUnlock()
	data, _ := json.MarshalIndent(entries, "", "  ")
	os.WriteFile(dataPath(), data, 0644)
}

func calculateSummary() CostSummary {
	mu.RLock()
	defer mu.RUnlock()

	summary := CostSummary{
		ByModel:        make(map[string]float64),
		ByDate:         make(map[string]float64),
		Currency:       currency,
		BudgetWarning:  budgetW,
		BudgetCritical: budgetC,
	}

	for _, e := range entries {
		summary.TotalCost += e.Cost
		summary.TotalInput += e.InputTokens
		summary.TotalOutput += e.OutputTokens
		summary.ByModel[e.Model] += e.Cost
		summary.ByDate[e.Date] += e.Cost
	}

	if summary.TotalCost >= budgetC {
		summary.BudgetStatus = "critical"
	} else if summary.TotalCost >= budgetW {
		summary.BudgetStatus = "warning"
	} else {
		summary.BudgetStatus = "ok"
	}

	return summary
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	s := calculateSummary()
	writeJSON(w, map[string]interface{}{
		"total_cost":    s.TotalCost,
		"budget_status": s.BudgetStatus,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"module": "cost_analyzer",
		"status": "running",
	})
}

func handleSummary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, calculateSummary())
}

func handleEntries(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()

	// Filter by date range if provided
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	model := strings.ToLower(r.URL.Query().Get("model"))

	var filtered []CostEntry
	for _, e := range entries {
		if from != "" && e.Date < from {
			continue
		}
		if to != "" && e.Date > to {
			continue
		}
		if model != "" && !strings.Contains(strings.ToLower(e.Model), model) {
			continue
		}
		filtered = append(filtered, e)
	}

	writeJSON(w, filtered)
}

func handleRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var entry CostEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if entry.Date == "" {
		entry.Date = time.Now().Format("2006-01-02")
	}

	// Calculate cost if not provided
	if entry.Cost == 0 && (entry.InputTokens > 0 || entry.OutputTokens > 0) {
		pricing, ok := modelPricing[entry.Model]
		if !ok {
			pricing = modelPricing["default"]
		}
		entry.Cost = (float64(entry.InputTokens)/1_000_000)*pricing.Input +
			(float64(entry.OutputTokens)/1_000_000)*pricing.Output
	}

	mu.Lock()
	entries = append(entries, entry)
	mu.Unlock()

	go saveData()

	writeJSON(w, map[string]interface{}{"success": true})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
