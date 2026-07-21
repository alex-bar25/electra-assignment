package api

import (
	"errors"
	"net/http"

	"electra-assignment/internal/domain"
	"electra-assignment/internal/service"
)

type availabilityUpdateRequest struct {
	Status domain.OperationalStatus `json:"status"`
}

func (api handler) updateChargerAvailability(response http.ResponseWriter, request *http.Request) {
	var body availabilityUpdateRequest
	if err := decodeJSON(request, &body); err != nil {
		api.logger.Warn("reject malformed charger availability update", "error", err)
		api.writeError(response, http.StatusBadRequest, "invalid_request", "request body must contain one valid availability update")
		return
	}

	chargerID := request.PathValue("chargerId")
	state, err := api.station.UpdateChargerStatus(chargerID, body.Status)
	if err != nil {
		api.writeAvailabilityError(response, "charger", chargerID, err)
		return
	}

	api.logger.Info("charger availability updated", "charger_id", chargerID, "status", body.Status)
	api.writeJSON(response, http.StatusOK, state)
}

func (api handler) updateConnectorAvailability(response http.ResponseWriter, request *http.Request) {
	var body availabilityUpdateRequest
	if err := decodeJSON(request, &body); err != nil {
		api.logger.Warn("reject malformed connector availability update", "error", err)
		api.writeError(response, http.StatusBadRequest, "invalid_request", "request body must contain one valid availability update")
		return
	}

	connectorID := request.PathValue("connectorId")
	state, err := api.station.UpdateConnectorStatus(connectorID, body.Status)
	if err != nil {
		api.writeAvailabilityError(response, "connector", connectorID, err)
		return
	}

	api.logger.Info("connector availability updated", "connector_id", connectorID, "status", body.Status)
	api.writeJSON(response, http.StatusOK, state)
}

func (api handler) writeAvailabilityError(response http.ResponseWriter, hardwareType, hardwareID string, err error) {
	status := http.StatusBadRequest
	code := "invalid_hardware_status"

	switch {
	case errors.Is(err, service.ErrStationNotConfigured):
		status = http.StatusNotFound
		code = "station_not_configured"
	case errors.Is(err, service.ErrChargerNotFound):
		status = http.StatusNotFound
		code = "charger_not_found"
	case errors.Is(err, service.ErrConnectorNotFound):
		status = http.StatusNotFound
		code = "connector_not_found"
	}

	api.logger.Warn("reject hardware availability update", "hardware_type", hardwareType, "hardware_id", hardwareID, "code", code, "error", err)
	api.writeError(response, status, code, err.Error())
}
