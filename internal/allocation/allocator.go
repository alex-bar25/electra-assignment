package allocation

import (
	"math"
	"sort"
	"time"

	"electra-assignment/internal/domain"
)

const epsilon = 1e-9

type Assignment struct {
	SessionID         string               `json:"sessionId"`
	EffectiveDemandKw float64              `json:"effectiveDemandKw"`
	AssignedPowerKw   float64              `json:"assignedPowerKw"`
	Status            domain.SessionStatus `json:"status"`
}

type connectorLocation struct {
	charger   domain.ChargerConfig
	connector domain.ConnectorConfig
}

type allocationState struct {
	sessionID string
	chargerID string
	demandKw  float64
	minimumKw float64
	startedAt time.Time
	assigned  float64
	eligible  bool
	active    bool
	status    domain.SessionStatus
}

func Allocate(config domain.StationConfig, sessions []domain.Session) []Assignment {
	states, remainingByCharger := prepareAllocation(config, sessions)
	remainingGrid := admitSessions(states, config.GridCapacityKw, remainingByCharger)
	distributePower(states, remainingGrid, remainingByCharger)
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
			state.demandKw = domain.EffectiveDemandKw(session, location.connector.MaxPowerKw)
			state.minimumKw = domain.NormalizeMinimumPowerKw(session.MinimumPowerKw)
			state.startedAt = session.StartedAt
			state.eligible = state.demandKw > epsilon
			if state.eligible {
				state.status = domain.SessionStatusWaitingForPower
			}
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].sessionID < states[j].sessionID
	})
	return states, remainingByCharger
}

func admitSessions(states []allocationState, remainingGrid float64, remainingByCharger map[string]float64) float64 {
	priority := make([]int, 0, len(states))
	for index := range states {
		if states[index].eligible {
			priority = append(priority, index)
		}
	}
	sort.Slice(priority, func(i, j int) bool {
		left, right := states[priority[i]], states[priority[j]]
		if left.startedAt.Equal(right.startedAt) {
			return left.sessionID < right.sessionID
		}
		return left.startedAt.Before(right.startedAt)
	})

	for _, index := range priority {
		state := &states[index]
		if state.minimumKw > state.demandKw+epsilon ||
			state.minimumKw > remainingGrid+epsilon ||
			state.minimumKw > remainingByCharger[state.chargerID]+epsilon {
			continue
		}
		state.assigned = state.minimumKw
		state.status = domain.SessionStatusCharging
		state.active = state.demandKw-state.assigned > epsilon
		remainingGrid -= state.minimumKw
		remainingByCharger[state.chargerID] -= state.minimumKw
	}
	return remainingGrid
}

func distributePower(states []allocationState, remainingGrid float64, remainingByCharger map[string]float64) {
	for index := range states {
		if states[index].active && remainingByCharger[states[index].chargerID] <= epsilon {
			states[index].active = false
		}
	}

	// Raise the lowest assigned sessions until they catch the next level or
	// reach a physical limit. This preserves max-min fairness after minimums.
	for {
		lowestAssigned, activeCount, activeByCharger, nextAssigned := lowestActiveGroup(states)
		if activeCount == 0 || remainingGrid <= epsilon {
			break
		}

		step := remainingGrid / float64(activeCount)
		if !math.IsInf(nextAssigned, 1) {
			step = minimum(step, nextAssigned-lowestAssigned)
		}
		for _, state := range states {
			if !state.active || state.assigned > lowestAssigned+epsilon {
				continue
			}
			chargerShare := remainingByCharger[state.chargerID] / float64(activeByCharger[state.chargerID])
			step = minimum(step, chargerShare, state.demandKw-state.assigned)
		}
		if step <= epsilon {
			break
		}

		for index := range states {
			if !states[index].active || states[index].assigned > lowestAssigned+epsilon {
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
			SessionID:         state.sessionID,
			EffectiveDemandKw: state.demandKw,
			AssignedPowerKw:   state.assigned,
			Status:            state.status,
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

func lowestActiveGroup(states []allocationState) (float64, int, map[string]int, float64) {
	lowestAssigned := math.Inf(1)
	for _, state := range states {
		if state.active && state.assigned < lowestAssigned {
			lowestAssigned = state.assigned
		}
	}

	activeByCharger := make(map[string]int)
	activeCount := 0
	nextAssigned := math.Inf(1)
	for _, state := range states {
		if !state.active {
			continue
		}
		if state.assigned <= lowestAssigned+epsilon {
			activeCount++
			activeByCharger[state.chargerID]++
		} else if state.assigned < nextAssigned {
			nextAssigned = state.assigned
		}
	}
	return lowestAssigned, activeCount, activeByCharger, nextAssigned
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
