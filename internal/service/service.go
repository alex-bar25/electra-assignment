package service

import (
	"errors"
	"sync"
	"time"

	"electra-assignment/internal/domain"
)

var (
	ErrStationNotConfigured = errors.New("station is not configured")
	ErrDuplicateSession     = errors.New("session already exists")
	ErrChargerNotFound      = errors.New("charger not found")
	ErrConnectorNotFound    = errors.New("connector not found")
	ErrConnectorOccupied    = errors.New("connector is occupied")
	ErrHardwareUnavailable  = errors.New("charger or connector is unavailable")
	ErrMinimumExceedsDemand = errors.New("minimum power exceeds effective demand")
	ErrSessionNotFound      = errors.New("session not found")
)

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

func cloneConfig(config domain.StationConfig) domain.StationConfig {
	clone := config
	if config.BESS != nil {
		bess := *config.BESS
		clone.BESS = &bess
	}
	clone.Chargers = make([]domain.ChargerConfig, len(config.Chargers))
	for index, charger := range config.Chargers {
		clone.Chargers[index] = charger
		clone.Chargers[index].Connectors = append([]domain.ConnectorConfig(nil), charger.Connectors...)
	}
	return clone
}
