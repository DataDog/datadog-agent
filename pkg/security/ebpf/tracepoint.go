// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package ebpf

import (
	"fmt"
)

// RegisterTracepoint registers a kernel tracepoint
func (m *Module) RegisterTracepoint(name string) error {
	if err := m.EnableTracepoint(name); err != nil {
		return fmt.Errorf("failed to load tracepoint %v: %s", name, err)
	}

	return nil
}
