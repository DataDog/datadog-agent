// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traps

import (
	logsConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Scheduler creates a single source to handle SNMP traps, if that is enabled.
type Scheduler struct{}

var _ schedulers.Scheduler = &Scheduler{}

// New creates a new scheduler.
//
// Note that this must be called _after_ the snmp server instance is created, or
// no logging will take place.
func New() schedulers.Scheduler {
	return &Scheduler{}
}

// Start implements schedulers.Scheduler#Start.
func (s *Scheduler) Start(sourceMgr schedulers.SourceManager) {
	if traps.IsEnabled() && traps.IsRunning() {
		// source to forward SNMP traps as logs.
		source := logsConfig.NewLogSource(logsConfig.SnmpTraps, &logsConfig.LogsConfig{
			Type:    logsConfig.SnmpTrapsType,
			Service: "snmp-traps",
			Source:  "snmp-traps",
		})
		log.Debug("Adding SNMPTraps source to the Logs Agent")
		sourceMgr.AddSource(source)
	}
}

// Stop implements schedulers.Scheduler#Stop.
func (s *Scheduler) Stop() {}
