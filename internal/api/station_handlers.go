package api

import (
	"errors"
	"net/http"

	"electra-assignment/internal/domain"
	"electra-assignment/internal/service"
)

func (api handler) configureStation(response http.ResponseWriter, request *http.Request) {
	var config domain.StationConfig
	if err := decodeJSON(request, &config); err != nil {
		api.logger.Warn("reject malformed station configuration", "error", err)
		api.writeError(response, http.StatusBadRequest, "invalid_request", "request body must contain one valid station configuration")
		return
	}
	state, err := api.station.Configure(config)
	if err != nil {
		api.logger.Warn("reject invalid station configuration", "error", err)
		api.writeError(response, http.StatusBadRequest, "invalid_station_config", err.Error())
		return
	}
	api.logger.Info("station configured", "station_id", config.ID)
	api.writeJSON(response, http.StatusOK, state)
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
