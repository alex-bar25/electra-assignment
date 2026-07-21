package api

import (
	"log/slog"
	"net/http"

	"electra-assignment/internal/service"
)

type handler struct {
	station *service.Service
	logger  *slog.Logger
}

func New(station *service.Service, logger *slog.Logger) http.Handler {
	api := handler{station: station, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", api.health)
	mux.HandleFunc("PUT /api/v1/station/config", api.configureStation)
	mux.HandleFunc("GET /api/v1/station", api.getStation)
	mux.HandleFunc("POST /api/v1/sessions", api.startSession)
	mux.HandleFunc("PATCH /api/v1/sessions/{sessionId}", api.updateSession)
	mux.HandleFunc("DELETE /api/v1/sessions/{sessionId}", api.stopSession)
	return mux
}
