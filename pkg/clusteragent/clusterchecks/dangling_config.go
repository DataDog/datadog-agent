// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

type danglingConfigWrapper struct {
	config                   integration.Config
	time                     time.Time
	detectedExtendedDangling bool
}

// createConfigEntry creates a new integrationConfigEntry
// This is used to keep track of the time a config was added to the store
func createConfigEntry(config integration.Config) *danglingConfigWrapper {
	return &danglingConfigWrapper{
		config:                   config,
		time:                     time.Now(),
		detectedExtendedDangling: false,
	}
}

// isStuckScheduling returns true if the config has been in the
// store for longer than expectedScheduleTime
func (e *danglingConfigWrapper) isStuckScheduling(expectedScheduleTime time.Duration) bool {
	return time.Since(e.time) > expectedScheduleTime
}
