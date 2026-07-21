package service

import (
	"testing"

	"electra-assignment/internal/domain"
)

func TestBESSDischargeBoostsStationSupply(t *testing.T) {
	service := configuredService(t, bessStationConfig(50))
	startBESSSession(t, service, "session-1", "connector-1", 300)
	startBESSSession(t, service, "session-2", "connector-2", 300)

	state, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Sessions[0].AssignedPowerKw != 300 || state.Sessions[1].AssignedPowerKw != 300 {
		t.Fatalf("session allocations = %#v, want 300/300 kW", state.Sessions)
	}
	if state.GridImportKw != 400 || state.AvailableGridPowerKw != 0 || state.AvailableStationPowerKw != 0 {
		t.Fatalf("power summary = (%v, %v, %v), want (400, 0, 0)", state.GridImportKw, state.AvailableGridPowerKw, state.AvailableStationPowerKw)
	}
	if state.BESS == nil || state.BESS.CurrentPowerKw != 200 || state.BESS.Mode != domain.BESSModeDischarging {
		t.Fatalf("BESS state = %#v, want 200 kW discharging", state.BESS)
	}
}

func TestBESSAtMinimumSocDoesNotDischarge(t *testing.T) {
	service := configuredService(t, bessStationConfig(10))
	startBESSSession(t, service, "session-1", "connector-1", 300)
	startBESSSession(t, service, "session-2", "connector-2", 300)

	state, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Sessions[0].AssignedPowerKw != 200 || state.Sessions[1].AssignedPowerKw != 200 {
		t.Fatalf("session allocations = %#v, want 200/200 kW", state.Sessions)
	}
	if state.BESS == nil || state.BESS.CurrentPowerKw != 0 || state.BESS.Mode != domain.BESSModeIdle {
		t.Fatalf("BESS state = %#v, want idle at minimum SoC", state.BESS)
	}
	if state.GridImportKw != 400 {
		t.Fatalf("grid import = %v, want 400", state.GridImportKw)
	}
}

func TestBESSChargesOnlyFromPowerLeftAfterEVs(t *testing.T) {
	tests := []struct {
		name            string
		demandKw        float64
		wantBESSPowerKw float64
		wantGridImport  float64
	}{
		{name: "charge power limit", demandKw: 100, wantBESSPowerKw: -200, wantGridImport: 300},
		{name: "EV demand takes priority", demandKw: 350, wantBESSPowerKw: -50, wantGridImport: 400},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := configuredService(t, bessStationConfig(50))
			started := startBESSSession(t, service, "session-1", "connector-1", test.demandKw)
			if started.AssignedPowerKw != test.demandKw {
				t.Fatalf("assigned power = %v, want %v", started.AssignedPowerKw, test.demandKw)
			}

			state, err := service.Snapshot()
			if err != nil {
				t.Fatalf("Snapshot() error = %v", err)
			}
			if state.BESS == nil || state.BESS.CurrentPowerKw != test.wantBESSPowerKw || state.BESS.Mode != domain.BESSModeCharging {
				t.Fatalf("BESS state = %#v, want %v kW charging", state.BESS, test.wantBESSPowerKw)
			}
			if state.GridImportKw != test.wantGridImport {
				t.Fatalf("grid import = %v, want %v", state.GridImportKw, test.wantGridImport)
			}
		})
	}
}

func TestBESSStateSnapshotIsCopied(t *testing.T) {
	service := configuredService(t, bessStationConfig(50))
	first, err := service.Snapshot()
	if err != nil {
		t.Fatalf("first Snapshot() error = %v", err)
	}
	first.BESS.SocPercent = 90

	second, err := service.Snapshot()
	if err != nil {
		t.Fatalf("second Snapshot() error = %v", err)
	}
	if second.BESS.SocPercent != 50 {
		t.Fatalf("stored BESS SoC = %v, want 50", second.BESS.SocPercent)
	}
}

func bessStationConfig(socPercent float64) domain.StationConfig {
	return domain.StationConfig{
		ID:             "station-1",
		GridCapacityKw: 400,
		Chargers: []domain.ChargerConfig{
			{
				ID: "charger-1", MaxPowerKw: 400, Status: domain.OperationalStatusAvailable,
				Connectors: []domain.ConnectorConfig{{ID: "connector-1", Type: "CCS", MaxPowerKw: 400, Status: domain.OperationalStatusAvailable}},
			},
			{
				ID: "charger-2", MaxPowerKw: 400, Status: domain.OperationalStatusAvailable,
				Connectors: []domain.ConnectorConfig{{ID: "connector-2", Type: "CCS", MaxPowerKw: 400, Status: domain.OperationalStatusAvailable}},
			},
		},
		BESS: &domain.BESSConfig{
			EnergyCapacityKwh: 200, SocPercent: socPercent,
			MaxChargePowerKw: 200, MaxDischargePowerKw: 200, MinSocPercent: 10,
		},
	}
}

func startBESSSession(t *testing.T, service *Service, sessionID, connectorID string, demandKw float64) domain.Session {
	t.Helper()
	session, err := service.StartSession(testSession(sessionID, connectorID, demandKw))
	if err != nil {
		t.Fatalf("StartSession(%q) error = %v", sessionID, err)
	}
	return session
}
