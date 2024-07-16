// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && linux_bpf

package tracer

import "github.com/DataDog/datadog-agent/pkg/network/usm"

// RemoveClient stops tracking stateful data for a given client
func (t *Tracer) RemoveClient(clientID string) {
	t.state.RemoveClient(clientID)
}

// USMMonitor returns the USM monitor field
func (t *Tracer) USMMonitor() *usm.Monitor {
	return t.usmMonitor
}
