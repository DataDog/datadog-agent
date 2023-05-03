// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ErrNoProcessContext defines an error for event without process context
type ErrNoProcessContext struct {
	Err error
}

// Error implements the error interface
func (e *ErrNoProcessContext) Error() string {
	return e.Err.Error()
}

// Unwrap implements the error interface
func (e *ErrNoProcessContext) Unwrap() error {
	return e.Err
}

// ErrProcessBrokenLineage returned when a process lineage is broken
type ErrProcessBrokenLineage struct {
	PIDContext model.PIDContext
}

// Error implements the error interface
func (e *ErrProcessBrokenLineage) Error() string {
	return fmt.Sprintf("broken process lineage, pid: %d", e.PIDContext.Pid)
}
