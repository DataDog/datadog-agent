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

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// rebalanceTestClcRunnerClient mocks the clcRunnersClient for rebalance tests
type rebalanceTestClcRunnerClient struct {
	testStats map[string]types.CLCRunnersStats
}

func (d *rebalanceTestClcRunnerClient) GetVersion(string) (version.Version, error) {
	return version.Version{}, nil
}

func (d *rebalanceTestClcRunnerClient) GetRunnerStats(ip string) (types.CLCRunnersStats, error) {
	// Return the stored test stats for this IP, or empty if not found
	if d.testStats != nil {
		if stats, found := d.testStats[ip]; found {
			return stats, nil
		}
	}
	return types.CLCRunnersStats{}, nil
}

func (d *rebalanceTestClcRunnerClient) GetRunnerWorkers(string) (types.Workers, error) {
	// Return default worker count
	return types.Workers{Count: pkgconfigsetup.DefaultNumWorkers}, nil
}

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

			fakeTagger := taggerfxmock.SetupFakeTagger(t)
			dispatcher := newDispatcher(fakeTagger)

			// Create a mock CLC runner client to avoid nil pointer errors
			mockClient := &rebalanceTestClcRunnerClient{
				testStats: make(map[string]types.CLCRunnersStats),
			}
			dispatcher.clcRunnersClient = mockClient

			// prepare store
			dispatcher.store.active = true
			for node, store := range tc.in {
				// Give each node a unique IP so the mock can distinguish them
				nodeIP := fmt.Sprintf("10.0.0.%d", len(mockClient.testStats)+1)
				dispatcher.store.nodes[node] = newNodeStore(node, nodeIP)
				// setup input
				dispatcher.store.nodes[node].clcRunnerStats = store.clcRunnerStats
				// Store the test data in the mock so it can return it
				mockClient.testStats[nodeIP] = store.clcRunnerStats
				// Each cluster-check ID is its own single-instance config
				// (digest == checkID). Skip non-cluster checks so
				// updateRunnersStats doesn't flip their IsClusterCheck flag.
				for checkID, stats := range store.clcRunnerStats {
					if !stats.IsClusterCheck {
						continue
					}
					digest := checkID
					dispatcher.store.idToDigest[checkid.ID(checkID)] = digest
					dispatcher.store.digestToConfig[digest] = integration.Config{Name: checkID}
					dispatcher.store.digestToNode[digest] = node
				}
			}

			// Use busyness-based rebalancing directly for these legacy tests
			// (preserves test coverage for busyness algorithm while allowing utilization as default)
			dispatcher.rebalanceUsingBusyness()

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
		id     checkid.ID //nolint:unused
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
			fakeTagger := taggerfxmock.SetupFakeTagger(t)
			dispatcher := newDispatcher(fakeTagger)

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

			// move config
			err := dispatcher.moveConfig(tc.check.node, tc.dest, tc.check.config.Digest())

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

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	testDispatcher := newDispatcher(fakeTagger)

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
	//   other tests specific for the configsDistribution struct that test more
	//   complex scenarios.

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	testDispatcher := newDispatcher(fakeTagger)

	// Create a mock CLC runner client to avoid nil pointer errors
	mockClient := &rebalanceTestClcRunnerClient{
		testStats: make(map[string]types.CLCRunnersStats),
	}
	testDispatcher.clcRunnersClient = mockClient
	testDispatcher.advancedDispatching.Store(true) // Enable advanced dispatching for utilization tests

	testDispatcher.store.active = true
	testDispatcher.store.nodes["node1"] = newNodeStore("node1", "10.0.0.1")
	testDispatcher.store.nodes["node1"].workers = pkgconfigsetup.DefaultNumWorkers
	testDispatcher.store.nodes["node2"] = newNodeStore("node2", "10.0.0.2")
	testDispatcher.store.nodes["node2"].workers = pkgconfigsetup.DefaultNumWorkers

	node1Stats := map[string]types.CLCRunnerStats{
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

	// Assign the stats to node1 and store in mock
	testDispatcher.store.nodes["node1"].clcRunnerStats = node1Stats
	mockClient.testStats["10.0.0.1"] = node1Stats
	mockClient.testStats["10.0.0.2"] = types.CLCRunnersStats{} // node2 has no initial stats

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

	// Moves are reported by the moved config's digest.
	require.Len(t, checksMoved, 1)
	assert.Equal(t, "digest2", checksMoved[0].Digest)
	assert.Equal(t, "node1", checksMoved[0].SourceNodeName)
	assert.Equal(t, "node2", checksMoved[0].DestNodeName)

	// The next rebalance should not move anything because there were not any
	// changes in the checks stats.
	checksMoved = testDispatcher.rebalanceUsingUtilization(false)
	assert.Empty(t, checksMoved)
}

