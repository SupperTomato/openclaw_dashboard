package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type SystemStats struct {
	CPU         float64   `json:"cpu_percent"`
	MemTotal    uint64    `json:"mem_total_bytes"`
	MemUsed     uint64    `json:"mem_used_bytes"`
	MemPercent  float64   `json:"mem_percent"`
	DiskTotal   uint64    `json:"disk_total_bytes"`
	DiskUsed    uint64    `json:"disk_used_bytes"`
	DiskPercent float64   `json:"disk_percent"`
	Temperature float64   `json:"temperature_celsius"`
	LoadAvg     [3]float64 `json:"load_average"`
	Uptime      int64     `json:"uptime_seconds"`
	Timestamp   time.Time `json:"timestamp"`
}

type HealthHistory struct {
	mu      sync.RWMutex
	entries []SystemStats
	maxAge  time.Duration
}

func NewHealthHistory(maxAge time.Duration) *HealthHistory {
	return &HealthHistory{
		maxAge: maxAge,
	}
}

func (h *HealthHistory) Add(s SystemStats) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, s)
	// Trim old entries
	cutoff := time.Now().Add(-h.maxAge)
	i := 0
	for i < len(h.entries) && h.entries[i].Timestamp.Before(cutoff) {
		i++
	}
	if i > 0 {
		h.entries = h.entries[i:]
	}
}

func (h *HealthHistory) GetAll() []SystemStats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]SystemStats, len(h.entries))
	copy(result, h.entries)
	return result
}

func (h *HealthHistory) GetSparkline(field string, points int) []float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.entries) == 0 {
		return nil
	}

	// Sample entries to get 'points' data points
	step := len(h.entries) / points
	if step < 1 {
		step = 1
	}

	var result []float64
	for i := 0; i < len(h.entries); i += step {
		switch field {
		case "cpu":
			result = append(result, h.entries[i].CPU)
		case "mem":
			result = append(result, h.entries[i].MemPercent)
		case "temp":
			result = append(result, h.entries[i].Temperature)
		}
		if len(result) >= points {
			break
		}
	}
	return result
}

var history *HealthHistory

func main() {
	port := os.Getenv("MODULE_PORT")
	if port == "" {
		port = "9001"
	}

	historyHours := 24
	if h := os.Getenv("MODULE_CONFIG"); h != "" {
		var cfg map[string]interface{}
		if json.Unmarshal([]byte(h), &cfg) == nil {
			if hh, ok := cfg["history_hours"].(float64); ok {
				historyHours = int(hh)
			}
		}
	}

	history = NewHealthHistory(time.Duration(historyHours) * time.Hour)

	// Start collection
	go collectStats()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/history", handleHistory)
	mux.HandleFunc("/sparkline", handleSparkline)

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

	log.Printf("[system-health] Starting on port %s", port)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[system-health] Server error: %v", err)
	}
}

func collectStats() {
	// Collect immediately, then on interval
	refreshInterval := 3
	if ri := os.Getenv("MODULE_CONFIG"); ri != "" {
		var cfg map[string]interface{}
		if json.Unmarshal([]byte(ri), &cfg) == nil {
			if r, ok := cfg["refresh_interval"].(float64); ok {
				refreshInterval = int(r)
			}
		}
	}

	for {
		stats := gatherStats()
		history.Add(stats)
		time.Sleep(time.Duration(refreshInterval) * time.Second)
	}
}

func gatherStats() SystemStats {
	stats := SystemStats{
		Timestamp: time.Now(),
	}

	// CPU usage from /proc/stat
	stats.CPU = getCPUPercent()

	// Memory from /proc/meminfo
	stats.MemTotal, stats.MemUsed, stats.MemPercent = getMemInfo()

	// Disk usage
	stats.DiskTotal, stats.DiskUsed, stats.DiskPercent = getDiskInfo("/")

	// Temperature
	stats.Temperature = getTemperature()

	// Load average
	stats.LoadAvg = getLoadAvg()

	// Uptime
	stats.Uptime = getUptime()

	return stats
}

var prevIdle, prevTotal uint64

func getCPUPercent() float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0
	}

	fields := strings.Fields(lines[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}

	var values []uint64
	for _, f := range fields[1:] {
		v, _ := strconv.ParseUint(f, 10, 64)
		values = append(values, v)
	}

	if len(values) < 4 {
		return 0
	}

	idle := values[3]
	var total uint64
	for _, v := range values {
		total += v
	}

	if prevTotal == 0 {
		prevIdle = idle
		prevTotal = total
		return 0
	}

	diffIdle := float64(idle - prevIdle)
	diffTotal := float64(total - prevTotal)
	prevIdle = idle
	prevTotal = total

	if diffTotal == 0 {
		return 0
	}

	return (1.0 - diffIdle/diffTotal) * 100.0
}

func getMemInfo() (total, used uint64, percent float64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}

	var memTotal, memAvailable uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		val *= 1024 // kB to bytes

		switch fields[0] {
		case "MemTotal:":
			memTotal = val
		case "MemAvailable:":
			memAvailable = val
		}
	}

	total = memTotal
	used = memTotal - memAvailable
	if memTotal > 0 {
		percent = float64(used) / float64(memTotal) * 100.0
	}
	return
}

func getDiskInfo(path string) (total, used uint64, percent float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return
	}

	total = stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used = total - free
	if total > 0 {
		percent = float64(used) / float64(total) * 100.0
	}
	return
}

func getTemperature() float64 {
	// Try common thermal zone paths
	paths := []string{
		"/sys/class/thermal/thermal_zone0/temp",
		"/sys/class/thermal/thermal_zone1/temp",
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		temp, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil {
			continue
		}
		return temp / 1000.0 // millidegrees to degrees
	}
	return 0
}

func getLoadAvg() [3]float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return [3]float64{}
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return [3]float64{}
	}

	var avg [3]float64
	for i := 0; i < 3; i++ {
		avg[i], _ = strconv.ParseFloat(fields[i], 64)
	}
	return avg
}

func getUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}
	secs, _ := strconv.ParseFloat(fields[0], 64)
	return int64(secs)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	entries := history.GetAll()
	if len(entries) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "collecting"})
		return
	}

	latest := entries[len(entries)-1]
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(latest)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	entries := history.GetAll()
	latest := SystemStats{}
	if len(entries) > 0 {
		latest = entries[len(entries)-1]
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"module":      "system_health",
		"status":      "running",
		"data_points": len(entries),
		"latest":      latest,
		"module_mem":  mem.Alloc,
	})
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history.GetAll())
}

func handleSparkline(w http.ResponseWriter, r *http.Request) {
	field := r.URL.Query().Get("field")
	if field == "" {
		field = "cpu"
	}
	pointsStr := r.URL.Query().Get("points")
	points := 60
	if p, err := strconv.Atoi(pointsStr); err == nil && p > 0 {
		points = p
	}

	data := history.GetSparkline(field, points)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"field":  field,
		"points": data,
	})
	_ = fmt.Sprintf("") // suppress unused import
}
