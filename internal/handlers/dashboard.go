package handlers

import (
	"compress/gzip"
	"net/http"
	"os"

	"microapi/internal/middleware"
	"microapi/internal/models"
	"microapi/web"
)

func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error
	if h.cfg.DevMode {
		// filesystem path in dev for hot reload
		data, err = os.ReadFile("web/static/dashboard.html")
	} else {
		data, err = web.FS.ReadFile("static/dashboard.html")
	}
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr("dashboard not found"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Content-Encoding", "gzip")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	_, _ = gz.Write(data)
}

func (h *Handlers) DashboardCSS(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error
	if h.cfg.DevMode {
		// filesystem path in dev for hot reload
		data, err = os.ReadFile("web/static/style.css")
	} else {
		data, err = web.FS.ReadFile("static/style.css")
	}
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr("style.css not found"))
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Content-Encoding", "gzip")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	_, _ = gz.Write(data)
}

func (h *Handlers) DashboardFavicon(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error
	if h.cfg.DevMode {
		// filesystem path in dev for hot reload
		data, err = os.ReadFile("web/static/favicon.ico")
	} else {
		data, err = web.FS.ReadFile("static/favicon.ico")
	}
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr("favicon.ico not found"))
		return
	}
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Content-Encoding", "gzip")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	_, _ = gz.Write(data)
}

func (h *Handlers) DashboardLogo(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error
	if h.cfg.DevMode {
		// filesystem path in dev for hot reload
		data, err = os.ReadFile("web/static/logo.svg")
	} else {
		data, err = web.FS.ReadFile("static/logo.svg")
	}
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr("logo.svg not found"))
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Content-Encoding", "gzip")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	_, _ = gz.Write(data)
}
