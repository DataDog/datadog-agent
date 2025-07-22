// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
)

// ProcessesUpdate is a set of updates to the actuator's state.
//
// Note that this does not represent a complete update to all of the
// process state, but rather an incremental update to the set of processes.
type ProcessesUpdate struct {
	// Processes is a list of updates to the processes's instrumentation
	// configuration.
	Processes []ProcessUpdate

	// Removals is a list of process IDs that are no longer being instrumented.
	Removals []ProcessID
}

// ProcessUpdate is an update to a process's instrumentation configuration.
type ProcessUpdate struct {
	ProcessID  ProcessID
	Executable Executable

	// Probes is the *complete* set of probes for the process.
	//
	// If a previous update contained a different set of probes, they
	// will be wholly replaced by the new set.
	Probes []ir.ProbeDefinition
}

// Executable forwards the definition from procmon.
type Executable = procmon.Executable

// ProcessID forwards the definition from procmon.
type ProcessID = procmon.ProcessID
