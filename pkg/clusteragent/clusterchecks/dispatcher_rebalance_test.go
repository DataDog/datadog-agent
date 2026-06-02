// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"sort"
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

			// Prepare store. Each cluster-check in the test data is
			// registered as a real one-instance config via addConfig (so
			// idToDigest / digestToConfig / digestToNode all get populated
			// the way production runs would populate them). The mock
			// runner client returns stats keyed by the real BuildID-derived
			// checkIDs. Non-cluster checks bypass addConfig and stay keyed
			// by their synthetic IDs in the mock — they contribute to node
			// busyness but never become rebalance candidates.
			dispatcher.store.active = true
			for node, store := range tc.in {
				nodeIP := fmt.Sprintf("10.0.0.%d", len(mockClient.testStats)+1)
				dispatcher.store.nodes[node] = newNodeStore(node, nodeIP)
				wireStats := types.CLCRunnersStats{}
				for syntheticID, stats := range store.clcRunnerStats {
					if !stats.IsClusterCheck {
						wireStats[syntheticID] = stats
						continue
					}
					config := makeRebalanceTestConfig(syntheticID)
					dispatcher.addConfig(config, node)
					realID := checkid.BuildID(config.Name, config.FastDigest(), config.Instances[0], config.InitConfig)
					wireStats[string(realID)] = stats
				}
				mockClient.testStats[nodeIP] = wireStats
				// Seed the node's local cache too so the assertion below
				// observes the pre-rebalance state if the algorithm doesn't
				// move anything.
				dispatcher.store.nodes[node].clcRunnerStats = wireStats
			}

			// Use busyness-based rebalancing directly for these legacy tests
			// (preserves test coverage for busyness algorithm while allowing utilization as default)
			dispatcher.rebalanceUsingBusyness()

			// Compare placement by check name — the test data uses synthetic
			// names ("checkA0", "checkB1") which map 1:1 to config names,
			// and IDToCheckName recovers them from the real BuildID-derived
			// IDs in clcRunnerStats.
			for node, store := range tc.out {
				expected := checkNamesIn(store.clcRunnerStats)
				actual := checkNamesIn(dispatcher.store.nodes[node].clcRunnerStats)
				assert.Equal(t, expected, actual, "node %s placement", node)
			}

			requireNotLocked(t, dispatcher.store)
		})
	}
}

// makeRebalanceTestConfig returns a one-instance config whose Name is the
// synthetic identifier used in TestRebalance tables. addConfig registers it
// and stores a digest-keyed entry; IDToCheckName(realID) recovers the name
// later for placement assertions.
func makeRebalanceTestConfig(name string) integration.Config {
	return integration.Config{
		Name:       name,
		InitConfig: integration.Data("{}"),
		Instances:  []integration.Data{integration.Data("{}")},
	}
}

// makeMultiInstanceTestConfig returns a config with `instances` distinct
// instances, each with unique Data so BuildID produces distinct checkIDs.
func makeMultiInstanceTestConfig(name string, instances int) integration.Config {
	cfg := integration.Config{
		Name:       name,
		InitConfig: integration.Data("{}"),
	}
	for i := 0; i < instances; i++ {
		cfg.Instances = append(cfg.Instances, integration.Data(fmt.Sprintf(`{"i":%d}`, i)))
	}
	return cfg
}

// instanceCheckIDs returns the per-instance checkIDs the dispatcher would
// compute for a config — same path as addConfig uses.
func instanceCheckIDs(cfg integration.Config) []checkid.ID {
	ids := make([]checkid.ID, 0, len(cfg.Instances))
	fastDigest := cfg.FastDigest()
	for _, inst := range cfg.Instances {
		ids = append(ids, checkid.BuildID(cfg.Name, fastDigest, inst, cfg.InitConfig))
	}
	return ids
}

// checkNamesIn returns the sorted set of check names present in a stats
// map. Non-cluster checks (whose checkIDs aren't real BuildID outputs) are
// excluded.
func checkNamesIn(stats types.CLCRunnersStats) []string {
	names := []string{}
	for id, s := range stats {
		if !s.IsClusterCheck {
			continue
		}
		names = append(names, checkid.IDToCheckName(checkid.ID(id)))
	}
	sort.Strings(names)
	return names
}

