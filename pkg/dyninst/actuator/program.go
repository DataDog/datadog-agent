// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
)

type loadedProgram struct {
	tenantID tenantID
	program  loader.Program
	ir       *ir.Program
	sink     Sink
}

type attachedProgram struct {
	ir             *ir.Program
	procID         ProcessID
	tenantID       tenantID
	executableLink *link.Executable
	attachedLinks  []link.Link
}
