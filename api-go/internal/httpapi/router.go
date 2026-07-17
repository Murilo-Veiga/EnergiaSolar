package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"energiasolar-api/internal/auth"
)

func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/signup", s.handleSignup)
		r.Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)
	})

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(s.JWTSecret))
		r.Get("/api/plants", s.handleListPlants)
		r.Route("/api/plants/{plantID}", func(r chi.Router) {
			r.Get("/", s.handleGetPlant)
			r.Get("/summary", s.handleSummary)
			r.Get("/inverters", s.handleInverters)
			r.Get("/collector-health", s.handleCollectorHealth)
			r.Get("/history", s.handleHistory)
			r.Get("/history/records", s.handleHistoryRecords)
			r.Get("/history/inverters", s.handleHistoryInverters)
			r.Get("/annotations", s.handleListAnnotations)
			r.Post("/annotations", s.handleCreateAnnotation)
		})
	})

	return r
}