// TestRebalanceUsingBusyness_MultiInstanceConfigIsAtomicAndIdempotent
// covers the two key properties of the per-config rebalance fix:
//   - a config with multiple instances is treated as a single movable unit
//     (one move reported, all instances migrated together);
//   - once placement is balanced, a second rebalance is a no-op.
func TestRebalanceUsingBusyness_MultiInstanceConfigIsAtomicAndIdempotent(t *testing.T) {
	// Hardcoded weights to keep the math independent of defaults.
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
	mockClient := &rebalanceTestClcRunnerClient{testStats: map[string]types.CLCRunnersStats{}}
	dispatcher.clcRunnersClient = mockClient
	dispatcher.store.active = true

	nodeAIP := "10.0.0.1"
	nodeBIP := "10.0.0.2"
	dispatcher.store.nodes["A"] = newNodeStore("A", nodeAIP)
	dispatcher.store.nodes["B"] = newNodeStore("B", nodeBIP)

	// Node A starts with everything:
	//   - a 3-instance "postgres" config; per-instance busyness = 80
	//     → aggregate config busyness = 240
	//   - a single-instance "heavy" config; busyness = 100
	multiConfig := makeMultiInstanceTestConfig("postgres", 3)
	heavyConfig := makeRebalanceTestConfig("heavy")
	dispatcher.addConfig(multiConfig, "A")
	dispatcher.addConfig(heavyConfig, "A")

	multiIDs := instanceCheckIDs(multiConfig)
	heavyID := instanceCheckIDs(heavyConfig)[0]

	initial := types.CLCRunnersStats{}
	for _, id := range multiIDs {
		initial[string(id)] = types.CLCRunnerStats{AverageExecutionTime: 80, IsClusterCheck: true}
	}
	initial[string(heavyID)] = types.CLCRunnerStats{AverageExecutionTime: 100, IsClusterCheck: true}
	mockClient.testStats[nodeAIP] = initial
	mockClient.testStats[nodeBIP] = types.CLCRunnersStats{}

	// First pass: the multi-instance config should move from A to B as a
	// single atomic unit.
	moves := dispatcher.rebalanceUsingBusyness()
	require.Len(t, moves, 1, "expected exactly one move; multi-instance config should move as a unit")

	bStats := dispatcher.store.nodes["B"].clcRunnerStats
	for _, id := range multiIDs {
		assert.Contains(t, bStats, string(id), "instance %s should have followed its config to B", id)
	}
	aStats := dispatcher.store.nodes["A"].clcRunnerStats
	assert.Contains(t, aStats, string(heavyID), "heavy config should remain on A")
	for _, id := range multiIDs {
		assert.NotContains(t, aStats, string(id), "instance %s should no longer be on A", id)
	}

	// Sync the mock to the dispatcher's post-move state so the next
	// updateRunnersStats reflects reality rather than reseting back to the
	// initial layout (the mock is otherwise stateless).
	for _, node := range dispatcher.store.nodes {
		snapshot := types.CLCRunnersStats{}
		for k, v := range node.clcRunnerStats {
			snapshot[k] = v
		}
		mockClient.testStats[node.clientIP] = snapshot
	}

	// Second pass: already balanced, so nothing should move.
	moves = dispatcher.rebalanceUsingBusyness()
	assert.Empty(t, moves, "second rebalance should be a no-op once placement is balanced")
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

			// move check
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
	//   other tests specific for the checksDistribution struct that test more
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
	currentDistribution.addCheck("check1", "check1", 1, "runner1")
	currentDistribution.addCheck("check2", "check2", 1, "runner1")

	proposedDistribution := newChecksDistribution(workersPerRunner)
	proposedDistribution.addCheck("check1", "check1", 1, "runner1")
	proposedDistribution.addCheck("check2", "check2", 1, "runner2")

	assert.True(t, rebalanceIsWorthIt(currentDistribution, proposedDistribution, 10))

	// The proposed	solution is worth it if it has fewer runners with a high utilization
	currentDistribution = newChecksDistribution(workersPerRunner)
	currentDistribution.addCheck("check1", "check1", 1, "runner1")
	currentDistribution.addCheck("check2", "check2", 1, "runner1")
	currentDistribution.addCheck("check3", "check3", 1, "runner1")

	proposedDistribution = newChecksDistribution(workersPerRunner)
	proposedDistribution.addCheck("check1", "check1", 1, "runner1")
	proposedDistribution.addCheck("check2", "check2", 1, "runner2")
	proposedDistribution.addCheck("check3", "check3", 1, "runner3")

	assert.True(t, rebalanceIsWorthIt(currentDistribution, proposedDistribution, 10))
}
