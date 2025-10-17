// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

type loadedProgram struct {
	programID ir.ProgramID
	tenantID  tenantID
	loaded    LoadedProgram
}

type attachedProgram struct {
	*loadedProgram
	processID       ProcessID
	attachedProgram AttachedProgram
}
