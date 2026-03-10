// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package memory provides exact byte accounting for retained logs-agent buffers.
package memory

import (
	"errors"
	"sync"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// EnabledConfigKey controls whether the logs memory budget is active.
	EnabledConfigKey = "logs_config.memory_budget.enabled"
	// MaxBytesConfigKey controls the maximum number of bytes the budget allows.
	MaxBytesConfigKey = "logs_config.memory_budget.max_bytes"
	// OverflowPolicyConfigKey controls whether overflow blocks or drops.
	OverflowPolicyConfigKey = "logs_config.memory_budget.overflow_policy"
	// TotalComponentName is used for the aggregate bytes-in-use gauge.
	TotalComponentName = "total"
)

var (
	// ErrInvalidReservationSize is returned when the requested byte count is negative.
	ErrInvalidReservationSize = errors.New("reservation size must be greater than or equal to zero")
	// ErrReservationExceedsLimit is returned when a single reservation cannot fit within the configured limit.
	ErrReservationExceedsLimit = errors.New("reservation exceeds memory budget limit")
	// ErrReleaseExceedsReservation is returned when a component attempts to release more bytes than it owns.
	ErrReleaseExceedsReservation = errors.New("release exceeds reserved bytes")
)

// OverflowPolicy controls how the budget should respond to overflows.
type OverflowPolicy string

const (
	// OverflowPolicyBlock blocks until bytes become available.
	OverflowPolicyBlock OverflowPolicy = "block"
	// OverflowPolicyDrop indicates future callers may drop instead of blocking.
	OverflowPolicyDrop OverflowPolicy = "drop"
)

// Config stores the static configuration for a logs memory budget.
type Config struct {
	Enabled        bool
	MaxBytes       int64
	OverflowPolicy OverflowPolicy
}

// Snapshot stores a point-in-time view of budget usage.
type Snapshot struct {
	Config
	UsedBytes      int64
	ComponentBytes map[string]int64
}

// Budget tracks bytes retained across logs-agent components.
type Budget struct {
	mu             sync.Mutex
	waiters        *sync.Cond
	config         Config
	usedBytes      int64
	componentBytes map[string]int64
}

// New returns a new logs memory budget.
func New(config Config) *Budget {
	normalized := normalizeConfig(config)
	budget := &Budget{
		config:         normalized,
		componentBytes: make(map[string]int64),
	}
	budget.waiters = sync.NewCond(&budget.mu)
	budget.reportConfig()
	budget.reportUsage(TotalComponentName, 0)
	return budget
}

// NewFromConfig returns a new logs memory budget using values from the config reader.
func NewFromConfig(cfg pkgconfigmodel.Reader) *Budget {
	return New(Config{
		Enabled:        cfg.GetBool(EnabledConfigKey),
		MaxBytes:       cfg.GetInt64(MaxBytesConfigKey),
		OverflowPolicy: OverflowPolicy(cfg.GetString(OverflowPolicyConfigKey)),
	})
}

