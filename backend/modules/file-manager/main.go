package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

var (
	rootDir     string
	allowedExts map[string]bool
	maxFileSize int64
	showHidden  bool
)

func main() {
	port := os.Getenv("MODULE_PORT")
	if port == "" {
		port = "9005"
	}

	rootDir = expandPath("~")
	allowedExts = map[string]bool{
		".md": true, ".txt": true, ".json": true, ".yaml": true, ".yml": true,
		".toml": true, ".cfg": true, ".conf": true, ".log": true,
		".sh": true, ".py": true, ".js": true, ".go": true,
	}
	maxFileSize = 1024 * 1024 // 1MB
	showHidden = false

	if cfg := os.Getenv("MODULE_CONFIG"); cfg != "" {
		var c map[string]interface{}
		if json.Unmarshal([]byte(cfg), &c) == nil {
			if rd, ok := c["root_dir"].(string); ok {
				rootDir = expandPath(rd)
			}
			if mfs, ok := c["max_file_size_kb"].(float64); ok {
				maxFileSize = int64(mfs) * 1024
			}
			if sh, ok := c["show_hidden"].(bool); ok {
				showHidden = sh
			}
			if exts, ok := c["allowed_extensions"].([]interface{}); ok {
				allowedExts = make(map[string]bool)
				for _, e := range exts {
					if s, ok := e.(string); ok {
						allowedExts[s] = true
					}
				}
			}
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/list", handleList)
	mux.HandleFunc("/read", handleRead)
	mux.HandleFunc("/write", handleWrite)

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

	log.Printf("[file-manager] Starting on port %s, root: %s", port, rootDir)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("[file-manager] Server error: %v", err)
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

// sanitizePath ensures the path is within rootDir (prevent directory traversal)
func sanitizePath(p string) (string, error) {
	// Clean the path
	cleaned := filepath.Clean(p)
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join(rootDir, cleaned)
	}

	// Resolve symlinks and get absolute path
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", err
	}

	// Ensure it's within rootDir
	rootAbs, _ := filepath.Abs(rootDir)
	if !strings.HasPrefix(abs, rootAbs) {
		return "", os.ErrPermission
	}

	return abs, nil
}

func isAllowedFile(name string) bool {
	ext := filepath.Ext(name)
	return allowedExts[ext]
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"root_dir":    rootDir,
		"show_hidden": showHidden,
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"module": "file_manager",
		"status": "running",
		"root":   rootDir,
	})
}

func handleList(w http.ResponseWriter, r *http.Request) {
	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		dirPath = rootDir
	}

	safe, err := sanitizePath(dirPath)
	if err != nil {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	entries, err := os.ReadDir(safe)
	if err != nil {
		http.Error(w, "cannot read directory", http.StatusInternalServerError)
		return
	}

	var files []FileInfo
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files if not enabled
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		fi := FileInfo{
			Name:    name,
			Path:    filepath.Join(safe, name),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		}

		// Only include allowed file types (or directories)
		if entry.IsDir() || isAllowedFile(name) {
			files = append(files, fi)
		}
	}

	writeJSON(w, map[string]interface{}{
		"path":    safe,
		"parent":  filepath.Dir(safe),
		"files":   files,
		"is_root": safe == rootDir,
	})
}

func handleRead(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	safe, err := sanitizePath(filePath)
	if err != nil {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	if !isAllowedFile(filepath.Base(safe)) {
		http.Error(w, "file type not allowed", http.StatusForbidden)
		return
	}

	info, err := os.Stat(safe)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	if info.Size() > maxFileSize {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}

	data, err := os.ReadFile(safe)
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"path":    safe,
		"name":    filepath.Base(safe),
		"content": string(data),
		"size":    info.Size(),
	})
}

func handleWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" && r.Method != "PUT" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxFileSize+1024))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	safe, err := sanitizePath(req.Path)
	if err != nil {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	if !isAllowedFile(filepath.Base(safe)) {
		http.Error(w, "file type not allowed", http.StatusForbidden)
		return
	}

	if err := os.WriteFile(safe, []byte(req.Content), 0644); err != nil {
		http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"path":    safe,
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
