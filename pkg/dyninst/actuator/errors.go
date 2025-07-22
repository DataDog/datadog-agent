// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// NoSuccessfulProbesError is returned when a program has no successful probes.
type NoSuccessfulProbesError struct {
	// Issues contains the issues that caused the program to have no
	// successful probes.
	Issues []ir.ProbeIssue
}

// Error implements the error interface.
func (e *NoSuccessfulProbesError) Error() string {
	return fmt.Sprintf(
		"has no successful probes, contains %d issues", len(e.Issues),
	)
}
