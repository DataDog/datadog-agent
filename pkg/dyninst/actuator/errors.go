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

// LoadingFailedError is returned when preparing a program fails.
//
// It unifies failures across IR generation, zero-successful-probes,
// program compilation/loading, and handler/dataplane setup. It carries the
// complete set of requested probes and optional structured issues when IR was
// successfully produced but contained no successful probes.
type LoadingFailedError struct {
	// Err is the underlying error.
	Err error

	// Probes is the complete set of probes requested for this load.
	Probes []ir.ProbeDefinition

	// Issues describes per-probe issues when IR contained no successful probes.
	// It may be empty for other failure kinds.
	Issues []ir.ProbeIssue
}

// Error implements the error interface.
func (e *LoadingFailedError) Error() string {
	if e == nil || e.Err == nil {
		return "loading failed"
	}
	return e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *LoadingFailedError) Unwrap() error { return e.Err }

// Format implements fmt.Formatter for a concise summary that includes up to
// three issues when present.
func (e *LoadingFailedError) Format(f fmt.State, _ rune) {
	if e == nil {
		fmt.Fprintf(f, "loading failed")
		return
	}
	if e.Err != nil {
		fmt.Fprintf(f, "loading failed: %v", e.Err)
	} else {
		fmt.Fprintf(f, "loading failed")
	}
	if len(e.Issues) > 0 {
		fmt.Fprintf(f, ", %d issue(s): ", len(e.Issues))
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
}
