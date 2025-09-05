// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clustercheckimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

type danglingConfigWrapper struct {
	config           integration.Config
	timeCreated      time.Time
	unscheduledCheck bool
}

// createDanglingConfig creates a new danglingConfigWrapper
// This is used to keep track of the lifecycle of a dangling config
func createDanglingConfig(config integration.Config) *danglingConfigWrapper {
	return &danglingConfigWrapper{
		config:           config,
		timeCreated:      time.Now(),
		unscheduledCheck: false,
	}
}

// isStuckScheduling returns true if the config has been in the store
// for longer than the unscheduledCheckThresholdSeconds
func (c *danglingConfigWrapper) isStuckScheduling(unscheduledCheckThresholdSeconds int64) bool {
	expectCheckIsScheduledTime := c.timeCreated.Add(time.Duration(unscheduledCheckThresholdSeconds) * time.Second)
	return time.Now().After(expectCheckIsScheduledTime)
}
