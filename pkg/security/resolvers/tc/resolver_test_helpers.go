// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package tc

import (
	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
)

// AddTestProgram registers a program as SetupNewTCClassifierWithNetNSHandle does after a successful clone
func (tcr *Resolver) AddTestProgram(key ProgramKey, programID uint32) {
	tcr.Lock()
	defer tcr.Unlock()

	tcr.programs[key] = programEntry{
		programID: programID,
		probe: &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: key.Key(), EBPFFuncName: key.FuncName},
		},
	}
	ddebpf.AddProgramNameMapping(programID, key.FuncName, "cws")
}
