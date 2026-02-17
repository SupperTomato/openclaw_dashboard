package api

import (
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/SupperTomato/openclaw_dashboard/backend/internal/config"
	"github.com/SupperTomato/openclaw_dashboard/backend/internal/modules"
)

type API struct {
	mux *http.ServeMux
	mod *modules.Manager
	cfg *config.Config
	dir map[string]string
}

func NewRouter(staticDir, tmplDir string) *API {
	return &API{
		mux: http.NewServeMux(),
		dir: map[string]string{"static": staticDir, "template": tmplDir},
	}
}

func (a *API) Handler(mod *modules.Manager, cfg *config.Config) http.Handler {
	a.mod = mod
	a.cfg = cfg
	a.routes()
	return lanOnly(a.mux, cfg.Security.AllowedNetworks)
}

func (a *API) routes() {
	a.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(a.dir["static"]))))
	a.mux.HandleFunc("/setup", a.setup)
	a.mux.HandleFunc("/", a.dashboard)
	a.mux.HandleFunc("/module/", a.modulePage)
	a.mux.HandleFunc("/config", a.configPage)
	a.mux.HandleFunc("/api/modules", a.apiModules)
	a.mux.HandleFunc("/api/modules/", a.apiModuleCtrl)
	a.mux.HandleFunc("/api/config", a.apiConfig)
	a.mux.HandleFunc("/api/config/save", a.apiConfigSave)
	a.mux.HandleFunc("/api/module-proxy/", a.apiModuleProxy)
}

func (a *API) setup(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		root := r.FormValue("root")
		if _, err := os.Stat(root); err == nil {
			config.SetupRoot(root)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		} else {
			w.Write([]byte("Invalid directory"))
		}
		return
	}
	a.tpl(w, "setup.html", nil)
}

func (a *API) dashboard(w http.ResponseWriter, r *http.Request) {
	if !config.IsSetup() {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	mods := a.mod.GetStatus()
	a.tpl(w, "dashboard.html", map[string]any{
		"Modules": mods,
		"Title":   a.cfg.Dashboard.Title,
		"Theme":   a.cfg.Dashboard.Theme,
	})
}

func (a *API) modulePage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/module/")
	status, err := a.mod.GetModuleStatus(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.tpl(w, "module_"+id+".html", status)
}

func (a *API) configPage(w http.ResponseWriter, r *http.Request) {
	raw, _ := config.GetRaw()
	a.tpl(w, "config.html", map[string]any{
		"Config":  string(raw),
		"Descs":   config.GetDescriptions(),
		"ReadOnly": !config.IsSetup(),
	})
}

func (a *API) apiModules(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a.mod.GetStatus())
}

func (a *API) apiModuleCtrl(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/modules/"), "/")
	id, action := parts[0], ""
	if len(parts) > 1 {
		action = parts[1]
	}

	if r.Method == "GET" {
		s, _ := a.mod.GetModuleStatus(id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var err error
	switch action {
	case "start":
		err = a.mod.StartModule(id)
	case "stop":
		err = a.mod.StopModule(id)
	case "restart":
		err = a.mod.RestartModule(id)
	}

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "err": err.Error()})
	} else {
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}

func (a *API) apiConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw, _ := config.GetRaw()
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

func (a *API) apiConfigSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	err := config.Save(json.RawMessage(body))
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "err": err.Error()})
	} else {
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}

func (a *API) apiModuleProxy(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/module-proxy/"), "/")
	if len(parts) < 1 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	id := parts[0]
	a.mod.ProxyToModule(id, "/"+strings.Join(parts[1:], "/"), w, r)
}

func (a *API) tpl(w http.ResponseWriter, name string, data any) {
	path := filepath.Join(a.dir["template"], name)
	t, err := template.ParseFiles(path)
	if err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t.Execute(w, data)
}

func lanOnly(h http.Handler, nets []string) http.Handler {
	var ipnets []*net.IPNet
	for _, cidr := range nets {
		_, n, _ := net.ParseCIDR(cidr)
		if n != nil {
			ipnets = append(ipnets, n)
		}
	}
	_, lo4, _ := net.ParseCIDR("127.0.0.0/8")
	_, lo6, _ := net.ParseCIDR("::1/128")
	ipnets = append(ipnets, lo4, lo6)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if ip == nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		for _, n := range ipnets {
			if n.Contains(ip) {
				h.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "Access denied", http.StatusForbidden)
	})
}
