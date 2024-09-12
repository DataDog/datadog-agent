// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package snmpscanimpl implements the snmpscan component interface
package snmpscanimpl

import (
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
)

// Provides defines the output of the snmpscan component
type Provides struct {
	Comp       snmpscan.Component
	RCListener rcclienttypes.TaskListenerProvider
}

// NewComponent creates a new snmpscan component
func NewComponent(reqs snmpscan.Requires) (Provides, error) {
	scanner := snmpScannerImpl{
		log:   reqs.Logger,
		demux: reqs.Demultiplexer,
	}
	provides := Provides{
		Comp: scanner,
	}
	return provides, nil
}

type snmpScannerImpl struct {
	log   log.Component
	demux demultiplexer.Component
}
