package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"electra-assignment/internal/domain"
	"electra-assignment/internal/service"
)

type handler struct {
	station *service.Service
	logger  *slog.Logger
}

type healthResponse struct {
	Status string `json:"status"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(station *service.Service, logger *slog.Logger) http.Handler {
	api := handler{station: station, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", api.health)
	mux.HandleFunc("PUT /api/v1/station/config", api.configureStation)
	mux.HandleFunc("GET /api/v1/station", api.getStation)
	return mux
}

func (api handler) configureStation(response http.ResponseWriter, request *http.Request) {
	var config domain.StationConfig
	if err := decodeJSON(request, &config); err != nil {
		api.logger.Warn("reject malformed station configuration", "error", err)
		api.writeError(response, http.StatusBadRequest, "invalid_request", "request body must contain one valid station configuration")
		return
	}
	if err := api.station.Configure(config); err != nil {
		api.logger.Warn("reject invalid station configuration", "error", err)
		api.writeError(response, http.StatusBadRequest, "invalid_station_config", err.Error())
		return
	}

	state, err := api.station.Snapshot()
	if err != nil {
		api.logger.Error("get state after station configuration", "error", err)
		api.writeError(response, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
		return
	}
	api.logger.Info("station configured", "station_id", config.ID)
	api.writeJSON(response, http.StatusOK, state)
}

func (api handler) health(response http.ResponseWriter, _ *http.Request) {
	api.writeJSON(response, http.StatusOK, healthResponse{Status: "ok"})
}

func (api handler) getStation(response http.ResponseWriter, _ *http.Request) {
	state, err := api.station.Snapshot()
	if errors.Is(err, service.ErrStationNotConfigured) {
		api.logger.Warn("station state requested before configuration")
		api.writeError(response, http.StatusNotFound, "station_not_configured", err.Error())
		return
	}
	if err != nil {
		api.logger.Error("get station state", "error", err)
		api.writeError(response, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
		return
	}
	api.writeJSON(response, http.StatusOK, state)
}

func (api handler) writeError(response http.ResponseWriter, status int, code, message string) {
	api.writeJSON(response, status, errorResponse{Code: code, Message: message})
}

func decodeJSON(request *http.Request, target any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func (api handler) writeJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(body); err != nil {
		api.logger.Error("encode response", "error", err)
	}
}
