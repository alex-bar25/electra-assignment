package service

import (
	"sort"
	"time"

	"electra-assignment/internal/domain"
)

type StationState struct {
	StationID               string           `json:"stationId"`
	GridCapacityKw          float64          `json:"gridCapacityKw"`
	GridImportKw            float64          `json:"gridImportKw"`
	AvailableGridPowerKw    float64          `json:"availableGridPowerKw"`
	AvailableStationPowerKw float64          `json:"availableStationPowerKw"`
	Chargers                []ChargerState   `json:"chargers"`
	Sessions                []domain.Session `json:"sessions"`
	LastUpdatedAt           time.Time        `json:"lastUpdatedAt"`
}

type ChargerState struct {
	ID             string                   `json:"id"`
	MaxPowerKw     float64                  `json:"maxPowerKw"`
	Status         domain.OperationalStatus `json:"status"`
	CurrentPowerKw float64                  `json:"currentPowerKw"`
	Connectors     []ConnectorState         `json:"connectors"`
}

type ConnectorState struct {
	ID              string                   `json:"id"`
	Type            string                   `json:"type"`
	MaxPowerKw      float64                  `json:"maxPowerKw"`
	Status          domain.OperationalStatus `json:"status"`
	Occupied        bool                     `json:"occupied"`
	ActiveSessionID string                   `json:"activeSessionId,omitempty"`
	AssignedPowerKw float64                  `json:"assignedPowerKw"`
}

func (service *Service) Snapshot() (StationState, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.config == nil {
		return StationState{}, ErrStationNotConfigured
	}
	return service.snapshotLocked(), nil
}

func (service *Service) snapshotLocked() StationState {
	sessions, gridImport := sessionsForSnapshot(service.sessions)
	available := service.config.GridCapacityKw - gridImport
	if available < 0 {
		available = 0
	}
	return StationState{
		StationID:               service.config.ID,
		GridCapacityKw:          service.config.GridCapacityKw,
		GridImportKw:            gridImport,
		AvailableGridPowerKw:    available,
		AvailableStationPowerKw: available,
		Chargers:                chargerStates(*service.config, sessions),
		Sessions:                sessions,
		LastUpdatedAt:           service.lastUpdatedAt,
	}
}

func chargerStates(config domain.StationConfig, sessions []domain.Session) []ChargerState {
	sessionByConnector := make(map[string]domain.Session, len(sessions))
	for _, session := range sessions {
		sessionByConnector[session.ConnectorID] = session
	}

	chargers := make([]ChargerState, 0, len(config.Chargers))
	for _, configuredCharger := range config.Chargers {
		charger := ChargerState{
			ID:         configuredCharger.ID,
			MaxPowerKw: configuredCharger.MaxPowerKw,
			Status:     configuredCharger.Status,
			Connectors: make([]ConnectorState, 0, len(configuredCharger.Connectors)),
		}
		for _, configuredConnector := range configuredCharger.Connectors {
			connector := ConnectorState{
				ID:         configuredConnector.ID,
				Type:       configuredConnector.Type,
				MaxPowerKw: configuredConnector.MaxPowerKw,
				Status:     configuredConnector.Status,
			}
			if session, occupied := sessionByConnector[configuredConnector.ID]; occupied {
				connector.Occupied = true
				connector.ActiveSessionID = session.ID
				connector.AssignedPowerKw = session.AssignedPowerKw
				charger.CurrentPowerKw += session.AssignedPowerKw
			}
			charger.Connectors = append(charger.Connectors, connector)
		}
		sort.Slice(charger.Connectors, func(i, j int) bool {
			return charger.Connectors[i].ID < charger.Connectors[j].ID
		})
		chargers = append(chargers, charger)
	}
	sort.Slice(chargers, func(i, j int) bool {
		return chargers[i].ID < chargers[j].ID
	})
	return chargers
}

func sessionsForSnapshot(stored map[string]domain.Session) ([]domain.Session, float64) {
	sessions := make([]domain.Session, 0, len(stored))
	for _, session := range stored {
		sessions = append(sessions, cloneSession(session))
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ID < sessions[j].ID
	})

	gridImport := 0.0
	for _, session := range sessions {
		gridImport += session.AssignedPowerKw
	}
	return sessions, gridImport
}
