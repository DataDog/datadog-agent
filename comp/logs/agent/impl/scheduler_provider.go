// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package agentimpl

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
)

// SchedulerProvider Provides a scheduler for the log Agent.
type SchedulerProvider struct {
	compdef.Out

	Scheduler schedulers.Scheduler `group:"log-agent-scheduler"`
}

// NewSchedulerProvider returns a new SchedulerProvider.
func NewSchedulerProvider(scheduler schedulers.Scheduler) SchedulerProvider {
	return SchedulerProvider{
		Scheduler: scheduler,
	}
}
