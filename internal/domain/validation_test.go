package domain

import (
	"math"
	"testing"
)

func TestStationConfigValidate(t *testing.T) {
	t.Run("accepts a valid configuration", func(t *testing.T) {
		if err := validStationConfig().Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*StationConfig)
	}{
		{name: "blank station ID", mutate: func(config *StationConfig) { config.ID = "" }},
		{name: "non-positive grid capacity", mutate: func(config *StationConfig) { config.GridCapacityKw = 0 }},
		{name: "non-finite grid capacity", mutate: func(config *StationConfig) { config.GridCapacityKw = math.NaN() }},
		{name: "duplicate charger ID", mutate: func(config *StationConfig) {
			config.Chargers = append(config.Chargers, ChargerConfig{
				ID: "charger-1", MaxPowerKw: 100, Status: OperationalStatusAvailable,
				Connectors: []ConnectorConfig{{ID: "connector-2", Type: "CCS", MaxPowerKw: 100, Status: OperationalStatusAvailable}},
			})
		}},
		{name: "non-positive charger capacity", mutate: func(config *StationConfig) { config.Chargers[0].MaxPowerKw = -1 }},
		{name: "charger without connectors", mutate: func(config *StationConfig) { config.Chargers[0].Connectors = nil }},
		{name: "invalid charger status", mutate: func(config *StationConfig) { config.Chargers[0].Status = "broken" }},
		{name: "duplicate connector ID", mutate: func(config *StationConfig) {
			config.Chargers = append(config.Chargers, ChargerConfig{
				ID: "charger-2", MaxPowerKw: 100, Status: OperationalStatusAvailable,
				Connectors: []ConnectorConfig{{ID: "connector-1", Type: "CCS", MaxPowerKw: 100, Status: OperationalStatusAvailable}},
			})
		}},
		{name: "blank connector type", mutate: func(config *StationConfig) { config.Chargers[0].Connectors[0].Type = "" }},
		{name: "non-positive connector capacity", mutate: func(config *StationConfig) { config.Chargers[0].Connectors[0].MaxPowerKw = 0 }},
		{name: "invalid connector status", mutate: func(config *StationConfig) { config.Chargers[0].Connectors[0].Status = "broken" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validStationConfig()
			test.mutate(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
		})
	}
}

func validStationConfig() StationConfig {
	return StationConfig{
		ID:             "station-1",
		GridCapacityKw: 400,
		Chargers: []ChargerConfig{{
			ID:         "charger-1",
			MaxPowerKw: 300,
			Status:     OperationalStatusAvailable,
			Connectors: []ConnectorConfig{{
				ID: "connector-1", Type: "CCS", MaxPowerKw: 200, Status: OperationalStatusAvailable,
			}},
		}},
	}
}

func TestSessionValidate(t *testing.T) {
	curveLimit := 120.0
	valid := Session{
		ID:                   "session-1",
		ConnectorID:          "connector-1",
		RequestedPowerKw:     150,
		VehicleMaxPowerKw:    140,
		ChargingCurveLimitKw: &curveLimit,
	}

	t.Run("accepts valid power limits", func(t *testing.T) {
		if err := valid.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*Session)
	}{
		{name: "blank session ID", mutate: func(session *Session) { session.ID = "" }},
		{name: "blank connector ID", mutate: func(session *Session) { session.ConnectorID = "" }},
		{name: "non-positive requested power", mutate: func(session *Session) { session.RequestedPowerKw = 0 }},
		{name: "non-positive vehicle maximum", mutate: func(session *Session) { session.VehicleMaxPowerKw = -1 }},
		{name: "non-positive curve limit", mutate: func(session *Session) {
			invalid := 0.0
			session.ChargingCurveLimitKw = &invalid
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := valid
			test.mutate(&session)
			if err := session.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
		})
	}
}
