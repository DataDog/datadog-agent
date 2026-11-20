// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metricsevaluatorimpl

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricsevaluator "github.com/DataDog/datadog-agent/comp/core/metricsevaluator/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

// BenchmarkEvaluate measures one evaluation tick, for registries of increasing size and an increasing number of
// monitors. Telemetry.Gather collects the whole registry before filtering, so both dimensions matter.
func BenchmarkEvaluate(b *testing.B) {
	for _, series := range []int{100, 1000, 10000} {
		for _, monitors := range []int{1, 10, 100} {
			b.Run(fmt.Sprintf("series=%d/monitors=%d", series, monitors), func(b *testing.B) {
				tel := telemetryimpl.NewMock(b)
				// 10 series per metric family.
				for f := 0; f < series/10; f++ {
					g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "metric_" + strconv.Itoa(f)}, []string{"tag"})
					tel.GetRegistry().MustRegister(g)
					for s := 0; s < 10; s++ {
						g.WithLabelValues(strconv.Itoa(s)).Set(float64(s))
					}
				}

				e := newEvaluator(logmock.New(b), tel, time.Minute)
				for m := 0; m < monitors; m++ {
					err := e.Register(metricsevaluator.Monitor{
						Name:        "monitor_" + strconv.Itoa(m),
						Metric:      "metric_" + strconv.Itoa(m%(series/10)),
						Tags:        map[string]string{"tag": "1"},
						Window:      5 * time.Minute,
						Aggregation: metricsevaluator.AggregationAvg,
						Comparator:  metricsevaluator.ComparatorAbove,
						Threshold:   0,
						Callback:    func(metricsevaluator.Event) {},
					})
					if err != nil {
						b.Fatal(err)
					}
				}

				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					e.evaluate(time.Now())
				}
			})
		}
	}
}
