// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

type danglingConfigWrapper struct {
	config                   integration.Config
	rescheduleAttempts       int
	detectedExtendedDangling bool
}

// createDanglingConfig creates a new danglingConfigWrapper
// This is used to keep track of the lifecycle of a dangling config
func createDanglingConfig(config integration.Config) *danglingConfigWrapper {
	return &danglingConfigWrapper{
		config:                   config,
		rescheduleAttempts:       0,
		detectedExtendedDangling: false,
	}
}

// isStuckScheduling returns true if the config has been attempted
// rescheduling more than attemptLimit times
func (c *danglingConfigWrapper) isStuckScheduling(attemptLimit int) bool {
	return c.rescheduleAttempts > attemptLimit
}
