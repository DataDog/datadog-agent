// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"errors"
	"fmt"
	"time"
)

// RotationHandoffMode controls whether replacement tailers wait for draining
// tailers on the same pathname before opening.
type RotationHandoffMode string

const (
	// RotationHandoffModeParallel is the default: replacement may open while the old tailer drains.
	RotationHandoffModeParallel RotationHandoffMode = "parallel"
	// RotationHandoffModeSequential blocks post-detection opens until all draining tailers finish.
	RotationHandoffModeSequential RotationHandoffMode = "sequential"
)

// Default sequential rotation handoff timing knobs.
const (
	DefaultSequentialRotationQuietPeriodSeconds = 2
	DefaultSequentialRotationMaxDrainSeconds    = 30
)

// RotationHandoffSettings holds resolved rotation handoff configuration.
type RotationHandoffSettings struct {
	Mode        RotationHandoffMode `json:"rotation_handoff_mode" mapstructure:"rotation_handoff_mode" yaml:"rotation_handoff_mode"`
	QuietPeriod time.Duration       `json:"-"`
	MaxDrain    time.Duration       `json:"-"`
	// QuietPeriodSeconds and MaxDrainSeconds are the wire/config forms.
	QuietPeriodSeconds int `json:"sequential_rotation_quiet_period" mapstructure:"sequential_rotation_quiet_period" yaml:"sequential_rotation_quiet_period"`
	MaxDrainSeconds    int `json:"sequential_rotation_max_drain" mapstructure:"sequential_rotation_max_drain" yaml:"sequential_rotation_max_drain"`
}

// DefaultRotationHandoffSettings returns production defaults (parallel mode).
func DefaultRotationHandoffSettings() RotationHandoffSettings {
	return RotationHandoffSettings{
		Mode:               RotationHandoffModeParallel,
		QuietPeriodSeconds: DefaultSequentialRotationQuietPeriodSeconds,
		MaxDrainSeconds:    DefaultSequentialRotationMaxDrainSeconds,
		QuietPeriod:        DefaultSequentialRotationQuietPeriodSeconds * time.Second,
		MaxDrain:           DefaultSequentialRotationMaxDrainSeconds * time.Second,
	}
}

// NormalizeDurations copies second fields into Duration fields when unset.
func (s *RotationHandoffSettings) NormalizeDurations() {
	if s.QuietPeriod == 0 && s.QuietPeriodSeconds > 0 {
		s.QuietPeriod = time.Duration(s.QuietPeriodSeconds) * time.Second
	}
	if s.MaxDrain == 0 && s.MaxDrainSeconds > 0 {
		s.MaxDrain = time.Duration(s.MaxDrainSeconds) * time.Second
	}
}

// ValidateRotationHandoffSettings validates rotation handoff settings.
func ValidateRotationHandoffSettings(settings *RotationHandoffSettings) error {
	if settings == nil {
		return nil
	}
	settings.NormalizeDurations()
	switch settings.Mode {
	case "", RotationHandoffModeParallel:
		return nil
	case RotationHandoffModeSequential:
		if settings.QuietPeriodSeconds <= 0 && settings.QuietPeriod <= 0 {
			return errors.New("sequential_rotation_quiet_period must be greater than 0 when rotation_handoff_mode is sequential")
		}
		if settings.MaxDrainSeconds <= 0 && settings.MaxDrain <= 0 {
			return errors.New("sequential_rotation_max_drain must be greater than 0 when rotation_handoff_mode is sequential")
		}
		if settings.QuietPeriodSeconds <= 0 {
			settings.QuietPeriodSeconds = int(settings.QuietPeriod / time.Second)
		}
		if settings.MaxDrainSeconds <= 0 {
			settings.MaxDrainSeconds = int(settings.MaxDrain / time.Second)
		}
		if settings.QuietPeriodSeconds >= settings.MaxDrainSeconds {
			return fmt.Errorf("sequential_rotation_quiet_period (%d) must be less than sequential_rotation_max_drain (%d)",
				settings.QuietPeriodSeconds, settings.MaxDrainSeconds)
		}
		return nil
	default:
		return fmt.Errorf("rotation_handoff_mode must be parallel or sequential, got %q", settings.Mode)
	}
}
