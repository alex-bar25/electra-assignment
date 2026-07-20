package domain

import (
	"fmt"
	"math"
	"strings"
)

func (config StationConfig) Validate() error {
	if strings.TrimSpace(config.ID) == "" {
		return fmt.Errorf("station ID is required")
	}
	if !isPositiveFinite(config.GridCapacityKw) {
		return fmt.Errorf("grid capacity must be positive")
	}

	chargerIDs := make(map[string]struct{}, len(config.Chargers))
	connectorIDs := make(map[string]struct{})
	for _, charger := range config.Chargers {
		if err := validateCharger(charger, chargerIDs, connectorIDs); err != nil {
			return err
		}
	}

	return nil
}

func (session Session) Validate() error {
	if strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("session ID is required")
	}
	if strings.TrimSpace(session.ConnectorID) == "" {
		return fmt.Errorf("connector ID is required")
	}
	if !isPositiveFinite(session.RequestedPowerKw) {
		return fmt.Errorf("requested power must be positive")
	}
	if !isPositiveFinite(session.VehicleMaxPowerKw) {
		return fmt.Errorf("vehicle maximum power must be positive")
	}
	if session.ChargingCurveLimitKw != nil && !isPositiveFinite(*session.ChargingCurveLimitKw) {
		return fmt.Errorf("charging curve limit must be positive when provided")
	}
	if session.MinimumPowerKw < 0 || math.IsInf(session.MinimumPowerKw, 0) || math.IsNaN(session.MinimumPowerKw) {
		return fmt.Errorf("minimum power cannot be negative or non-finite")
	}

	return nil
}

func NormalizeMinimumPowerKw(value float64) float64 {
	if value == 0 {
		return DefaultMinimumPowerKw
	}
	return value
}

func EffectiveDemandKw(session Session, connectorMaxPowerKw float64) float64 {
	demand := minimumPower(session.RequestedPowerKw, session.VehicleMaxPowerKw, connectorMaxPowerKw)
	if session.ChargingCurveLimitKw != nil {
		demand = minimumPower(demand, *session.ChargingCurveLimitKw)
	}
	return demand
}

func minimumPower(values ...float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func validateCharger(charger ChargerConfig, chargerIDs, connectorIDs map[string]struct{}) error {
	if strings.TrimSpace(charger.ID) == "" {
		return fmt.Errorf("charger ID is required")
	}
	if _, exists := chargerIDs[charger.ID]; exists {
		return fmt.Errorf("duplicate charger ID %q", charger.ID)
	}
	chargerIDs[charger.ID] = struct{}{}

	if !isPositiveFinite(charger.MaxPowerKw) {
		return fmt.Errorf("charger %q maximum power must be positive", charger.ID)
	}
	if !charger.Status.valid() {
		return fmt.Errorf("charger %q has invalid status %q", charger.ID, charger.Status)
	}
	if len(charger.Connectors) == 0 {
		return fmt.Errorf("charger %q must have at least one connector", charger.ID)
	}

	for _, connector := range charger.Connectors {
		if err := validateConnector(connector, connectorIDs); err != nil {
			return err
		}
	}

	return nil
}

func validateConnector(connector ConnectorConfig, connectorIDs map[string]struct{}) error {
	if strings.TrimSpace(connector.ID) == "" {
		return fmt.Errorf("connector ID is required")
	}
	if _, exists := connectorIDs[connector.ID]; exists {
		return fmt.Errorf("duplicate connector ID %q", connector.ID)
	}
	connectorIDs[connector.ID] = struct{}{}

	if strings.TrimSpace(connector.Type) == "" {
		return fmt.Errorf("connector %q type is required", connector.ID)
	}
	if !isPositiveFinite(connector.MaxPowerKw) {
		return fmt.Errorf("connector %q maximum power must be positive", connector.ID)
	}
	if !connector.Status.valid() {
		return fmt.Errorf("connector %q has invalid status %q", connector.ID, connector.Status)
	}

	return nil
}

func (status OperationalStatus) valid() bool {
	return status == OperationalStatusAvailable || status == OperationalStatusUnavailable
}

func isPositiveFinite(value float64) bool {
	return value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}
