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

func TestStationConfigValidateBESS(t *testing.T) {
	validBESS := func() *BESSConfig {
		return &BESSConfig{
			EnergyCapacityKwh:   200,
			SocPercent:          50,
			MaxChargePowerKw:    200,
			MaxDischargePowerKw: 200,
			MinSocPercent:       10,
		}
	}

	t.Run("accepts valid BESS configuration", func(t *testing.T) {
		config := validStationConfig()
		config.BESS = validBESS()
		if err := config.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*BESSConfig)
	}{
		{name: "non-positive energy capacity", mutate: func(bess *BESSConfig) { bess.EnergyCapacityKwh = 0 }},
		{name: "non-positive charge power", mutate: func(bess *BESSConfig) { bess.MaxChargePowerKw = 0 }},
		{name: "non-finite discharge power", mutate: func(bess *BESSConfig) { bess.MaxDischargePowerKw = math.NaN() }},
		{name: "zero minimum SoC", mutate: func(bess *BESSConfig) { bess.MinSocPercent = 0 }},
		{name: "minimum SoC at 100 percent", mutate: func(bess *BESSConfig) { bess.MinSocPercent = 100 }},
		{name: "SoC below minimum", mutate: func(bess *BESSConfig) { bess.SocPercent = 9 }},
		{name: "SoC above 100 percent", mutate: func(bess *BESSConfig) { bess.SocPercent = 101 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validStationConfig()
			config.BESS = validBESS()
			test.mutate(config.BESS)
			if err := config.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
		})
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

func TestSessionValidateRejectsNegativeMinimumPower(t *testing.T) {
	session := Session{
		ID: "session-1", ConnectorID: "connector-1",
		RequestedPowerKw: 100, VehicleMaxPowerKw: 100,
		MinimumPowerKw: -1,
	}
	if err := session.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want an error")
	}
}

func TestNormalizeMinimumPowerKw(t *testing.T) {
	if got := NormalizeMinimumPowerKw(0); got != 5 {
		t.Fatalf("NormalizeMinimumPowerKw(0) = %v, want 5", got)
	}
	if got := NormalizeMinimumPowerKw(12); got != 12 {
		t.Fatalf("NormalizeMinimumPowerKw(12) = %v, want 12", got)
	}
}
