// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// Scheduler is the interface that should be implemented if you want to schedule and
// unschedule integrations.  Values implementing this interface can be passed to the
// MetaScheduler's Register method (or AutoConf.AddScheduler).
type Scheduler interface {
	// Schedule zero or more new configurations.
	Schedule([]integration.Config)

	// Unschedule zero or more configurations that were previously scheduled.
	Unschedule([]integration.Config)

	// Stop the scheduler.  This method is called from the MetaScheduler's Stop
	// method.  Note that currently-scheduled configs are _not_ unscheduled when
	// the MetaScheduler stops.
	Stop()
}
