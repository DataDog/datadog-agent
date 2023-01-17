// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks
// +build clusterchecks

package clusterchecks

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/collector/check"

	"github.com/stretchr/testify/assert"
)

func TestRebalance(t *testing.T) {
	for i, tc := range []struct {
		in  map[string]*nodeStore
		out map[string]*nodeStore
	}{
		{
			in: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					name: "B",
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					name: "B",
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 200,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					name: "B",
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					name: "B",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 200,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					name: "B",
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"C": {
					name: "C",
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 5,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC1": types.CLCRunnerStats{
							AverageExecutionTime: 90,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC2": types.CLCRunnerStats{
							AverageExecutionTime: 110,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"D": {
					name: "D",
					clcRunnerStats: types.CLCRunnersStats{
						"checkD0": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					name: "B",
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"C": {
					name: "C",
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 5,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC1": types.CLCRunnerStats{
							AverageExecutionTime: 90,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC2": types.CLCRunnerStats{
							AverageExecutionTime: 110,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"D": {
					name: "D",
					clcRunnerStats: types.CLCRunnersStats{
						"checkD0": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB3": types.CLCRunnerStats{
							AverageExecutionTime: 200,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB4": types.CLCRunnerStats{
							AverageExecutionTime: 500,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"C": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC1": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"D": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkD0": types.CLCRunnerStats{
							AverageExecutionTime: 5,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkD1": types.CLCRunnerStats{
							AverageExecutionTime: 90,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkD2": types.CLCRunnerStats{
							AverageExecutionTime: 110,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"E": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkE0": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"C": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC1": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB3": types.CLCRunnerStats{
							AverageExecutionTime: 200,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"D": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkD0": types.CLCRunnerStats{
							AverageExecutionTime: 5,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkD1": types.CLCRunnerStats{
							AverageExecutionTime: 90,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkD2": types.CLCRunnerStats{
							AverageExecutionTime: 110,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"E": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkE0": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB4": types.CLCRunnerStats{
							AverageExecutionTime: 500,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB3": types.CLCRunnerStats{
							AverageExecutionTime: 500,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB4": types.CLCRunnerStats{
							AverageExecutionTime: 40,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB5": types.CLCRunnerStats{
							AverageExecutionTime: 60,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"C": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},

						"checkB4": types.CLCRunnerStats{
							AverageExecutionTime: 40,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB5": types.CLCRunnerStats{
							AverageExecutionTime: 60,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"C": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB3": types.CLCRunnerStats{
							AverageExecutionTime: 500,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{},
				},
				"C": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC1": types.CLCRunnerStats{
							AverageExecutionTime: 500,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{},
				},
				"C": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC1": types.CLCRunnerStats{
							AverageExecutionTime: 500,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"C": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"D": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkD0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkD1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkD2": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"E": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkE0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkE1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkE2": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkE3": types.CLCRunnerStats{
							AverageExecutionTime: 500,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkE3": types.CLCRunnerStats{
							AverageExecutionTime: 500,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkE2": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"C": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkC1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"D": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkD0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkD1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkD2": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"E": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkE0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkE1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"C": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        5,
							IsClusterCheck:       true,
						},
						"checkC1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        20,
							IsClusterCheck:       true,
						},
					},
				},
				"D": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkD0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        5,
							IsClusterCheck:       true,
						},
						"checkD1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        50,
							IsClusterCheck:       true,
						},
						"checkD2": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        600,
							IsClusterCheck:       true,
						},
					},
				},
				"E": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkE0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkE1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        300,
							IsClusterCheck:       true,
						},
						"checkE2": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkE3": types.CLCRunnerStats{
							AverageExecutionTime: 500,
							MetricSamples:        1000,
							IsClusterCheck:       true,
						},
					},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkE3": types.CLCRunnerStats{
							AverageExecutionTime: 500,
							MetricSamples:        1000,
							IsClusterCheck:       true,
						},
					},
				},
				"B": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkE2": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
				"C": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkC0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        5,
							IsClusterCheck:       true,
						},
						"checkC1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        20,
							IsClusterCheck:       true,
						},
					},
				},
				"D": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkD0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        5,
							IsClusterCheck:       true,
						},
						"checkD1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        50,
							IsClusterCheck:       true,
						},
						"checkD2": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        600,
							IsClusterCheck:       true,
						},
					},
				},
				"E": {
					clcRunnerStats: types.CLCRunnersStats{
						"checkE0": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkE1": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        300,
							IsClusterCheck:       true,
						},
					},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
					},
				},
				"B": {
					name: "B",
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
					},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
					},
				},
				"B": {
					name: "B",
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       true,
						},
					},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
					},
				},
				"B": {
					name: "B",
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
					},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkA2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkA3": types.CLCRunnerStats{
							AverageExecutionTime: 300,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
					},
				},
				"B": {
					name: "B",
					clcRunnerStats: types.CLCRunnersStats{
						"checkB0": types.CLCRunnerStats{
							AverageExecutionTime: 50,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkB1": types.CLCRunnerStats{
							AverageExecutionTime: 20,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
						"checkB2": types.CLCRunnerStats{
							AverageExecutionTime: 100,
							MetricSamples:        10,
							IsClusterCheck:       false,
						},
					},
				},
			},
		},
		{
			in: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 1000,
							MetricSamples:        1000,
							LastExecFailed:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							LastExecFailed:       false,
						},
					},
				},
				"B": {
					name:           "B",
					clcRunnerStats: types.CLCRunnersStats{},
				},
			},
			out: map[string]*nodeStore{
				"A": {
					name: "A",
					clcRunnerStats: types.CLCRunnersStats{
						"checkA0": types.CLCRunnerStats{
							AverageExecutionTime: 1000,
							MetricSamples:        1000,
							LastExecFailed:       true,
						},
						"checkA1": types.CLCRunnerStats{
							AverageExecutionTime: 10,
							MetricSamples:        10,
							LastExecFailed:       false,
						},
					},
				},
				"B": {
					name:           "B",
					clcRunnerStats: types.CLCRunnersStats{},
				},
			},
		},
	} {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			dispatcher := newDispatcher()

			// prepare store
			dispatcher.store.active = true
			for node, store := range tc.in {
				// init nodeStore
				dispatcher.store.nodes[node] = newNodeStore(node, "") // no need to setup the clientIP in this test
				// setup input
				dispatcher.store.nodes[node].clcRunnerStats = store.clcRunnerStats
			}

			// rebalance checks
			dispatcher.rebalance()

			// assert runner stats repartition is updated correctly
			for node, store := range tc.out {
				assert.EqualValues(t, store.clcRunnerStats, dispatcher.store.nodes[node].clcRunnerStats)
			}

			requireNotLocked(t, dispatcher.store)
		})
	}
}

