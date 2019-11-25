// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package scheduler

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// Scheduler is the interface that should be implemented if you want to schedule and
// unschedule integrations
type Scheduler interface {
	Schedule([]integration.Config)
	Unschedule([]integration.Config)
	Stop()
}
