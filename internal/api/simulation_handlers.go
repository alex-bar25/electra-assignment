package api

import (
	"errors"
	"net/http"

	"electra-assignment/internal/service"
)

type advanceSimulationRequest struct {
	ElapsedSeconds float64 `json:"elapsedSeconds"`
}

func (api handler) advanceSimulation(response http.ResponseWriter, request *http.Request) {
	var body advanceSimulationRequest
	if err := decodeJSON(request, &body); err != nil {
		api.logger.Warn("reject malformed simulation tick", "error", err)
		api.writeError(response, http.StatusBadRequest, "invalid_tick", "request body must contain one valid simulation tick")
		return
	}

	state, err := api.station.AdvanceSimulation(body.ElapsedSeconds)
	if err != nil {
		api.writeSimulationError(response, err)
		return
	}

	api.logger.Info("simulation advanced", "elapsed_seconds", body.ElapsedSeconds, "bess_soc_percent", state.BESS.SocPercent)
	api.writeJSON(response, http.StatusOK, state)
}

func (api handler) writeSimulationError(response http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "an unexpected error occurred"

	switch {
	case errors.Is(err, service.ErrInvalidSimulationDuration):
		status = http.StatusBadRequest
		code = "invalid_tick"
		message = err.Error()
	case errors.Is(err, service.ErrStationNotConfigured):
		status = http.StatusNotFound
		code = "station_not_configured"
		message = err.Error()
	case errors.Is(err, service.ErrBESSNotConfigured):
		status = http.StatusNotFound
		code = "bess_not_configured"
		message = err.Error()
	}

	if status == http.StatusInternalServerError {
		api.logger.Error("advance simulation", "error", err)
	} else {
		api.logger.Warn("reject simulation tick", "code", code, "error", err)
	}
	api.writeError(response, status, code, message)
}
