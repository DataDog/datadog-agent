// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package adschedulerimpl contains the AD scheduler implementation.
package adschedulerimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	logsadscheduler "github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"
)

// Requires defines the dependencies for the adscheduler component.
type Requires struct {
	Autodiscovery autodiscovery.Component
}

// Provides defines the output of the adscheduler component.
type Provides struct {
	compdef.Out

	Scheduler schedulers.Scheduler `group:"log-agent-scheduler"`
}

// NewComponent creates a new adscheduler component.
func NewComponent(reqs Requires) Provides {
	scheduler := logsadscheduler.New(reqs.Autodiscovery)
	return Provides{
		Scheduler: scheduler,
	}
}
