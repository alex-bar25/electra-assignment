package service

import (
	"time"

	"electra-assignment/internal/allocation"
	"electra-assignment/internal/domain"
)

type SessionUpdate struct {
	RequestedPowerKw     *float64
	VehicleMaxPowerKw    *float64
	ChargingCurveLimitKw *float64
	MinimumPowerKw       *float64
}

func (service *Service) StartSession(session domain.Session) (domain.Session, error) {
	session = cloneSession(session)
	if err := session.Validate(); err != nil {
		return domain.Session{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if service.config == nil {
		return domain.Session{}, ErrStationNotConfigured
	}
	if _, exists := service.sessions[session.ID]; exists {
		return domain.Session{}, ErrDuplicateSession
	}
	charger, connector, exists := findConnector(*service.config, session.ConnectorID)
	if !exists {
		return domain.Session{}, ErrConnectorNotFound
	}
	if charger.Status != domain.OperationalStatusAvailable || connector.Status != domain.OperationalStatusAvailable {
		return domain.Session{}, ErrHardwareUnavailable
	}
	if err := normalizeSessionPower(&session, charger, connector); err != nil {
		return domain.Session{}, err
	}
	for _, active := range service.sessions {
		if active.ConnectorID == session.ConnectorID {
			return domain.Session{}, ErrConnectorOccupied
		}
	}

	now := time.Now().UTC()
	session.AssignedPowerKw = 0
	session.StartedAt = now
	session.UpdatedAt = now
	service.sessions[session.ID] = session
	service.recomputeLocked(now)
	return cloneSession(service.sessions[session.ID]), nil
}

func (service *Service) UpdateSession(sessionID string, update SessionUpdate) (domain.Session, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.config == nil {
		return domain.Session{}, ErrStationNotConfigured
	}

	stored, exists := service.sessions[sessionID]
	if !exists {
		return domain.Session{}, ErrSessionNotFound
	}
	candidate := cloneSession(stored)
	if update.RequestedPowerKw != nil {
		candidate.RequestedPowerKw = *update.RequestedPowerKw
	}
	if update.VehicleMaxPowerKw != nil {
		candidate.VehicleMaxPowerKw = *update.VehicleMaxPowerKw
	}
	if update.ChargingCurveLimitKw != nil {
		curveLimit := *update.ChargingCurveLimitKw
		candidate.ChargingCurveLimitKw = &curveLimit
	}
	if update.MinimumPowerKw != nil {
		candidate.MinimumPowerKw = *update.MinimumPowerKw
	}
	if err := candidate.Validate(); err != nil {
		return domain.Session{}, err
	}

	charger, connector, exists := findConnector(*service.config, candidate.ConnectorID)
	if !exists {
		return domain.Session{}, ErrConnectorNotFound
	}
	if err := normalizeSessionPower(&candidate, charger, connector); err != nil {
		return domain.Session{}, err
	}

	now := time.Now().UTC()
	candidate.UpdatedAt = now
	service.sessions[sessionID] = candidate
	service.recomputeLocked(now)
	return cloneSession(service.sessions[sessionID]), nil
}

func (service *Service) StopSession(sessionID string) (StationState, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.config == nil {
		return StationState{}, ErrStationNotConfigured
	}
	if _, exists := service.sessions[sessionID]; !exists {
		return StationState{}, ErrSessionNotFound
	}

	delete(service.sessions, sessionID)
	service.recomputeLocked(time.Now().UTC())
	return service.snapshotLocked(), nil
}

func (service *Service) recomputeLocked(now time.Time) {
	sessions := make([]domain.Session, 0, len(service.sessions))
	for _, session := range service.sessions {
		sessions = append(sessions, session)
	}
	for _, assignment := range allocation.Allocate(*service.config, sessions) {
		session := service.sessions[assignment.SessionID]
		session.EffectiveDemandKw = assignment.EffectiveDemandKw
		session.AssignedPowerKw = assignment.AssignedPowerKw
		session.Status = assignment.Status
		service.sessions[assignment.SessionID] = session
	}
	service.lastUpdatedAt = now
}

func findConnector(config domain.StationConfig, connectorID string) (domain.ChargerConfig, domain.ConnectorConfig, bool) {
	for _, charger := range config.Chargers {
		for _, connector := range charger.Connectors {
			if connector.ID == connectorID {
				return charger, connector, true
			}
		}
	}
	return domain.ChargerConfig{}, domain.ConnectorConfig{}, false
}

func normalizeSessionPower(session *domain.Session, charger domain.ChargerConfig, connector domain.ConnectorConfig) error {
	session.MinimumPowerKw = domain.NormalizeMinimumPowerKw(session.MinimumPowerKw)
	session.EffectiveDemandKw = domain.EffectiveDemandKw(*session, connector.MaxPowerKw)
	if session.MinimumPowerKw > session.EffectiveDemandKw || session.MinimumPowerKw > charger.MaxPowerKw {
		return ErrMinimumExceedsDemand
	}
	return nil
}

func cloneSession(session domain.Session) domain.Session {
	clone := session
	if session.ChargingCurveLimitKw != nil {
		curveLimit := *session.ChargingCurveLimitKw
		clone.ChargingCurveLimitKw = &curveLimit
	}
	return clone
}
