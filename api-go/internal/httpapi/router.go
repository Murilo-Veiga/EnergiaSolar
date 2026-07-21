package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"energiasolar-api/internal/auth"
)

func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		// Libera qualquer origem. Como a sessão usa cookie com
		// credentials:"include", um wildcard literal "*" não funciona (o
		// browser recusa credentials com Access-Control-Allow-Origin: "*"),
		// então refletimos a origem da própria requisição via AllowOriginFunc.
		AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	}))

	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)
	})

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(s.JWTSecret))
		r.Get("/api/me", s.handleGetMe)
		r.Put("/api/me", s.handleUpdateProfile)
		r.Put("/api/me/password", s.handleUpdatePassword)
		r.Route("/api/admin/users", func(r chi.Router) {
			r.Use(s.RequireAdmin)
			r.Get("/", s.handleAdminListUsers)
			r.Post("/", s.handleAdminCreateUser)
			r.Get("/{userID}", s.handleAdminGetUser)
			r.Put("/{userID}", s.handleAdminUpdateUser)
			r.Put("/{userID}/password", s.handleAdminResetPassword)
			r.Delete("/{userID}", s.handleAdminDeleteUser)
		})
		r.Route("/api/admin/system-settings", func(r chi.Router) {
			r.Use(s.RequireAdmin)
			r.Get("/", s.handleGetSystemSettings)
			r.Put("/", s.handleUpdateSystemSettings)
		})
		r.Get("/api/plants", s.handleListPlants)
		r.Post("/api/plants", s.handleCreatePlant)
		r.Route("/api/plants/{plantID}", func(r chi.Router) {
			r.Get("/", s.handleGetPlant)
			r.Put("/", s.handleUpdatePlant)
			r.Delete("/", s.handleDeletePlant)
			r.Get("/summary", s.handleSummary)
			r.Get("/inverters", s.handleInverters)
			r.Get("/collector-health", s.handleCollectorHealth)
			r.Get("/history", s.handleHistory)
			r.Get("/history/records", s.handleHistoryRecords)
			r.Get("/history/inverters", s.handleHistoryInverters)
			r.Get("/annotations", s.handleListAnnotations)
			r.Post("/annotations", s.handleCreateAnnotation)
			r.Post("/consumption/upload", s.handleUploadConsumption)
			r.Get("/consumption/summary", s.handleConsumptionSummary)
			r.Get("/consumption/history", s.handleConsumptionHistory)
			r.Get("/day-status", s.handleDayStatus)
			r.Get("/forecast", s.handleForecast)
			r.Get("/inverters-config", s.handleListInverterCredentials)
			r.Post("/inverters-config", s.handleCreateInverterCredential)
			r.Post("/inverters-config/test", s.handleTestInverterCredential)
			r.Put("/inverters-config/{credID}", s.handleUpdateInverterCredential)
			r.Delete("/inverters-config/{credID}", s.handleDeleteInverterCredential)
			r.Get("/access", s.handleListPlantAccess)
			r.Post("/access", s.handleGrantPlantAccess)
			r.Delete("/access/{userID}", s.handleRevokePlantAccess)
		})
	})

	return r
}
