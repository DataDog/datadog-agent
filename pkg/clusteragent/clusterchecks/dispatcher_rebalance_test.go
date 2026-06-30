// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
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

	configmock.New(t).SetInTest("cluster_checks.stickiness_enabled", false)
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

// Verifies two things: currentDistribution groups each config's instances under
// one entry (summing their workersNeeded), and when two heavy multi-instance
// configs are stacked on node1 alongside a lightweight config on node2, the
// utilization rebalancer moves one heavy config to node2, spreading the load.
func TestRebalanceUsingUtilization_GroupsAndSpreadsMultiInstanceConfigs(t *testing.T) {
	configmock.New(t).SetInTest("cluster_checks.stickiness_enabled", false)
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

	// node1: two heavy multi-instance configs. node2: one lightweight config.
	node1Stats := map[string]types.CLCRunnerStats{
		"checkA0": {AverageExecutionTime: 3750, IsClusterCheck: true},
		"checkA1": {AverageExecutionTime: 250, IsClusterCheck: true},
		"checkB0": {AverageExecutionTime: 3750, IsClusterCheck: true},
		"checkB1": {AverageExecutionTime: 250, IsClusterCheck: true},
	}
	node2Stats := map[string]types.CLCRunnerStats{
		"checkL0": {AverageExecutionTime: 500, IsClusterCheck: true},
	}
	testDispatcher.store.nodes["node1"].clcRunnerStats = node1Stats
	testDispatcher.store.nodes["node2"].clcRunnerStats = node2Stats
	mockClient.testStats["10.0.0.1"] = node1Stats
	mockClient.testStats["10.0.0.2"] = node2Stats

	// checkA0/checkA1 → digestMultiA; checkB0/checkB1 → digestMultiB; checkL0 → digestLight.
	testDispatcher.store.idToDigest = map[checkid.ID]string{
		"checkA0": "digestMultiA",
		"checkA1": "digestMultiA",
		"checkB0": "digestMultiB",
		"checkB1": "digestMultiB",
		"checkL0": "digestLight",
	}
	testDispatcher.store.digestToConfig = map[string]integration.Config{
		"digestMultiA": {Name: "multiA"},
		"digestMultiB": {Name: "multiB"},
		"digestLight":  {Name: "light"},
	}
	testDispatcher.store.digestToNode = map[string]string{
		"digestMultiA": "node1",
		"digestMultiB": "node1",
		"digestLight":  "node2",
	}

	// Verify currentDistribution groups each config's instances into one entry.
	dist := testDispatcher.currentDistribution()
	require.Len(t, dist.Configs, 3)
	require.Contains(t, dist.Configs, "digestMultiA")
	require.Contains(t, dist.Configs, "digestMultiB")
	require.Contains(t, dist.Configs, "digestLight")
	assert.Equal(t, "node1", dist.Configs["digestMultiA"].Runner)
	assert.Equal(t, "node1", dist.Configs["digestMultiB"].Runner)
	assert.Equal(t, "node2", dist.Configs["digestLight"].Runner)
	// This ensures that the tracking of workersNeeded is accurate relative to 1 instance configs.
	assert.InDelta(t, dist.Configs["digestMultiA"].WorkersNeeded, dist.Configs["digestLight"].WorkersNeeded*8, 0.001)
	assert.InDelta(t, dist.Configs["digestMultiB"].WorkersNeeded, dist.Configs["digestLight"].WorkersNeeded*8, 0.001)

	checksMoved := testDispatcher.rebalanceUsingUtilization(true)
	requireNotLocked(t, testDispatcher.store)
	require.NotEmpty(t, checksMoved)

	node1After := testDispatcher.store.nodes["node1"].clcRunnerStats
	node2After := testDispatcher.store.nodes["node2"].clcRunnerStats

	// Each multi config's instances must remain together — no splitting.
	_, a0OnNode1 := node1After["checkA0"]
	_, a1OnNode1 := node1After["checkA1"]
	assert.Equal(t, a0OnNode1, a1OnNode1, "digestMultiA instances must stay together")
	_, b0OnNode1 := node1After["checkB0"]
	_, b1OnNode1 := node1After["checkB1"]
	assert.Equal(t, b0OnNode1, b1OnNode1, "digestMultiB instances must stay together")

	// The two heavy configs must end up on different nodes.
	assert.NotEqual(t, a0OnNode1, b0OnNode1, "digestMultiA and digestMultiB should be on different nodes after rebalance")

	// Every heavy instance is on exactly one node.
	for _, checkID := range []string{"checkA0", "checkA1", "checkB0", "checkB1"} {
		_, onNode1 := node1After[checkID]
		_, onNode2 := node2After[checkID]
		assert.True(t, onNode1 || onNode2, "check %s should be on one of the nodes", checkID)
		assert.False(t, onNode1 && onNode2, "check %s should not be on both nodes", checkID)
	}

	// Phase 2: reduce the exec time of the heavy instance that is NOT co-located with
	// checkL0. This lightens one multi config, creating an imbalance that should
	// cause digestLight to move to the lighter side.
	_, checkL0OnNode1 := node1After["checkL0"]
	var heavyNodeName string
	if checkL0OnNode1 {
		heavyNodeName = "node2"
	} else {
		heavyNodeName = "node1"
	}

	heavyNodeStats := testDispatcher.store.nodes[heavyNodeName].clcRunnerStats
	var reducedCheckID string
	if _, ok := heavyNodeStats["checkA0"]; ok {
		reducedCheckID = "checkA0"
	} else {
		reducedCheckID = "checkB0"
	}
	s := heavyNodeStats[reducedCheckID]
	s.AverageExecutionTime = 250
	heavyNodeStats[reducedCheckID] = s

	checksMoved2 := testDispatcher.rebalanceUsingUtilization(true)
	requireNotLocked(t, testDispatcher.store)

	// The lighter multi config now has far less work than the other; digestLight should migrate to balance the load.
	require.Len(t, checksMoved2, 1)
	assert.Equal(t, "digestLight", checksMoved2[0].Digest)
}

