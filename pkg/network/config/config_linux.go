// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	cebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
)

// RingBufferSupportedNPM returns true if the kernel supports ring buffers and the config enables them
func (c *Config) RingBufferSupportedNPM() bool {
	return (features.HaveMapType(cebpf.RingBuf) == nil) && c.NPMRingbuffersEnabled
}
