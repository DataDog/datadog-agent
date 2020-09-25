// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux,!linux_bpf

package probes

import "github.com/DataDog/ebpf/manager"

// AllProbes returns the list of probes of the runtime security module
func AllProbes() []*manager.Probe {
	return []*manager.Probe{}
}