// Verifies that checks whose AverageExecutionTime is 0 (e.g. long-running checks
// or checks that haven't completed a meaningful run yet) are pinned to their
// current runner and excluded from rebalancing, while a sibling check with real
// execution-time data remains eligible to move.
func TestRebalanceUsingUtilization_PinsChecksWithoutExecutionTime(t *testing.T) {
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

	// All three checks start on node1: two with AverageExecutionTime == 0
	// (pinned) and one with real data (eligible to move).
	node1Stats := types.CLCRunnersStats{
		"pinned1": {AverageExecutionTime: 0, IsClusterCheck: true},
		"pinned2": {AverageExecutionTime: 0, IsClusterCheck: true},
		"movable": {AverageExecutionTime: 2000, IsClusterCheck: true},
	}
	testDispatcher.store.nodes["node1"].clcRunnerStats = node1Stats
	mockClient.testStats["10.0.0.1"] = node1Stats
	mockClient.testStats["10.0.0.2"] = types.CLCRunnersStats{}

	testDispatcher.store.idToDigest = map[checkid.ID]string{
		"pinned1": "digestPinned1",
		"pinned2": "digestPinned2",
		"movable": "digestMovable",
	}
	testDispatcher.store.digestToConfig = map[string]integration.Config{
		"digestPinned1": {Name: "pinned1"},
		"digestPinned2": {Name: "pinned2"},
		"digestMovable": {Name: "movable"},
	}
	testDispatcher.store.digestToNode = map[string]string{
		"digestPinned1": "node1",
		"digestPinned2": "node1",
		"digestMovable": "node1",
	}

	// currentDistribution should mark the two zero-execution-time checks as
	// pinned and leave the movable one unpinned.
	dist := testDispatcher.currentDistribution()
	require.Contains(t, dist.Configs, "digestPinned1")
	require.Contains(t, dist.Configs, "digestPinned2")
	require.Contains(t, dist.Configs, "digestMovable")
	assert.True(t, dist.Configs["digestPinned1"].Pinned, "pinned1 should be pinned (AverageExecutionTime == 0)")
	assert.True(t, dist.Configs["digestPinned2"].Pinned, "pinned2 should be pinned (AverageExecutionTime == 0)")
	assert.False(t, dist.Configs["digestMovable"].Pinned, "movable should not be pinned")

	// Force the rebalance and verify pinned checks are never relocated.
	checksMoved := testDispatcher.rebalanceUsingUtilization(true)

	requireNotLocked(t, testDispatcher.store)

	for _, move := range checksMoved {
		assert.NotEqual(t, "digestPinned1", move.Digest, "pinned1 must not move")
		assert.NotEqual(t, "digestPinned2", move.Digest, "pinned2 must not move")
	}

	// Final placement: both pinned checks remain on node1 regardless of what
	// the rebalancer decided to do with the movable check.
	assert.Contains(t, testDispatcher.store.nodes["node1"].clcRunnerStats, "pinned1")
	assert.Contains(t, testDispatcher.store.nodes["node1"].clcRunnerStats, "pinned2")
	assert.NotContains(t, testDispatcher.store.nodes["node2"].clcRunnerStats, "pinned1")
	assert.NotContains(t, testDispatcher.store.nodes["node2"].clcRunnerStats, "pinned2")
}

