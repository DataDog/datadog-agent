// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package wire

import (
	"testing"

	noops "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
)

var benchmarkValue float64

func BenchmarkParseMetricValues(b *testing.B) {
	for _, enabled := range []bool{false, true} {
		mode := "telemetry_off"
		if enabled {
			mode = "telemetry_on_noop"
		}
		b.Run(mode, func(b *testing.B) {
			for _, message := range []string{"metric:17|g|#a:b", "metric:17:23|g|#a:b"} {
				b.Run(message, func(b *testing.B) {
					telemetry := noops.GetCompatComponent()
					pool := NewFloat64ListPool(enabled, telemetry)
					p := NewParser(Options{InternerSize: 4096, ReadTimestamps: true}, pool, 1, NewInternerTelemetry(enabled, telemetry))
					payload := []byte(message)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						sample, err := p.ParseMetric(payload)
						if err != nil {
							b.Fatal(err)
						}
						benchmarkValue = sample.Value + float64(len(sample.Values))
						if len(sample.Values) > 0 {
							pool.Put(sample.Values)
						}
					}
				})
			}
		})
	}
}
