package service

import (
	"errors"
	"testing"

	"electra-assignment/internal/domain"
)

func TestServiceConfigureAndSnapshot(t *testing.T) {
	service := New()
	if _, err := service.Snapshot(); !errors.Is(err, ErrStationNotConfigured) {
		t.Fatalf("Snapshot() error = %v, want ErrStationNotConfigured", err)
	}

	if _, err := service.Configure(testStationConfig()); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	state, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.StationID != "station-1" || state.GridCapacityKw != 100 || len(state.Sessions) != 0 {
		t.Fatalf("Snapshot() = %#v, want configured station without sessions", state)
	}
	if state.GridImportKw != 0 || state.AvailableGridPowerKw != 100 || state.AvailableStationPowerKw != 100 {
		t.Fatalf("power summary = (%v, %v, %v), want (0, 100, 100)", state.GridImportKw, state.AvailableGridPowerKw, state.AvailableStationPowerKw)
	}
	if state.LastUpdatedAt.IsZero() {
		t.Fatal("LastUpdatedAt is zero")
	}
}

func TestCloneConfigCopiesBESS(t *testing.T) {
	config := testStationConfig()
	config.BESS = &domain.BESSConfig{
		EnergyCapacityKwh: 200, SocPercent: 50,
		MaxChargePowerKw: 200, MaxDischargePowerKw: 200, MinSocPercent: 10,
	}

	cloned := cloneConfig(config)
	cloned.BESS.SocPercent = 80

	if config.BESS.SocPercent != 50 {
		t.Fatalf("original BESS SoC = %v, want 50", config.BESS.SocPercent)
	}
}

func testStationConfig() domain.StationConfig {
	return domain.StationConfig{
		ID:             "station-1",
		GridCapacityKw: 100,
		Chargers: []domain.ChargerConfig{{
			ID:         "charger-1",
			MaxPowerKw: 200,
			Status:     domain.OperationalStatusAvailable,
			Connectors: []domain.ConnectorConfig{
				{ID: "connector-1", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
				{ID: "connector-2", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
			},
		}},
	}
}
