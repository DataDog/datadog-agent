// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux_bpf

package ebpf

import "errors"

// GetBTFLoaderInfo is not supported without linux_bpf
func GetBTFLoaderInfo() (string, error) {
	return "", errors.New("BTF is not supported")
}