// Verifies the utilization algorithm treats a config's instances as a single
// unit: one entry in the distribution, and never split across runners.
func TestRebalanceUsingUtilization_GroupsInstancesByConfig(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	testDispatcher := newDispatcher(fakeTagger)

	mockClient := &rebalanceTestClcRunnerClient{
		testStats: make(map[string]types.CLCRunnersStats),
	}
	testDispatcher.clcRunnersClient = mockClient
	testDispatcher.advancedDispatching.Store(true)

	testDispatcher.store.active = true
	testDispatcher.store.nodes["node1"] = newNodeStore("node1", "10.0.0.1")
	testDispatcher.store.nodes["node1"].workers = pkgconfigsetup.DefaultNumWorkers
	testDispatcher.store.nodes["node2"] = newNodeStore("node2", "10.0.0.2")
	testDispatcher.store.nodes["node2"].workers = pkgconfigsetup.DefaultNumWorkers

	node1Stats := map[string]types.CLCRunnerStats{
		"checkM0": {AverageExecutionTime: 2000, IsClusterCheck: true},
		"checkM1": {AverageExecutionTime: 2000, IsClusterCheck: true},
	}
	node2Stats := map[string]types.CLCRunnerStats{
		"checkL0": {AverageExecutionTime: 200, IsClusterCheck: true},
	}
	testDispatcher.store.nodes["node1"].clcRunnerStats = node1Stats
	testDispatcher.store.nodes["node2"].clcRunnerStats = node2Stats
	mockClient.testStats["10.0.0.1"] = node1Stats
	mockClient.testStats["10.0.0.2"] = node2Stats

	// checkM0 and checkM1 are two instances of one config.
	testDispatcher.store.idToDigest = map[checkid.ID]string{
		"checkM0": "digestMulti",
		"checkM1": "digestMulti",
		"checkL0": "digestLight",
	}
	testDispatcher.store.digestToConfig = map[string]integration.Config{
		"digestMulti": {Name: "multi"},
		"digestLight": {Name: "light"},
	}
	testDispatcher.store.digestToNode = map[string]string{
		"digestMulti": "node1",
		"digestLight": "node2",
	}

	// One entry per config in the distribution; multi's workers are summed.
	dist := testDispatcher.currentDistribution()
	require.Len(t, dist.Configs, 2)
	require.Contains(t, dist.Configs, "digestMulti")
	require.Contains(t, dist.Configs, "digestLight")
	assert.Equal(t, "node1", dist.Configs["digestMulti"].Runner)
	assert.InDelta(t, dist.Configs["digestMulti"].WorkersNeeded, dist.Configs["digestLight"].WorkersNeeded*20, 0.0001)

	// Force the rebalance so the worth-it check doesn't short-circuit it.
	checksMoved := testDispatcher.rebalanceUsingUtilization(true)
	requireNotLocked(t, testDispatcher.store)

	// At most one move; if there is one, both instances follow it.
	assert.LessOrEqual(t, len(checksMoved), 1)
	for _, mv := range checksMoved {
		assert.Equal(t, "digestMulti", mv.Digest)
		destStats := testDispatcher.store.nodes[mv.DestNodeName].clcRunnerStats
		assert.Contains(t, destStats, "checkM0")
		assert.Contains(t, destStats, "checkM1")
		srcStats := testDispatcher.store.nodes[mv.SourceNodeName].clcRunnerStats
		assert.NotContains(t, srcStats, "checkM0")
		assert.NotContains(t, srcStats, "checkM1")
	}
}

// Verifies that two configs sharing a check name but with different digests
// (e.g., two postgres configs monitoring different databases) get separate
// distribution entries and are moved independently.
func TestCurrentDistribution_SeparatesConfigsByDigest(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	testDispatcher := newDispatcher(fakeTagger)
	testDispatcher.store.active = true
	testDispatcher.store.nodes["node1"] = newNodeStore("node1", "10.0.0.1")
	testDispatcher.store.nodes["node1"].workers = pkgconfigsetup.DefaultNumWorkers

	testDispatcher.store.nodes["node1"].clcRunnerStats = map[string]types.CLCRunnerStats{
		"checkPgA": {AverageExecutionTime: 1000, IsClusterCheck: true},
		"checkPgB": {AverageExecutionTime: 1000, IsClusterCheck: true},
	}
	// Same check name, two different digests — two distinct configs.
	testDispatcher.store.idToDigest = map[checkid.ID]string{
		"checkPgA": "digestPgA",
		"checkPgB": "digestPgB",
	}
	testDispatcher.store.digestToConfig = map[string]integration.Config{
		"digestPgA": {Name: "postgres"},
		"digestPgB": {Name: "postgres"},
	}
	testDispatcher.store.digestToNode = map[string]string{
		"digestPgA": "node1",
		"digestPgB": "node1",
	}

	dist := testDispatcher.currentDistribution()
	require.Len(t, dist.Configs, 2)
	require.Contains(t, dist.Configs, "digestPgA")
	require.Contains(t, dist.Configs, "digestPgB")
	assert.Equal(t, "postgres", dist.Configs["digestPgA"].CheckName)
	assert.Equal(t, "postgres", dist.Configs["digestPgB"].CheckName)
}

