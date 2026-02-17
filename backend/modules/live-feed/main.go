package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type FeedMessage struct {
	ID        int       `json:"id"`
	Source    string    `json:"source"`
	Content  string    `json:"content"`
	Type     string    `json:"type"` // user, assistant, system, tool
	Session  string    `json:"session"`
	Timestamp time.Time `json:"timestamp"`
}

type FeedBuffer struct {
	mu       sync.RWMutex
	messages []FeedMessage
	maxSize  int
	nextID   int
}

func NewFeedBuffer(maxSize int) *FeedBuffer {
	return &FeedBuffer{maxSize: maxSize}
}

func (b *FeedBuffer) Add(msg FeedMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	msg.ID = b.nextID
	b.messages = append(b.messages, msg)
	if len(b.messages) > b.maxSize {
		b.messages = b.messages[len(b.messages)-b.maxSize:]
	}
}

func (b *FeedBuffer) GetSince(afterID int) []FeedMessage {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var result []FeedMessage
	for _, m := range b.messages {
		if m.ID > afterID {
			result = append(result, m)
		}
	}
	return result
}

func (b *FeedBuffer) GetRecent(count int) []FeedMessage {
	b.mu.RLock()
	defer b.mu.RUnlock()
	start := len(b.messages) - count
	if start < 0 {
		start = 0
	}
	result := make([]FeedMessage, len(b.messages)-start)
	copy(result, b.messages[start:])
	return result
}

var buffer *FeedBuffer

func main() {
	port := os.Getenv("MODULE_PORT")
	if port == "" {
		port = "9003"
	}

	maxMessages := 500
	if cfg := os.Getenv("MODULE_CONFIG"); cfg != "" {
		var c map[string]interface{}
		if json.Unmarshal([]byte(cfg), &c) == nil {
			if mm, ok := c["max_messages"].(float64); ok {
				maxMessages = int(mm)
			}
		}
	}

	buffer = NewFeedBuffer(maxMessages)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/messages", handleMessages)
	mux.HandleFunc("/post", handlePost)
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

	log.Printf("[live-feed] Starting on port %s", port)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[live-feed] Server error: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"total_messages": len(buffer.GetRecent(buffer.maxSize)),
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"module": "live_feed",
		"status": "running",
	})
}

func handleMessages(w http.ResponseWriter, r *http.Request) {
	afterStr := r.URL.Query().Get("after")
	if afterStr != "" {
		afterID, _ := strconv.Atoi(afterStr)
		writeJSON(w, buffer.GetSince(afterID))
		return
	}

	countStr := r.URL.Query().Get("count")
	count := 50
	if c, err := strconv.Atoi(countStr); err == nil && c > 0 {
		count = c
	}
	writeJSON(w, buffer.GetRecent(count))
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg FeedMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	msg.Timestamp = time.Now()
	buffer.Add(msg)
	writeJSON(w, map[string]interface{}{"success": true, "id": msg.ID})
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
	lastID := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			msgs := buffer.GetSince(lastID)
			if len(msgs) > 0 {
				data, _ := json.Marshal(msgs)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
				lastID = msgs[len(msgs)-1].ID
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
