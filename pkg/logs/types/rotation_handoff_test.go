// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRotationHandoffSettings(t *testing.T) {
	settings := DefaultRotationHandoffSettings()
	assert.Equal(t, RotationHandoffModeParallel, settings.Mode)
	assert.Equal(t, 2, settings.QuietPeriodSeconds)
	assert.Equal(t, 30, settings.MaxDrainSeconds)
}

func TestValidateRotationHandoffSettings(t *testing.T) {
	t.Run("parallel default", func(t *testing.T) {
		err := ValidateRotationHandoffSettings(&RotationHandoffSettings{Mode: RotationHandoffModeParallel})
		require.NoError(t, err)
	})

	t.Run("sequential valid", func(t *testing.T) {
		err := ValidateRotationHandoffSettings(&RotationHandoffSettings{
			Mode:               RotationHandoffModeSequential,
			QuietPeriodSeconds: 2,
			MaxDrainSeconds:    30,
		})
		require.NoError(t, err)
	})

	t.Run("reject quiet >= max_drain", func(t *testing.T) {
		err := ValidateRotationHandoffSettings(&RotationHandoffSettings{
			Mode:               RotationHandoffModeSequential,
			QuietPeriodSeconds: 30,
			MaxDrainSeconds:    30,
		})
		require.Error(t, err)
	})

	t.Run("reject zero quiet", func(t *testing.T) {
		err := ValidateRotationHandoffSettings(&RotationHandoffSettings{
			Mode:               RotationHandoffModeSequential,
			QuietPeriodSeconds: 0,
			MaxDrainSeconds:    30,
		})
		require.Error(t, err)
	})

	t.Run("reject invalid mode", func(t *testing.T) {
		err := ValidateRotationHandoffSettings(&RotationHandoffSettings{Mode: "invalid"})
		require.Error(t, err)
	})

	t.Run("normalize durations", func(t *testing.T) {
		settings := &RotationHandoffSettings{
			Mode:        RotationHandoffModeSequential,
			QuietPeriod: 2 * time.Second,
			MaxDrain:    15 * time.Second,
		}
		require.NoError(t, ValidateRotationHandoffSettings(settings))
		assert.Equal(t, 2, settings.QuietPeriodSeconds)
		assert.Equal(t, 15, settings.MaxDrainSeconds)
	})
}
