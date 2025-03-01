// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package model

// NoisyNeighborStats contains the statistics from the noisy neighbor check
type NoisyNeighborStats struct {
	Name       string
	MaxLatency uint64

	// TODO noisy: structure of JSON stats that are sent from system-probe to core agent check go here
}
