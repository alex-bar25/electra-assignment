package allocation

import (
	"sort"

	"electra-assignment/internal/domain"
)

const epsilon = 1e-9

type Assignment struct {
	SessionID       string  `json:"sessionId"`
	AssignedPowerKw float64 `json:"assignedPowerKw"`
}

type connectorLocation struct {
	charger   domain.ChargerConfig
	connector domain.ConnectorConfig
}

type allocationState struct {
	sessionID string
	chargerID string
	demandKw  float64
	assigned  float64
	active    bool
}

func Allocate(config domain.StationConfig, sessions []domain.Session) []Assignment {
	states, remainingByCharger := prepareAllocation(config, sessions)
	distributePower(states, config.GridCapacityKw, remainingByCharger)
	return assignmentsFromStates(states)
}

func prepareAllocation(config domain.StationConfig, sessions []domain.Session) ([]allocationState, map[string]float64) {
	locations := connectorLocations(config)
	states := make([]allocationState, 0, len(sessions))
	remainingByCharger := make(map[string]float64, len(config.Chargers))
	for _, charger := range config.Chargers {
		remainingByCharger[charger.ID] = charger.MaxPowerKw
	}

	for _, session := range sessions {
		state := allocationState{sessionID: session.ID}
		location, exists := locations[session.ConnectorID]
		if exists && location.charger.Status == domain.OperationalStatusAvailable && location.connector.Status == domain.OperationalStatusAvailable {
			state.chargerID = location.charger.ID
			state.demandKw = effectiveDemand(session, location.connector.MaxPowerKw)
			state.active = state.demandKw > epsilon
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].sessionID < states[j].sessionID
	})
	return states, remainingByCharger
}

func distributePower(states []allocationState, remainingGrid float64, remainingByCharger map[string]float64) {
	// Raise every active session by the same step. A session leaves the active
	// set when its own demand or its charger's shared capacity is exhausted.
	for {
		activeCount, activeByCharger := countActive(states)
		if activeCount == 0 || remainingGrid <= epsilon {
			break
		}

		step := remainingGrid / float64(activeCount)
		for _, state := range states {
			if !state.active {
				continue
			}
			chargerShare := remainingByCharger[state.chargerID] / float64(activeByCharger[state.chargerID])
			step = minimum(step, chargerShare, state.demandKw-state.assigned)
		}
		if step <= epsilon {
			break
		}

		for index := range states {
			if !states[index].active {
				continue
			}
			states[index].assigned += step
			remainingGrid -= step
			remainingByCharger[states[index].chargerID] -= step
		}

		for index := range states {
			if !states[index].active {
				continue
			}
			if states[index].demandKw-states[index].assigned <= epsilon || remainingByCharger[states[index].chargerID] <= epsilon {
				states[index].active = false
			}
		}
	}
}

func assignmentsFromStates(states []allocationState) []Assignment {
	assignments := make([]Assignment, 0, len(states))
	for _, state := range states {
		assignments = append(assignments, Assignment{
			SessionID:       state.sessionID,
			AssignedPowerKw: state.assigned,
		})
	}
	return assignments
}

func connectorLocations(config domain.StationConfig) map[string]connectorLocation {
	locations := make(map[string]connectorLocation)
	for _, charger := range config.Chargers {
		for _, connector := range charger.Connectors {
			locations[connector.ID] = connectorLocation{
				charger:   charger,
				connector: connector,
			}
		}
	}
	return locations
}

func effectiveDemand(session domain.Session, connectorMax float64) float64 {
	demand := minimum(session.RequestedPowerKw, session.VehicleMaxPowerKw, connectorMax)
	if session.ChargingCurveLimitKw != nil {
		demand = minimum(demand, *session.ChargingCurveLimitKw)
	}
	return demand
}

func countActive(states []allocationState) (int, map[string]int) {
	activeByCharger := make(map[string]int)
	activeCount := 0
	for _, state := range states {
		if state.active {
			activeCount++
			activeByCharger[state.chargerID]++
		}
	}
	return activeCount, activeByCharger
}

func minimum(values ...float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
