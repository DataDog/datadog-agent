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
		"has no successful probes, contains %d issue(s)", len(e.Issues),
	)
}

// Format implements the fmt.Formatter interface.
func (e *NoSuccessfulProbesError) Format(f fmt.State, _ rune) {
	fmt.Fprintf(f, "has no successful probes, %d issue(s): ", len(e.Issues))
	for i := 0; i < min(len(e.Issues), 3); i++ {
		fmt.Fprintf(f, "%s: %s", e.Issues[i].Kind.String(), e.Issues[i].Message)
		if i < len(e.Issues)-1 {
			fmt.Fprintf(f, ", ")
		}
	}
	if len(e.Issues) > 3 {
		fmt.Fprintf(f, "...")
	}
}
