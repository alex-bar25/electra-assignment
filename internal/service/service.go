package service

import (
	"errors"
	"sort"
	"sync"
	"time"

	"electra-assignment/internal/allocation"
	"electra-assignment/internal/domain"
)

var (
	ErrStationNotConfigured = errors.New("station is not configured")
	ErrDuplicateSession     = errors.New("session already exists")
	ErrConnectorNotFound    = errors.New("connector not found")
	ErrConnectorOccupied    = errors.New("connector is occupied")
	ErrHardwareUnavailable  = errors.New("charger or connector is unavailable")
	ErrMinimumExceedsDemand = errors.New("minimum power exceeds effective demand")
	ErrSessionNotFound      = errors.New("session not found")
)

type SessionUpdate struct {
	RequestedPowerKw     *float64
	VehicleMaxPowerKw    *float64
	ChargingCurveLimitKw *float64
	MinimumPowerKw       *float64
}

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

type Service struct {
	mu            sync.Mutex
	config        *domain.StationConfig
	sessions      map[string]domain.Session
	lastUpdatedAt time.Time
}

func New() *Service {
	return &Service{sessions: make(map[string]domain.Session)}
}

func (service *Service) Configure(config domain.StationConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	config = cloneConfig(config)

	service.mu.Lock()
	defer service.mu.Unlock()
	service.config = &config
	service.sessions = make(map[string]domain.Session)
	service.lastUpdatedAt = time.Now().UTC()
	return nil
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

func cloneConfig(config domain.StationConfig) domain.StationConfig {
	clone := config
	clone.Chargers = make([]domain.ChargerConfig, len(config.Chargers))
	for index, charger := range config.Chargers {
		clone.Chargers[index] = charger
		clone.Chargers[index].Connectors = append([]domain.ConnectorConfig(nil), charger.Connectors...)
	}
	return clone
}

func cloneSession(session domain.Session) domain.Session {
	clone := session
	if session.ChargingCurveLimitKw != nil {
		curveLimit := *session.ChargingCurveLimitKw
		clone.ChargingCurveLimitKw = &curveLimit
	}
	return clone
}
