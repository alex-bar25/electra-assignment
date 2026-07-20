package domain

import "time"

type OperationalStatus string

const (
	OperationalStatusAvailable   OperationalStatus = "available"
	OperationalStatusUnavailable OperationalStatus = "unavailable"
)

type StationConfig struct {
	ID             string          `json:"id"`
	GridCapacityKw float64         `json:"gridCapacityKw"`
	Chargers       []ChargerConfig `json:"chargers"`
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
	ID                   string    `json:"id"`
	ConnectorID          string    `json:"connectorId"`
	RequestedPowerKw     float64   `json:"requestedPowerKw"`
	VehicleMaxPowerKw    float64   `json:"vehicleMaxPowerKw"`
	ChargingCurveLimitKw *float64  `json:"chargingCurveLimitKw,omitempty"`
	AssignedPowerKw      float64   `json:"assignedPowerKw"`
	StartedAt            time.Time `json:"startedAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}
