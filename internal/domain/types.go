package domain

import "time"

type OperationalStatus string

type SessionStatus string

const DefaultMinimumPowerKw = 5.0

const (
	OperationalStatusAvailable   OperationalStatus = "available"
	OperationalStatusUnavailable OperationalStatus = "unavailable"

	SessionStatusCharging        SessionStatus = "charging"
	SessionStatusWaitingForPower SessionStatus = "waiting_for_power"
)

type StationConfig struct {
	ID             string          `json:"id"`
	GridCapacityKw float64         `json:"gridCapacityKw"`
	Chargers       []ChargerConfig `json:"chargers"`
	BESS           *BESSConfig     `json:"bess,omitempty"`
}

type BESSConfig struct {
	EnergyCapacityKwh   float64 `json:"energyCapacityKwh"`
	SocPercent          float64 `json:"socPercent"`
	MaxChargePowerKw    float64 `json:"maxChargePowerKw"`
	MaxDischargePowerKw float64 `json:"maxDischargePowerKw"`
	MinSocPercent       float64 `json:"minSocPercent"`
}

type ChargerConfig struct {
	ID         string            `json:"id"`
	MaxPowerKw float64           `json:"maxPowerKw"`
	Status     OperationalStatus `json:"status"`
	Connectors []ConnectorConfig `json:"connectors"`
}

type ConnectorConfig struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	MaxPowerKw float64           `json:"maxPowerKw"`
	Status     OperationalStatus `json:"status"`
}

type Session struct {
	ID                   string        `json:"id"`
	ConnectorID          string        `json:"connectorId"`
	RequestedPowerKw     float64       `json:"requestedPowerKw"`
	VehicleMaxPowerKw    float64       `json:"vehicleMaxPowerKw"`
	ChargingCurveLimitKw *float64      `json:"chargingCurveLimitKw,omitempty"`
	MinimumPowerKw       float64       `json:"minimumPowerKw"`
	EffectiveDemandKw    float64       `json:"effectiveDemandKw"`
	AssignedPowerKw      float64       `json:"assignedPowerKw"`
	Status               SessionStatus `json:"status"`
	StartedAt            time.Time     `json:"startedAt"`
	UpdatedAt            time.Time     `json:"updatedAt"`
}
