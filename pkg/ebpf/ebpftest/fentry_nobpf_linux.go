// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !bpf

package ebpftest

// supportsFentry always reports false when the bpf tag is not set: no eBPF
// programs (fentry included) can be loaded in this configuration.
func supportsFentry() bool {
	return false
}
