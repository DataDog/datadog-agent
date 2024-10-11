// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package snmpscanimpl implements the snmpscan component interface
package snmpscanimpl

import (
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
)

// Requires defines the dependencies for the snmpscan component
type Requires struct {
	compdef.In
	Logger        log.Component
	Demultiplexer demultiplexer.Component
}

// Provides defines the output of the snmpscan component
type Provides struct {
	Comp       snmpscan.Component
	RCListener rcclienttypes.TaskListenerProvider
}

// NewComponent creates a new snmpscan component
func NewComponent(reqs Requires) (Provides, error) {
	forwarder, err := reqs.Demultiplexer.GetEventPlatformForwarder()
	if err != nil {
		return Provides{}, err
	}
	scanner := snmpScannerImpl{
		log:         reqs.Logger,
		epforwarder: forwarder,
	}
	provides := Provides{
		Comp: scanner,
	}
	return provides, nil
}

type snmpScannerImpl struct {
	log         log.Component
	epforwarder eventplatform.Forwarder
}
