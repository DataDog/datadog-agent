// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package modules

import "github.com/DataDog/datadog-agent/pkg/ebpf"

// GetBTFLoaderInfo Returns where the ebpf BTF files were sourced from, only on linux
func GetBTFLoaderInfo() (string, error) {
	return ebpf.GetBTFLoaderInfo()
}
