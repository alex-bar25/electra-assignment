package service

import (
	"math"
	"time"

	"electra-assignment/internal/domain"
)

const powerEpsilon = 1e-9

func (service *Service) AdvanceSimulation(elapsedSeconds float64) (StationState, error) {
	if elapsedSeconds <= 0 || math.IsInf(elapsedSeconds, 0) || math.IsNaN(elapsedSeconds) {
		return StationState{}, ErrInvalidSimulationDuration
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if service.config == nil {
		return StationState{}, ErrStationNotConfigured
	}
	if service.bess == nil {
		return StationState{}, ErrBESSNotConfigured
	}

	elapsedHours := elapsedSeconds / 3600
	// Positive BESS power is discharge, so it reduces the battery's stored energy.
	energyDeltaKwh := -service.bess.CurrentPowerKw * elapsedHours
	service.bess.SocPercent += energyDeltaKwh / service.bess.EnergyCapacityKwh * 100
	if service.bess.SocPercent < service.bess.MinSocPercent {
		service.bess.SocPercent = service.bess.MinSocPercent
	}
	if service.bess.SocPercent > 100 {
		service.bess.SocPercent = 100
	}

	service.recomputeLocked(time.Now().UTC())
	return service.snapshotLocked(), nil
}

func newBESSState(config *domain.BESSConfig) *domain.BESSState {
	if config == nil {
		return nil
	}
	return &domain.BESSState{
		EnergyCapacityKwh:   config.EnergyCapacityKwh,
		SocPercent:          config.SocPercent,
		MaxChargePowerKw:    config.MaxChargePowerKw,
		MaxDischargePowerKw: config.MaxDischargePowerKw,
		MinSocPercent:       config.MinSocPercent,
		Mode:                domain.BESSModeIdle,
	}
}

func (service *Service) availableStationSupplyLocked() float64 {
	return service.config.GridCapacityKw + service.permittedBESSDischargeLocked()
}

func (service *Service) permittedBESSDischargeLocked() float64 {
	if service.bess == nil || service.bess.SocPercent <= service.bess.MinSocPercent+powerEpsilon {
		return 0
	}
	return service.bess.MaxDischargePowerKw
}

func (service *Service) updateBESSDispatchLocked(evPowerKw float64) {
	if service.bess == nil {
		return
	}

	gridCapacityKw := service.config.GridCapacityKw
	if shortfallKw := evPowerKw - gridCapacityKw; shortfallKw > powerEpsilon {
		service.bess.CurrentPowerKw = minimumPower(shortfallKw, service.bess.MaxDischargePowerKw)
		service.bess.Mode = domain.BESSModeDischarging
		return
	}

	if spareGridPowerKw := gridCapacityKw - evPowerKw; spareGridPowerKw > powerEpsilon && service.bess.SocPercent < 100-powerEpsilon {
		service.bess.CurrentPowerKw = -minimumPower(spareGridPowerKw, service.bess.MaxChargePowerKw)
		service.bess.Mode = domain.BESSModeCharging
		return
	}

	service.bess.CurrentPowerKw = 0
	service.bess.Mode = domain.BESSModeIdle
}

func cloneBESSState(state *domain.BESSState) *domain.BESSState {
	if state == nil {
		return nil
	}
	clone := *state
	return &clone
}

func minimumPower(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
