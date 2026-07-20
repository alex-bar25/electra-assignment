package api

import (
	"errors"
	"net/http"

	"electra-assignment/internal/domain"
	"electra-assignment/internal/service"
)

type startSessionRequest struct {
	ID                   string   `json:"id"`
	ConnectorID          string   `json:"connectorId"`
	RequestedPowerKw     float64  `json:"requestedPowerKw"`
	VehicleMaxPowerKw    float64  `json:"vehicleMaxPowerKw"`
	ChargingCurveLimitKw *float64 `json:"chargingCurveLimitKw,omitempty"`
	MinimumPowerKw       float64  `json:"minimumPowerKw,omitempty"`
}

func (api handler) startSession(response http.ResponseWriter, request *http.Request) {
	var body startSessionRequest
	if err := decodeJSON(request, &body); err != nil {
		api.logger.Warn("reject malformed session start", "error", err)
		api.writeError(response, http.StatusBadRequest, "invalid_request", "request body must contain one valid session")
		return
	}

	session, err := api.station.StartSession(domain.Session{
		ID:                   body.ID,
		ConnectorID:          body.ConnectorID,
		RequestedPowerKw:     body.RequestedPowerKw,
		VehicleMaxPowerKw:    body.VehicleMaxPowerKw,
		ChargingCurveLimitKw: body.ChargingCurveLimitKw,
		MinimumPowerKw:       body.MinimumPowerKw,
	})
	if err != nil {
		api.writeStartSessionError(response, err)
		return
	}

	api.logger.Info("session started", "session_id", session.ID, "connector_id", session.ConnectorID)
	api.writeJSON(response, http.StatusCreated, session)
}

func (api handler) writeStartSessionError(response http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := "invalid_session"

	switch {
	case errors.Is(err, service.ErrStationNotConfigured):
		status = http.StatusNotFound
		code = "station_not_configured"
	case errors.Is(err, service.ErrConnectorNotFound):
		status = http.StatusNotFound
		code = "connector_not_found"
	case errors.Is(err, service.ErrDuplicateSession):
		status = http.StatusConflict
		code = "duplicate_session"
	case errors.Is(err, service.ErrConnectorOccupied):
		status = http.StatusConflict
		code = "connector_occupied"
	case errors.Is(err, service.ErrHardwareUnavailable):
		status = http.StatusConflict
		code = "hardware_unavailable"
	}

	api.logger.Warn("reject session start", "code", code, "error", err)
	api.writeError(response, status, code, err.Error())
}
