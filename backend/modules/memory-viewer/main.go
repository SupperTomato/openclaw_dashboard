package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

type MemoryFile struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
	Exists   bool   `json:"exists"`
}

var (
	memoryDir  string
	watchFiles []string
)

func main() {
	port := os.Getenv("MODULE_PORT")
	if port == "" {
		port = "9008"
	}

	memoryDir = expandPath("~/.claude")
	watchFiles = []string{"MEMORY.md", "HEARTBEAT.md"}

	if cfg := os.Getenv("MODULE_CONFIG"); cfg != "" {
		var c map[string]interface{}
		if json.Unmarshal([]byte(cfg), &c) == nil {
			if md, ok := c["memory_dir"].(string); ok {
				memoryDir = expandPath(md)
			}
			if wf, ok := c["watch_files"].([]interface{}); ok {
				watchFiles = nil
				for _, f := range wf {
					if s, ok := f.(string); ok {
						watchFiles = append(watchFiles, s)
					}
				}
			}
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/files", handleFiles)
	mux.HandleFunc("/file/", handleFile)
	mux.HandleFunc("/scan", handleScan)

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

	log.Printf("[memory-viewer] Starting on port %s, dir: %s", port, memoryDir)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[memory-viewer] Server error: %v", err)
	}
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, _ := os.UserHomeDir()
		if p == "~" {
			return home
		}
		return filepath.Join(home, p[2:])
	}
	return os.ExpandEnv(p)
}

func getWatchedFiles() []MemoryFile {
	var files []MemoryFile
	for _, name := range watchFiles {
		path := filepath.Join(memoryDir, name)
		mf := MemoryFile{
			Name: name,
			Path: path,
		}

		info, err := os.Stat(path)
		if err != nil {
			mf.Exists = false
		} else {
			mf.Exists = true
			mf.Size = info.Size()
			mf.ModTime = info.ModTime().Format("2006-01-02 15:04:05")

			data, err := os.ReadFile(path)
			if err == nil {
				mf.Content = string(data)
			}
		}

		files = append(files, mf)
	}
	return files
}

func scanMemoryDir() []MemoryFile {
	var files []MemoryFile

	filepath.Walk(memoryDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// Only include markdown and text files
		ext := filepath.Ext(path)
		if ext != ".md" && ext != ".txt" {
			return nil
		}

		relPath, _ := filepath.Rel(memoryDir, path)
		files = append(files, MemoryFile{
			Name:    relPath,
			Path:    path,
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			Exists:  true,
		})
		return nil
	})

	return files
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	files := getWatchedFiles()
	existing := 0
	for _, f := range files {
		if f.Exists {
			existing++
		}
	}
	writeJSON(w, map[string]interface{}{
		"watched_files": len(watchFiles),
		"existing":      existing,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"module": "memory_viewer",
		"status": "running",
		"dir":    memoryDir,
	})
}

func handleFiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, getWatchedFiles())
}

func handleFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/file/")
	path := filepath.Join(memoryDir, name)

	// Security: ensure path is within memoryDir
	absPath, _ := filepath.Abs(path)
	absDir, _ := filepath.Abs(memoryDir)
	if !strings.HasPrefix(absPath, absDir) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, MemoryFile{
		Name:    name,
		Path:    path,
		Content: string(data),
		Size:    info.Size(),
		ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		Exists:  true,
	})
}

func handleScan(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, scanMemoryDir())
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