// Verifies pickConfigToMove aggregates weight by digest so a multi-instance
// config can outrank a single-instance one with higher per-check weight.
func TestRebalanceUsingBusyness_GroupsInstancesByConfig(t *testing.T) {
	originalCheckExecutionTimeWeight := checkExecutionTimeWeight
	originalMetricSamplesWeight := checkMetricSamplesWeight
	checkExecutionTimeWeight = 0.8
	checkMetricSamplesWeight = 0.2
	defer func() {
		checkExecutionTimeWeight = originalCheckExecutionTimeWeight
		checkMetricSamplesWeight = originalMetricSamplesWeight
	}()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	testDispatcher := newDispatcher(fakeTagger)

	mockClient := &rebalanceTestClcRunnerClient{
		testStats: make(map[string]types.CLCRunnersStats),
	}
	testDispatcher.clcRunnersClient = mockClient

	testDispatcher.store.active = true
	testDispatcher.store.nodes["A"] = newNodeStore("A", "10.0.0.1")
	testDispatcher.store.nodes["B"] = newNodeStore("B", "10.0.0.2")

	// Per-checkID, checkH0 (250) looks heaviest; aggregated per-config,
	// digestMulti (3 × 125) outweighs digestHeavy.
	nodeAStats := map[string]types.CLCRunnerStats{
		"checkH0": {AverageExecutionTime: 250, MetricSamples: 0, IsClusterCheck: true},
		"checkM0": {AverageExecutionTime: 125, MetricSamples: 0, IsClusterCheck: true},
		"checkM1": {AverageExecutionTime: 125, MetricSamples: 0, IsClusterCheck: true},
		"checkM2": {AverageExecutionTime: 125, MetricSamples: 0, IsClusterCheck: true},
	}
	testDispatcher.store.nodes["A"].clcRunnerStats = nodeAStats
	mockClient.testStats["10.0.0.1"] = nodeAStats
	mockClient.testStats["10.0.0.2"] = types.CLCRunnersStats{}

	testDispatcher.store.idToDigest = map[checkid.ID]string{
		"checkH0": "digestHeavy",
		"checkM0": "digestMulti",
		"checkM1": "digestMulti",
		"checkM2": "digestMulti",
	}
	testDispatcher.store.digestToConfig = map[string]integration.Config{
		"digestHeavy": {Name: "heavy", Instances: []integration.Data{integration.Data("")}},
		"digestMulti": {Name: "multi", Instances: []integration.Data{integration.Data("a"), integration.Data("b"), integration.Data("c")}},
	}
	testDispatcher.store.digestToNode = map[string]string{
		"digestHeavy": "A",
		"digestMulti": "A",
	}

	digest, weight, err := testDispatcher.pickConfigToMove("A")
	require.NoError(t, err)
	assert.Equal(t, "digestMulti", digest)
	assert.Greater(t, weight, 200)
}

func TestRebalanceIsWorthIt(t *testing.T) {
	workersPerRunner := map[string]int{
		"runner1": 3,
		"runner2": 3,
		"runner3": 3,
	}

	// The proposed solution is worth it if it leaves less unused runners

	currentDistribution := newConfigsDistribution(workersPerRunner)
	currentDistribution.addConfig("check1", "check1", 1, "runner1")
	currentDistribution.addConfig("check2", "check2", 1, "runner1")

	proposedDistribution := newConfigsDistribution(workersPerRunner)
	proposedDistribution.addConfig("check1", "check1", 1, "runner1")
	proposedDistribution.addConfig("check2", "check2", 1, "runner2")

	assert.True(t, rebalanceIsWorthIt(currentDistribution, proposedDistribution, 10))

	// The proposed	solution is worth it if it has fewer runners with a high utilization
	currentDistribution = newConfigsDistribution(workersPerRunner)
	currentDistribution.addConfig("check1", "check1", 1, "runner1")
	currentDistribution.addConfig("check2", "check2", 1, "runner1")
	currentDistribution.addConfig("check3", "check3", 1, "runner1")

	proposedDistribution = newConfigsDistribution(workersPerRunner)
	proposedDistribution.addConfig("check1", "check1", 1, "runner1")
	proposedDistribution.addConfig("check2", "check2", 1, "runner2")
	proposedDistribution.addConfig("check3", "check3", 1, "runner3")

	assert.True(t, rebalanceIsWorthIt(currentDistribution, proposedDistribution, 10))
}
