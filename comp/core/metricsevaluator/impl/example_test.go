// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metricsevaluatorimpl

import (
	"fmt"
	"time"

	metricsevaluator "github.com/DataDog/datadog-agent/comp/core/metricsevaluator/def"
)

// This example alerts when the node Agent uses more than 90% of its container memory limit, on average over
// 5 minutes. Both gauges are recorded by the container checks (see pkg/collector/corechecks/containers/agentperformance)
// for the Datadog Agent pods running on the node, so they only have values on Kubernetes.
func Example_agentMemoryUsage() {
	// In a component, the evaluator is injected through the Requires struct.
	var evaluator metricsevaluator.Component

	err := evaluator.Register(metricsevaluator.Monitor{
		Name:     "agent_memory_usage",
		Metric:   "agent_performance__memory_usage",
		DivideBy: "agent_performance__memory_limit",
		// Only the node Agent pod; the cluster Agent and cluster checks runners have their own series.
		Tags:        map[string]string{"kind": "agent"},
		Window:      5 * time.Minute,
		Aggregation: metricsevaluator.AggregationAvg,
		Comparator:  metricsevaluator.ComparatorAbove,
		Threshold:   0.9,
		Callback: func(ev metricsevaluator.Event) {
			switch ev.State {
			case metricsevaluator.StateAlert:
				fmt.Printf("agent memory usage is %.0f%% of its limit\n", ev.Value*100)
			case metricsevaluator.StateOK:
				fmt.Println("agent memory usage is back under 90% of its limit")
			}
		},
	})
	if err != nil {
		fmt.Println(err)
	}
}
