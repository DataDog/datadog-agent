// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package scheduler

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// ActiveSchedulers holds every registered scheduler
var ActiveSchedulers = make(map[string]Scheduler)

// Register a scheduler in the scheduler catalog, the meta scheduler in
// autodiscovery will dispatch to every registered scheduler
func Register(name string, s Scheduler) {
	if _, ok := ActiveSchedulers[name]; ok {
		log.Warnf("Scheduler %s already registered, overriding it", name)
	}
	ActiveSchedulers[name] = s
}

// Scheduler is the interface that should be implemented if you want to schedule and
// unschedule integrations
type Scheduler interface {
	Schedule([]integration.Config)
	Unschedule([]integration.Config)
	Stop()
}