// Acquire blocks until the budget can reserve the requested number of bytes.
func (b *Budget) Acquire(component string, size int64) error {
	if err := validateReservationSize(size); err != nil {
		return err
	}
	if !b.config.Enabled || size == 0 {
		return nil
	}
	if size > b.config.MaxBytes {
		return ErrReservationExceedsLimit
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	blocked := false
	for b.usedBytes+size > b.config.MaxBytes {
		blocked = true
		b.waiters.Wait()
	}
	if blocked {
		b.reportOverflow(component, "block")
	}
	b.reserveLocked(component, size)
	return nil
}

// TryAcquire reserves bytes without blocking.
func (b *Budget) TryAcquire(component string, size int64) (bool, error) {
	if err := validateReservationSize(size); err != nil {
		return false, err
	}
	if !b.config.Enabled || size == 0 {
		return true, nil
	}
	if size > b.config.MaxBytes {
		b.reportOverflow(component, "try_acquire")
		return false, ErrReservationExceedsLimit
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.usedBytes+size > b.config.MaxBytes {
		b.reportOverflow(component, "try_acquire")
		return false, nil
	}
	b.reserveLocked(component, size)
	return true, nil
}

// Release returns bytes to the budget.
func (b *Budget) Release(component string, size int64) error {
	if err := validateReservationSize(size); err != nil {
		return err
	}
	if !b.config.Enabled || size == 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	componentUsage := b.componentBytes[component]
	if size > componentUsage || size > b.usedBytes {
		return ErrReleaseExceedsReservation
	}

	componentUsage -= size
	b.usedBytes -= size
	if componentUsage == 0 {
		delete(b.componentBytes, component)
		b.reportUsage(component, 0)
	} else {
		b.componentBytes[component] = componentUsage
		b.reportUsage(component, componentUsage)
	}
	b.reportUsage(TotalComponentName, b.usedBytes)
	b.waiters.Broadcast()
	return nil
}

// Config returns the normalized budget configuration.
func (b *Budget) Config() Config {
	return b.config
}

// Snapshot returns a copy of the current usage state.
func (b *Budget) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()

	componentBytes := make(map[string]int64, len(b.componentBytes))
	for component, size := range b.componentBytes {
		componentBytes[component] = size
	}

	return Snapshot{
		Config:         b.config,
		UsedBytes:      b.usedBytes,
		ComponentBytes: componentBytes,
	}
}

func (b *Budget) reserveLocked(component string, size int64) {
	componentUsage := b.componentBytes[component] + size
	b.componentBytes[component] = componentUsage
	b.usedBytes += size
	b.reportUsage(component, componentUsage)
	b.reportUsage(TotalComponentName, b.usedBytes)
}

func (b *Budget) reportConfig() {
	enabledValue := int64(0)
	if b.config.Enabled {
		enabledValue = 1
	}
	metrics.LogsMemoryBudgetEnabled.Set(enabledValue)
	metrics.LogsMemoryBudgetMaxBytes.Set(b.config.MaxBytes)
	metrics.TlmLogsMemoryBudgetEnabled.Set(float64(enabledValue))
	metrics.TlmLogsMemoryBudgetMaxBytes.Set(float64(b.config.MaxBytes))
}

func (b *Budget) reportOverflow(component string, operation string) {
	metrics.LogsMemoryBudgetOverflowCount.Add(1)
	metrics.TlmLogsMemoryBudgetOverflows.Inc(component, operation)
}

func (b *Budget) reportUsage(component string, size int64) {
	if component == TotalComponentName {
		metrics.LogsMemoryBudgetBytesInUse.Set(size)
	}
	metrics.TlmLogsMemoryBudgetBytesInUse.Set(float64(size), component)
}

func normalizeConfig(config Config) Config {
	if config.OverflowPolicy != OverflowPolicyBlock && config.OverflowPolicy != OverflowPolicyDrop {
		if config.OverflowPolicy != "" {
			log.Warnf("Invalid %s: %q should be one of [block, drop], defaulting to block", OverflowPolicyConfigKey, config.OverflowPolicy)
		}
		config.OverflowPolicy = OverflowPolicyBlock
	}
	if config.MaxBytes < 0 {
		log.Warnf("Invalid %s: %d should be greater than or equal to 0, defaulting to 0", MaxBytesConfigKey, config.MaxBytes)
		config.MaxBytes = 0
	}
	if config.Enabled && config.MaxBytes == 0 {
		log.Warnf("%s is enabled but %s is 0; disabling the logs memory budget", EnabledConfigKey, MaxBytesConfigKey)
		config.Enabled = false
	}
	return config
}

func validateReservationSize(size int64) error {
	// TODO: add a max reservation?
	if size < 0 {
		return ErrInvalidReservationSize
	}
	return nil
}
