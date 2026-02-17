package api

import (
	"net"
	"net/http"
	"strings"

	"github.com/SupperTomato/openclaw_dashboard/backend/internal/config"
	"github.com/SupperTomato/openclaw_dashboard/backend/internal/modules"
)

// Router sets up all HTTP routes
type Router struct {
	mux       *http.ServeMux
	configMgr *config.ConfigManager
	modMgr    *modules.Manager
	staticDir string
	tmplDir   string
}

// NewRouter creates a new router
func NewRouter(configMgr *config.ConfigManager, modMgr *modules.Manager, staticDir, tmplDir string) *Router {
	r := &Router{
		mux:       http.NewServeMux(),
		configMgr: configMgr,
		modMgr:    modMgr,
		staticDir: staticDir,
		tmplDir:   tmplDir,
	}
	r.setupRoutes()
	return r
}

func (r *Router) setupRoutes() {
	h := NewHandlers(r.configMgr, r.modMgr, r.tmplDir)

	// Static files
	fs := http.FileServer(http.Dir(r.staticDir))
	r.mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Pages (server-rendered HTML)
	r.mux.HandleFunc("/", h.DashboardPage)
	r.mux.HandleFunc("/config", h.ConfigPage)

	// API routes
	r.mux.HandleFunc("/api/status", h.APIStatus)
	r.mux.HandleFunc("/api/modules", h.APIModules)
	r.mux.HandleFunc("/api/modules/", h.APIModuleAction)
	r.mux.HandleFunc("/api/config", h.APIConfig)
	r.mux.HandleFunc("/api/config/descriptions", h.APIConfigDescriptions)
	r.mux.HandleFunc("/api/system", h.APISystem)

	// SSE endpoint
	r.mux.HandleFunc("/api/events", h.SSEHandler)

	// Module proxy - forward requests to modules
	r.mux.HandleFunc("/module/", h.ModuleProxy)

	// htmx partial endpoints
	r.mux.HandleFunc("/partial/modules", h.PartialModules)
	r.mux.HandleFunc("/partial/system", h.PartialSystem)
	r.mux.HandleFunc("/partial/module/", h.PartialModuleContent)
}

// Handler returns the HTTP handler with middleware
func (r *Router) Handler() http.Handler {
	cfg := r.configMgr.Get()

	var handler http.Handler = r.mux

	// LAN-only middleware
	if cfg.Security.LANOnly {
		handler = lanOnlyMiddleware(handler, cfg.Security.AllowedNetworks)
	}

	// Logging middleware
	handler = loggingMiddleware(handler)

	// CORS for LAN
	handler = corsMiddleware(handler)

	return handler
}

func lanOnlyMiddleware(next http.Handler, allowedNetworks []string) http.Handler {
	var nets []*net.IPNet
	for _, cidr := range allowedNetworks {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, ipnet)
		}
	}
	// Always allow localhost
	_, lo4, _ := net.ParseCIDR("127.0.0.0/8")
	_, lo6, _ := net.ParseCIDR("::1/128")
	nets = append(nets, lo4, lo6)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		allowed := false
		for _, n := range nets {
			if n.Contains(ip) {
				allowed = true
				break
			}
		}

		if !allowed {
			http.Error(w, "Access denied. Dashboard is LAN-only.", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip logging for static files and SSE keepalive
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			// Lightweight logging - only log API calls
			if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/" {
				// Could log here if needed
			}
		}
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
