package server

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"microapi/internal/config"
	"microapi/internal/handlers"
	mw "microapi/internal/middleware"
)

type Server struct {
	*chi.Mux
}

func New(cfg *config.Config, db *sql.DB) *Server {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(mw.Logger)
	r.Use(mw.LimitBody(cfg.MaxRequestSize))
	r.Use(mw.CORS(cfg.CORSOrigins))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		mw.WriteJSON(w, http.StatusOK, true, map[string]string{"status": "ok"}, nil)
	})

	// Register API routes
	h := handlers.New(db, cfg)
	r.Route("/", func(r chi.Router) {
		// Document routes
		r.Post("/{set}/{collection}", h.CreateDocument)
		r.Get("/{set}/{collection}", h.QueryCollection)
		r.Get("/{set}/{collection}/{id}", h.GetDocument)
		r.Put("/{set}/{collection}/{id}", h.ReplaceDocument)
		r.Patch("/{set}/{collection}/{id}", h.UpdateDocument)
		r.Delete("/{set}/{collection}/{id}", h.DeleteDocument)
		r.Delete("/{set}/{collection}", h.DeleteCollection)
		// Set routes
		r.Get("/{set}", h.GetSetStats)
		r.Delete("/{set}", h.DeleteSet)
		// Utility
		r.Get("/_sets", h.ListSets)
	})

	// Dashboard fallback at root
	r.Get("/", h.Dashboard)
	r.Get("/style.css", h.DashboardCSS)
	r.Get("/favicon.ico", h.DashboardFavicon)

	return &Server{Mux: r}
}

func (s *Server) Shutdown(ctx context.Context) error {
	// nothing significant to close here, but allow future hooks
	slog.Info("shutdown server", slog.String("at", time.Now().Format(time.RFC3339)))
	return nil
}

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ValidName exported for reuse
func ValidName(name string) bool { return nameRe.MatchString(name) }
