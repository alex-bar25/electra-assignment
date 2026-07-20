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
)

type StationState struct {
	Config               domain.StationConfig `json:"config"`
	Sessions             []domain.Session     `json:"sessions"`
	GridImportKw         float64              `json:"gridImportKw"`
	AvailableGridPowerKw float64              `json:"availableGridPowerKw"`
	LastUpdatedAt        time.Time            `json:"lastUpdatedAt"`
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
	session.MinimumPowerKw = domain.NormalizeMinimumPowerKw(session.MinimumPowerKw)
	session.EffectiveDemandKw = domain.EffectiveDemandKw(session, connector.MaxPowerKw)
	if session.MinimumPowerKw > session.EffectiveDemandKw || session.MinimumPowerKw > charger.MaxPowerKw {
		return domain.Session{}, ErrMinimumExceedsDemand
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
		Config:               cloneConfig(*service.config),
		Sessions:             sessions,
		GridImportKw:         gridImport,
		AvailableGridPowerKw: available,
		LastUpdatedAt:        service.lastUpdatedAt,
	}
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
