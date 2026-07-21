package api

import (
	"encoding/json"
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

type updateSessionRequest struct {
	RequestedPowerKw     *float64        `json:"requestedPowerKw"`
	VehicleMaxPowerKw    *float64        `json:"vehicleMaxPowerKw"`
	ChargingCurveLimitKw optionalFloat64 `json:"chargingCurveLimitKw"`
	MinimumPowerKw       *float64        `json:"minimumPowerKw"`
}

type optionalFloat64 struct {
	value *float64
	set   bool
}

func (field *optionalFloat64) UnmarshalJSON(data []byte) error {
	field.set = true
	return json.Unmarshal(data, &field.value)
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
		api.writeSessionError(response, "start", err)
		return
	}

	api.logger.Info("session started", "session_id", session.ID, "connector_id", session.ConnectorID)
	api.writeJSON(response, http.StatusCreated, session)
}

func (api handler) updateSession(response http.ResponseWriter, request *http.Request) {
	var body updateSessionRequest
	if err := decodeJSON(request, &body); err != nil {
		api.logger.Warn("reject malformed session update", "error", err)
		api.writeError(response, http.StatusBadRequest, "invalid_request", "request body must contain one valid session update")
		return
	}
	if body.RequestedPowerKw == nil && body.VehicleMaxPowerKw == nil &&
		!body.ChargingCurveLimitKw.set && body.MinimumPowerKw == nil {
		api.logger.Warn("reject empty session update")
		api.writeError(response, http.StatusBadRequest, "invalid_request", "at least one session power limit must be provided")
		return
	}

	session, err := api.station.UpdateSession(request.PathValue("sessionId"), service.SessionUpdate{
		RequestedPowerKw:        body.RequestedPowerKw,
		VehicleMaxPowerKw:       body.VehicleMaxPowerKw,
		ChargingCurveLimitKw:    body.ChargingCurveLimitKw.value,
		ClearChargingCurveLimit: body.ChargingCurveLimitKw.set && body.ChargingCurveLimitKw.value == nil,
		MinimumPowerKw:          body.MinimumPowerKw,
	})
	if err != nil {
		api.writeSessionError(response, "update", err)
		return
	}

	api.logger.Info("session updated", "session_id", session.ID)
	api.writeJSON(response, http.StatusOK, session)
}

func (api handler) stopSession(response http.ResponseWriter, request *http.Request) {
	state, err := api.station.StopSession(request.PathValue("sessionId"))
	if err != nil {
		api.writeSessionError(response, "stop", err)
		return
	}

	api.logger.Info("session stopped", "session_id", request.PathValue("sessionId"))
	api.writeJSON(response, http.StatusOK, state)
}

func (api handler) writeSessionError(response http.ResponseWriter, operation string, err error) {
	status := http.StatusBadRequest
	code := "invalid_session"

	switch {
	case errors.Is(err, service.ErrStationNotConfigured):
		status = http.StatusNotFound
		code = "station_not_configured"
	case errors.Is(err, service.ErrConnectorNotFound):
		status = http.StatusNotFound
		code = "connector_not_found"
	case errors.Is(err, service.ErrSessionNotFound):
		status = http.StatusNotFound
		code = "session_not_found"
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

	api.logger.Warn("reject session request", "operation", operation, "code", code, "error", err)
	api.writeError(response, status, code, err.Error())
}
