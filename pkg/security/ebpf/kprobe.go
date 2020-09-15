// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package ebpf

// KProbe describes a Linux Kprobe
type KProbe struct {
	Name      string
	EntryFunc string
	ExitFunc  string
}
