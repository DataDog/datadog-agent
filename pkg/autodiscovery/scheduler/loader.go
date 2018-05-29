// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package scheduler

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

// Catalog holds available schedulers
type Catalog map[string]Scheduler

// DefaultCatalog holds every compiled-in diagnosis
var DefaultCatalog = make(Catalog)

// Register a diagnosis that will be called on diagnose
func Register(name string, s Scheduler) {
	if _, ok := DefaultCatalog[name]; ok {
		log.Warnf("Scheduler %s already registered, overriding it", name)
	}
	DefaultCatalog[name] = s
}

// Scheduler should return an error to report its health
type Scheduler interface {
	ScheduleConfigs([]integration.Config)
	UnscheduleConfigs([]integration.Config)
	Stop()
}
