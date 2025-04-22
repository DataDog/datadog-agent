// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package protocols

import (
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
	"github.com/DataDog/sketches-go/ddsketch"
)

// RelativeAccuracy defines the acceptable error in quantile values calculated by DDSketch.
// For example, if the actual value at p50 is 100, with a relative accuracy of 0.01 the value calculated
// will be between 99 and 101
const RelativeAccuracy = 0.01

var (
	// SketchesPool is a pool of DDSketches
	SketchesPool = ddsync.NewTypedPool[ddsketch.DDSketch](func() *ddsketch.DDSketch {
		latencies, err := ddsketch.NewDefaultDDSketch(RelativeAccuracy)
		if err != nil {
			return nil
		}
		return latencies
	})
)
