// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package agent

import (
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"go.uber.org/fx"
)

// SchedulerProvider provides a scheduler for the log Agent.
type SchedulerProvider struct {
	fx.Out

	Scheduler schedulers.Scheduler `group:"log-agent-scheduler"`
}

// NewSchedulerProvider returns a new SchedulerProvider.
func NewSchedulerProvider(scheduler schedulers.Scheduler) SchedulerProvider {
	return SchedulerProvider{
		Scheduler: scheduler,
	}
}
