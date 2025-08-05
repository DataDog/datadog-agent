// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package core

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=network_mock.go

// TimeProvider defines an interface for getting the current time.
type TimeProvider interface {
	Now() time.Time
}

// RealTime provides the real system time.
type RealTime struct{}

// Now returns the current system time.
func (RealTime) Now() time.Time { return time.Now() }

// ServiceNetworkCache holds cached network information for a service.
type ServiceNetworkCache struct {
	RxBytes uint64
	TxBytes uint64
	RxBps   float64
	TxBps   float64
}

// NetworkStatsManager manages network statistics collection and computation.
type NetworkStatsManager struct {
	network                NetworkCollector
	timeProvider           TimeProvider
	lastNetworkStatsUpdate time.Time
	networkStatsPeriod     time.Duration
	cache                  map[int32]*ServiceNetworkCache
	networkErrorLimit      *log.Limit
}

// NewNetworkStatsManager creates a new NetworkStatsManager.
func NewNetworkStatsManager(network NetworkCollector, networkStatsPeriod time.Duration) *NetworkStatsManager {
	return &NetworkStatsManager{
		network:            network,
		timeProvider:       RealTime{},
		networkStatsPeriod: networkStatsPeriod,
		cache:              make(map[int32]*ServiceNetworkCache),
		networkErrorLimit:  log.NewLogLimit(10, 10*time.Minute),
	}
}

// WithTimeProvider replaces the time provider used by the network stats manager.
func (m *NetworkStatsManager) WithTimeProvider(timeProvider TimeProvider) *NetworkStatsManager {
	m.timeProvider = timeProvider
	return m
}

// UpdateNetworkStats updates the network statistics for the provided PIDs.
// It computes the bytes per second (BPS) rates based on the delta from previous measurements.
func (m *NetworkStatsManager) UpdateNetworkStats(deltaSeconds float64, pids PidSet) error {
	if m.network == nil {
		return nil
	}

	allStats, err := m.network.GetStats(pids)
	if err != nil {
		if m.networkErrorLimit.ShouldLog() {
			log.Warnf("unable to get network stats: %v", err)
		}
		return err
	}

	// Update cache with new network stats and compute BPS rates
	for pid, stats := range allStats {
		cachedStats, ok := m.cache[int32(pid)]
		if !ok {
			// First time seeing this PID, initialize cache
			m.cache[int32(pid)] = &ServiceNetworkCache{
				RxBytes: stats.Rx,
				TxBytes: stats.Tx,
				RxBps:   0,
				TxBps:   0,
			}
			continue
		}

		deltaRx := stats.Rx - cachedStats.RxBytes
		deltaTx := stats.Tx - cachedStats.TxBytes

		cachedStats.RxBps = float64(deltaRx) / deltaSeconds
		cachedStats.TxBps = float64(deltaTx) / deltaSeconds
		cachedStats.RxBytes = stats.Rx
		cachedStats.TxBytes = stats.Tx
	}

	return nil
}

// GetNetworkStats returns the cached network statistics for a specific PID.
func (m *NetworkStatsManager) GetNetworkStats(pid int32) (*ServiceNetworkCache, bool) {
	stats, ok := m.cache[pid]
	return stats, ok
}

// MaybeUpdateNetworkStats updates network statistics if enough time has passed since the last update.
func (m *NetworkStatsManager) MaybeUpdateNetworkStats(pids PidSet) error {
	if m.network == nil {
		return nil
	}

	now := m.timeProvider.Now()
	delta := now.Sub(m.lastNetworkStatsUpdate)
	if delta < m.networkStatsPeriod {
		return nil
	}

	deltaSeconds := delta.Seconds()
	err := m.UpdateNetworkStats(deltaSeconds, pids)
	if err == nil {
		m.lastNetworkStatsUpdate = now
	}
	return err
}

// CleanCache removes network statistics for PIDs that are no longer alive.
func (m *NetworkStatsManager) CleanCache(alivePids PidSet) {
	for pid := range m.cache {
		if !alivePids.Has(pid) {
			delete(m.cache, pid)
		}
	}
}

// Close cleans up resources used by the NetworkStatsManager.
func (m *NetworkStatsManager) Close() {
	if m.network != nil {
		m.network.Close()
		m.network = nil
	}
	clear(m.cache)
}

// ComputeNetworkBPS computes bytes per second rates from current and previous network stats.
// This is a standalone utility function for computing BPS rates.
func ComputeNetworkBPS(currentRx, currentTx, previousRx, previousTx uint64, deltaSeconds float64) (rxBps, txBps float64) {
	deltaRx := currentRx - previousRx
	deltaTx := currentTx - previousTx

	rxBps = float64(deltaRx) / deltaSeconds
	txBps = float64(deltaTx) / deltaSeconds

	return rxBps, txBps
}
