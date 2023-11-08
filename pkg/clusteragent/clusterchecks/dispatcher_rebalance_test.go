// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config"
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
			// Tests have been written with this value hardcoded
			// Changing the values rather than re-writing all the tests.
			originalCheckExecutionTimeWeight := checkExecutionTimeWeight
			originalMetricSamplesWeight := checkMetricSamplesWeight
			checkExecutionTimeWeight = 0.8
			checkMetricSamplesWeight = 0.2
			defer func() {
				checkExecutionTimeWeight = originalCheckExecutionTimeWeight
				checkMetricSamplesWeight = originalMetricSamplesWeight
			}()

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
			dispatcher.rebalance(false)

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
		id     checkid.ID
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
			id := checkid.BuildID(tc.check.config.Name, tc.check.config.FastDigest(), tc.check.config.Instances[0], tc.check.config.InitConfig)

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

func TestCalculateAvg(t *testing.T) {
	// checkMetricSamplesWeight affects this test. To avoid coupling this test
	// with the actual value, overwrite here and restore after the test.
	originalMetricSamplesWeight := checkMetricSamplesWeight
	checkMetricSamplesWeight = 1
	defer func() {
		checkMetricSamplesWeight = originalMetricSamplesWeight
	}()

	testDispatcher := newDispatcher()

	// The busyness of this node is 3 (1 + 2)
	testDispatcher.store.nodes["node1"] = newNodeStore("node1", "")
	testDispatcher.store.nodes["node1"].clcRunnerStats = types.CLCRunnersStats{
		"check1": types.CLCRunnerStats{
			MetricSamples:  1,
			IsClusterCheck: true,
		},
		"check2": types.CLCRunnerStats{
			MetricSamples:  2,
			IsClusterCheck: true,
		},
	}

	// The busyness of this node is 7 (3 + 4)
	testDispatcher.store.nodes["node2"] = newNodeStore("node2", "")
	testDispatcher.store.nodes["node2"].clcRunnerStats = types.CLCRunnersStats{
		"check3": types.CLCRunnerStats{
			MetricSamples:  3,
			IsClusterCheck: true,
		},
		"check4": types.CLCRunnerStats{
			MetricSamples:  4,
			IsClusterCheck: true,
		},
	}

	avg, err := testDispatcher.calculateAvg()
	require.NoError(t, err)
	assert.Equal(t, 5, avg)
}

func TestRebalanceUsingUtilization(t *testing.T) {
	// To simplify the test:
	//   - Fill the store manually to avoid having to fetch the information from
	//   the API exposed by the check runners.
	//   - Use basic example with only 3 checks and 2 runners because there are
	//   other tests specific for the checksDistribution struct that test more
	//   complex scenarios.

	testDispatcher := newDispatcher()

	testDispatcher.store.active = true
	testDispatcher.store.nodes["node1"] = newNodeStore("node1", "")
	testDispatcher.store.nodes["node1"].workers = config.DefaultNumWorkers
	testDispatcher.store.nodes["node2"] = newNodeStore("node2", "")
	testDispatcher.store.nodes["node2"].workers = config.DefaultNumWorkers

	testDispatcher.store.nodes["node1"].clcRunnerStats = map[string]types.CLCRunnerStats{
		// This is the check with the highest utilization. The code will try to
		// place this one first, but it'll give precedence to the node where the
		// check is already running, so the check won't move.
		"check1": {
			AverageExecutionTime: 3000,
			IsClusterCheck:       true,
		},
		// This check should be moved.
		"check2": {
			AverageExecutionTime: 2000,
			IsClusterCheck:       true,
		},
		// This check is not a cluster check, so it won't be moved.
		"check3": {
			AverageExecutionTime: 1000,
			IsClusterCheck:       false,
		},
	}

	// check3 not included because it's a cluster check.
	testDispatcher.store.idToDigest = map[checkid.ID]string{
		"check1": "digest1",
		"check2": "digest2",
	}
	testDispatcher.store.digestToConfig = map[string]integration.Config{
		"digest1": {},
		"digest2": {},
	}
	testDispatcher.store.digestToNode = map[string]string{
		"digest1": "node1",
		"digest2": "node1",
	}

	checksMoved := testDispatcher.rebalanceUsingUtilization(false)

	requireNotLocked(t, testDispatcher.store)

	// Check that the internal state has been updated
	expectedStatsNode1 := types.CLCRunnersStats{
		"check1": {
			AverageExecutionTime: 3000,
			IsClusterCheck:       true,
		},
		"check3": {
			AverageExecutionTime: 1000,
			IsClusterCheck:       false,
		},
	}
	expectedStatsNode2 := types.CLCRunnersStats{
		"check2": {
			AverageExecutionTime: 2000,
			IsClusterCheck:       true,
		},
	}
	assert.Equal(t, expectedStatsNode1, testDispatcher.store.nodes["node1"].clcRunnerStats)
	assert.Equal(t, expectedStatsNode2, testDispatcher.store.nodes["node2"].clcRunnerStats)

	// Check response
	require.Len(t, checksMoved, 1)
	assert.Equal(t, "check2", checksMoved[0].CheckID)
	assert.Equal(t, "node1", checksMoved[0].SourceNodeName)
	assert.Equal(t, "node2", checksMoved[0].DestNodeName)

	// The next rebalance should not move anything because there were not any
	// changes in the checks stats.
	checksMoved = testDispatcher.rebalanceUsingUtilization(false)
	assert.Empty(t, checksMoved)
}

func TestRebalanceIsWorthIt(t *testing.T) {
	workersPerRunner := map[string]int{
		"runner1": 3,
		"runner2": 3,
		"runner3": 3,
	}

	// The proposed solution is worth it if it leaves less unused runners

	currentDistribution := newChecksDistribution(workersPerRunner)
	currentDistribution.addCheck("check1", 1, "runner1")
	currentDistribution.addCheck("check2", 1, "runner1")

	proposedDistribution := newChecksDistribution(workersPerRunner)
	proposedDistribution.addCheck("check1", 1, "runner1")
	proposedDistribution.addCheck("check2", 1, "runner2")

	assert.True(t, rebalanceIsWorthIt(currentDistribution, proposedDistribution, 10))

	// The proposed	solution is worth it if it has fewer runners with a high utilization
	currentDistribution = newChecksDistribution(workersPerRunner)
	currentDistribution.addCheck("check1", 1, "runner1")
	currentDistribution.addCheck("check2", 1, "runner1")
	currentDistribution.addCheck("check3", 1, "runner1")

	proposedDistribution = newChecksDistribution(workersPerRunner)
	proposedDistribution.addCheck("check1", 1, "runner1")
	proposedDistribution.addCheck("check2", 1, "runner2")
	proposedDistribution.addCheck("check3", 1, "runner3")

	assert.True(t, rebalanceIsWorthIt(currentDistribution, proposedDistribution, 10))
}
