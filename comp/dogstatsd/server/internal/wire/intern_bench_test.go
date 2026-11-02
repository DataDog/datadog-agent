// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package wire

import (
	"fmt"
	"testing"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

func BenchmarkLoadOrStoreReset(b *testing.B) {
	telemetryComp := telemetryimpl.NewMock(b)
	// benchmark with the internal telemetry enabled
	internerTelemetry := NewInternerTelemetry(true, telemetryComp)

	sInterner := newStringInterner(4, 1, internerTelemetry)

	list := []string{}
	for i := 0; i < 512; i++ {
		list = append(list, fmt.Sprintf("testing.metric%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sInterner.LoadOrStore([]byte(list[i%len(list)]))
	}
}
