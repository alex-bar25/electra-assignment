package service

import (
	"time"

	"electra-assignment/internal/domain"
)

func (service *Service) UpdateChargerStatus(chargerID string, status domain.OperationalStatus) (StationState, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.config == nil {
		return StationState{}, ErrStationNotConfigured
	}

	config := cloneConfig(*service.config)
	for chargerIndex := range config.Chargers {
		charger := &config.Chargers[chargerIndex]
		if charger.ID != chargerID {
			continue
		}

		charger.Status = status
		unavailableConnectors := make(map[string]struct{})
		if status == domain.OperationalStatusUnavailable {
			for _, connector := range charger.Connectors {
				unavailableConnectors[connector.ID] = struct{}{}
			}
		}
		return service.applyAvailabilityUpdateLocked(config, unavailableConnectors)
	}

	return StationState{}, ErrChargerNotFound
}

func (service *Service) UpdateConnectorStatus(connectorID string, status domain.OperationalStatus) (StationState, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.config == nil {
		return StationState{}, ErrStationNotConfigured
	}

	config := cloneConfig(*service.config)
	for chargerIndex := range config.Chargers {
		for connectorIndex := range config.Chargers[chargerIndex].Connectors {
			connector := &config.Chargers[chargerIndex].Connectors[connectorIndex]
			if connector.ID != connectorID {
				continue
			}

			connector.Status = status
			unavailableConnectors := make(map[string]struct{})
			if status == domain.OperationalStatusUnavailable {
				unavailableConnectors[connector.ID] = struct{}{}
			}
			return service.applyAvailabilityUpdateLocked(config, unavailableConnectors)
		}
	}

	return StationState{}, ErrConnectorNotFound
}

func (service *Service) applyAvailabilityUpdateLocked(config domain.StationConfig, unavailableConnectors map[string]struct{}) (StationState, error) {
	if err := config.Validate(); err != nil {
		return StationState{}, err
	}

	for sessionID, session := range service.sessions {
		if _, unavailable := unavailableConnectors[session.ConnectorID]; unavailable {
			delete(service.sessions, sessionID)
		}
	}
	service.config = &config
	service.recomputeLocked(time.Now().UTC())
	return service.snapshotLocked(), nil
}