func TestMoveCheck(t *testing.T) {
	type checkInfo struct {
		config integration.Config
		id     check.ID
		node   string
	}

	for i, tc := range []struct {
		nodes []string
		check checkInfo
		src   string
		dest  string
	}{
		{
			nodes: []string{
				"srcNode",
				"destNode",
			},
			check: checkInfo{
				config: integration.Config{
					Name: "check1",
					Instances: []integration.Data{
						integration.Data(""),
					},
					InitConfig: integration.Data(""),
				},
				node: "srcNode",
			},
			dest: "destNode",
		},
	} {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			dispatcher := newDispatcher()

			// setup check id
			id := check.BuildID(tc.check.config.Name, tc.check.config.FastDigest(), tc.check.config.Instances[0], tc.check.config.InitConfig)

			// prepare store
			dispatcher.store.active = true
			for _, node := range tc.nodes {
				// init nodeStore
				dispatcher.store.nodes[node] = newNodeStore(node, "") // no need to setup the clientIP in this test
			}
			dispatcher.addConfig(tc.check.config, tc.check.node)
			dispatcher.store.nodes[tc.check.node].clcRunnerStats = types.CLCRunnersStats{string(id): types.CLCRunnerStats{}}

			// move check
			err := dispatcher.moveCheck(tc.check.node, tc.dest, string(id))

			// assert no error
			assert.Nil(t, err)

			// assert checks repartition is updated correctly
			assert.EqualValues(t, tc.dest, dispatcher.store.digestToNode[tc.check.config.Digest()])
			assert.Len(t, dispatcher.store.digestToNode, 1)
			assert.EqualValues(t, tc.check.config, dispatcher.store.digestToConfig[tc.check.config.Digest()])
			assert.Len(t, dispatcher.store.digestToConfig, 1)

			// assert node store is updated correctly
			assert.EqualValues(t, tc.check.config, dispatcher.store.nodes[tc.dest].digestToConfig[tc.check.config.Digest()])
			assert.Len(t, dispatcher.store.nodes[tc.dest].digestToConfig, 1)

			requireNotLocked(t, dispatcher.store)
		})
	}
}