// Regression: when a pinned config and a movable config share a runner, the
// pinned config's load must be accounted for before greedy placement so the
// movable one is rebalanced off the (already overloaded) runner. Without the
// two-loop split, sorting by WorkersNeeded places the heavier movable config
// first while both runners still look empty, which anchors it to its current
// (overloaded) runner.
func TestRebalanceUsingUtilization_PinnedLoadAccountedBeforeGreedyPlacement(t *testing.T) {
	configmock.New(t).SetInTest("cluster_checks.stickiness_enabled", false)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	testDispatcher := newDispatcher(fakeTagger)

	mockClient := &rebalanceTestClcRunnerClient{
		testStats: make(map[string]types.CLCRunnersStats),
	}
	testDispatcher.clcRunnersClient = mockClient
	testDispatcher.advancedDispatching.Store(true)

	// Pin the "pinned" check via the exclusion list so it ends up with
	// WorkersNeeded > 0 (the AverageExecutionTime == 0 path would give 0).
	testDispatcher.excludedChecksFromDispatching = map[string]struct{}{
		"pinned": {},
	}

	testDispatcher.store.active = true
	testDispatcher.store.nodes["node1"] = newNodeStore("node1", "10.0.0.1")
	testDispatcher.store.nodes["node1"].workers = pkgconfigsetup.DefaultNumWorkers
	testDispatcher.store.nodes["node2"] = newNodeStore("node2", "10.0.0.2")
	testDispatcher.store.nodes["node2"].workers = pkgconfigsetup.DefaultNumWorkers

	// node1: pinned (0.6 workers) + movable (0.7 workers). node2: empty.
	// With a default 15s interval, AvgExecTime in ms equals workersNeeded * 15000.
	node1Stats := types.CLCRunnersStats{
		"pinned":  {AverageExecutionTime: 9000, IsClusterCheck: true},  // 0.6 workers
		"movable": {AverageExecutionTime: 10500, IsClusterCheck: true}, // 0.7 workers
	}
	testDispatcher.store.nodes["node1"].clcRunnerStats = node1Stats
	mockClient.testStats["10.0.0.1"] = node1Stats
	mockClient.testStats["10.0.0.2"] = types.CLCRunnersStats{}

	testDispatcher.store.idToDigest = map[checkid.ID]string{
		"pinned":  "digestPinned",
		"movable": "digestMovable",
	}
	testDispatcher.store.digestToConfig = map[string]integration.Config{
		"digestPinned":  {Name: "pinned"},
		"digestMovable": {Name: "movable"},
	}
	testDispatcher.store.digestToNode = map[string]string{
		"digestPinned":  "node1",
		"digestMovable": "node1",
	}

	checksMoved := testDispatcher.rebalanceUsingUtilization(true)

	requireNotLocked(t, testDispatcher.store)

	// The pinned check must stay on node1; the movable one must move to node2
	// because node1 already carries 0.6 workers of pinned load.
	require.Len(t, checksMoved, 1)
	assert.Equal(t, "digestMovable", checksMoved[0].Digest)
	assert.Equal(t, "node1", checksMoved[0].SourceNodeName)
	assert.Equal(t, "node2", checksMoved[0].DestNodeName)

	assert.Contains(t, testDispatcher.store.nodes["node1"].clcRunnerStats, "pinned")
	assert.NotContains(t, testDispatcher.store.nodes["node1"].clcRunnerStats, "movable")
	assert.Contains(t, testDispatcher.store.nodes["node2"].clcRunnerStats, "movable")
	assert.NotContains(t, testDispatcher.store.nodes["node2"].clcRunnerStats, "pinned")
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

	currentDistribution := newConfigsDistribution(workersPerRunner, false, 4.0, 1.0, 0.05)
	currentDistribution.addConfig("check1", "check1", 1, "runner1", false)
	currentDistribution.addConfig("check2", "check2", 1, "runner1", false)

	proposedDistribution := newConfigsDistribution(workersPerRunner, false, 4.0, 1.0, 0.05)
	proposedDistribution.addConfig("check1", "check1", 1, "runner1", false)
	proposedDistribution.addConfig("check2", "check2", 1, "runner2", false)

	assert.True(t, rebalanceIsWorthIt(currentDistribution, proposedDistribution, 10))

	// The proposed	solution is worth it if it has fewer runners with a high utilization
	currentDistribution = newConfigsDistribution(workersPerRunner, false, 4.0, 1.0, 0.05)
	currentDistribution.addConfig("check1", "check1", 1, "runner1", false)
	currentDistribution.addConfig("check2", "check2", 1, "runner1", false)
	currentDistribution.addConfig("check3", "check3", 1, "runner1", false)

	proposedDistribution = newConfigsDistribution(workersPerRunner, false, 4.0, 1.0, 0.05)
	proposedDistribution.addConfig("check1", "check1", 1, "runner1", false)
	proposedDistribution.addConfig("check2", "check2", 1, "runner2", false)
	proposedDistribution.addConfig("check3", "check3", 1, "runner3", false)

	assert.True(t, rebalanceIsWorthIt(currentDistribution, proposedDistribution, 10))
}

