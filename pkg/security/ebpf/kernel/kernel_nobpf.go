// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !linux_bpf

// Package kernel holds kernel related files
package kernel

// SupportCORE returns is CORE is supported (here it's not, since we are built without eBPF support)
func (k *Version) SupportCORE() bool {
	return false
}