// TestRebalanceUsingBusyness_BreaksOnMoveConfigFailure verifies that the inner
// rebalancing loop exits (break) when moveConfig fails, rather than retrying
// the same failing move indefinitely (continue). The latter would spike the
// rebalancing_decisions telemetry counter and potentially hang the process.
func TestRebalanceUsingBusyness_BreaksOnMoveConfigFailure(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	dispatcher := newDispatcher(fakeTagger)

	mockClient := &rebalanceTestClcRunnerClient{
		testStats: make(map[string]types.CLCRunnersStats),
	}
	dispatcher.clcRunnersClient = mockClient
	dispatcher.store.active = true

	// Deliberately leave digestToConfig empty for "digestA1".
	// This causes moveConfig to return "no config registered" without moving
	// anything, while pickConfigToMove still selects "digestA1" (it only needs
	// idToDigest, not digestToConfig).
	dispatcher.store.idToDigest[checkid.ID("checkA0")] = "digestA0"
	dispatcher.store.idToDigest[checkid.ID("checkA1")] = "digestA1"
	dispatcher.store.digestToConfig["digestA0"] = integration.Config{Name: "checkA0"}
	// digestToConfig["digestA1"] intentionally absent to force moveConfig failure
	dispatcher.store.digestToNode["digestA0"] = "A"
	dispatcher.store.digestToNode["digestA1"] = "A"

	// Node A: two cluster checks; checkA1 is heavier and will be picked first.
	// With default weights (checkMetricSamplesWeight=1), busyness(A) = 100.
	nodeAStats := types.CLCRunnersStats{
		"checkA0": {MetricSamples: 30},
		"checkA1": {MetricSamples: 70},
	}
	dispatcher.store.nodes["A"] = newNodeStore("A", "10.0.0.1")
	mockClient.testStats["10.0.0.1"] = nodeAStats

	// Node B: empty, busyness 0. avg = 50, diffMap[A]=+50, diffMap[B]=-50.
	dispatcher.store.nodes["B"] = newNodeStore("B", "10.0.0.2")
	mockClient.testStats["10.0.0.2"] = types.CLCRunnersStats{}

	// The algorithm will:
	//   1. pickConfigToMove("A") → "digestA1"
	//   2. moveConfig("A", "B", "digestA1") → error (no digestToConfig entry)
	//   3. With the fix (break): loop exits; node A is unchanged
	//      Without the fix (continue): loop spins indefinitely
	done := make(chan struct{})
	go func() {
		dispatcher.rebalanceUsingBusyness()
		close(done)
	}()

	select {
	case <-done:
		// Algorithm terminated as expected.
	case <-time.After(5 * time.Second):
		t.Fatal("rebalanceUsingBusyness did not terminate: possible infinite loop on moveConfig failure (continue vs break bug)")
	}

	// No move should have occurred: digestA1 still belongs to A and A's stats are intact.
	assert.Equal(t, "A", dispatcher.store.digestToNode["digestA1"])
	assert.Contains(t, dispatcher.store.nodes["A"].clcRunnerStats, "checkA1")
	assert.Empty(t, dispatcher.store.nodes["B"].clcRunnerStats)
}
