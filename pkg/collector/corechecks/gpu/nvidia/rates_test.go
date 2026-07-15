// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package nvidia

import (
	"testing"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/require"
)

func TestBuildRateKeySortsTags(t *testing.T) {
	metricA := &Metric{
		Name: "metric.name",
		Tags: []string{"b:2", "a:1"},
	}
	metricB := &Metric{
		Name: "metric.name",
		Tags: []string{"a:1", "b:2"},
	}

	require.Equal(t, buildRateKey(metricA, "gpu-1"), buildRateKey(metricB, "gpu-1"))
	require.Equal(t, []string{"b:2", "a:1"}, metricA.Tags, "input tags should not be mutated")
}

func TestBuildRateKeySortsAssociatedWorkloads(t *testing.T) {
	metricA := &Metric{
		Name: "metric.name",
		AssociatedWorkloads: []workloadmeta.EntityID{
			{Kind: workloadmeta.KindContainer, ID: "container-1"},
			{Kind: workloadmeta.KindProcess, ID: "123"},
		},
	}
	metricB := &Metric{
		Name: "metric.name",
		AssociatedWorkloads: []workloadmeta.EntityID{
			{Kind: workloadmeta.KindProcess, ID: "123"},
			{Kind: workloadmeta.KindContainer, ID: "container-1"},
		},
	}

	require.Equal(t, buildRateKey(metricA, "gpu-1"), buildRateKey(metricB, "gpu-1"))
	require.Equal(t, []workloadmeta.EntityID{
		{Kind: workloadmeta.KindContainer, ID: "container-1"},
		{Kind: workloadmeta.KindProcess, ID: "123"},
	}, metricA.AssociatedWorkloads, "input workloads should not be mutated")
}

func TestRateCalculatorNoRateCalculationLeavesMetricUntouched(t *testing.T) {
	calculator := NewRateCalculator()
	now := time.Unix(100, 0)
	metric := &Metric{
		Name:                "test.metric",
		Value:               42,
		Tags:                []string{"gpu_uuid:abc"},
		RateCalculationMode: NoRateCalculation,
	}

	result := calculator.ProcessMetrics([]*Metric{metric}, now, "gpu-1")

	require.Equal(t, 42.0, metric.Value)
	require.Len(t, result, 1)
	require.Same(t, metric, result[0])
	require.Empty(t, calculator.previousValues)
}

func TestRateCalculatorAbsoluteDelta(t *testing.T) {
	calculator := NewRateCalculator()
	key := []string{"gpu_uuid:abc"}
	t1 := time.Unix(100, 0)
	t2 := time.Unix(105, 0)

	first := &Metric{
		Name:                "errors.total",
		Value:               10,
		Tags:                key,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}
	second := &Metric{
		Name:                "errors.total",
		Value:               16,
		Tags:                []string{"gpu_uuid:abc"},
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}

	firstResult := calculator.ProcessMetrics([]*Metric{first}, t1, "gpu-1")
	require.Empty(t, firstResult)

	secondResult := calculator.ProcessMetrics([]*Metric{second}, t2, "gpu-1")
	require.Len(t, secondResult, 1)
	require.Same(t, second, secondResult[0])
	require.Equal(t, 6.0, second.Value)
}

func TestRateCalculatorPerSecond(t *testing.T) {
	calculator := NewRateCalculator()
	t1 := time.Unix(100, 0)
	t2 := time.Unix(104, 0)

	first := &Metric{
		Name:                "bytes.transferred",
		Value:               20,
		Tags:                []string{"gpu_uuid:abc"},
		RateCalculationMode: PerSecondRateCalculation,
	}
	second := &Metric{
		Name:                "bytes.transferred",
		Value:               36,
		Tags:                []string{"gpu_uuid:abc"},
		RateCalculationMode: PerSecondRateCalculation,
	}

	firstResult := calculator.ProcessMetrics([]*Metric{first}, t1, "gpu-1")
	require.Empty(t, firstResult)

	secondResult := calculator.ProcessMetrics([]*Metric{second}, t2, "gpu-1")
	require.Len(t, secondResult, 1)
	require.Same(t, second, secondResult[0])
	require.Equal(t, 4.0, second.Value)
}

func TestRateCalculatorPerSecondNonPositiveTimeDiff(t *testing.T) {
	calculator := NewRateCalculator()
	t1 := time.Unix(100, 0)
	t2 := time.Unix(100, 0)
	t3 := time.Unix(99, 0)

	first := &Metric{
		Name:                "bytes.transferred",
		Value:               20,
		Tags:                []string{"gpu_uuid:abc"},
		RateCalculationMode: PerSecondRateCalculation,
	}
	sameTimestamp := &Metric{
		Name:                "bytes.transferred",
		Value:               30,
		Tags:                []string{"gpu_uuid:abc"},
		RateCalculationMode: PerSecondRateCalculation,
	}
	earlierTimestamp := &Metric{
		Name:                "bytes.transferred",
		Value:               40,
		Tags:                []string{"gpu_uuid:abc"},
		RateCalculationMode: PerSecondRateCalculation,
	}

	firstResult := calculator.ProcessMetrics([]*Metric{first}, t1, "gpu-1")
	sameTimestampResult := calculator.ProcessMetrics([]*Metric{sameTimestamp}, t2, "gpu-1")
	earlierTimestampResult := calculator.ProcessMetrics([]*Metric{earlierTimestamp}, t3, "gpu-1")

	require.Empty(t, firstResult)
	require.Len(t, sameTimestampResult, 1)
	require.Len(t, earlierTimestampResult, 1)
	require.Equal(t, 0.0, sameTimestamp.Value)
	require.Equal(t, 0.0, earlierTimestamp.Value)
}

func TestRateCalculatorNegativeDeltaIsClampedToZero(t *testing.T) {
	calculator := NewRateCalculator()
	t1 := time.Unix(100, 0)
	t2 := time.Unix(104, 0)

	firstAbsolute := &Metric{
		Name:                "errors.total",
		Value:               20,
		Tags:                []string{"gpu_uuid:abc"},
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}
	secondAbsolute := &Metric{
		Name:                "errors.total",
		Value:               15,
		Tags:                []string{"gpu_uuid:abc"},
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}

	firstPerSecond := &Metric{
		Name:                "bytes.transferred",
		Value:               40,
		Tags:                []string{"gpu_uuid:abc"},
		RateCalculationMode: PerSecondRateCalculation,
	}
	secondPerSecond := &Metric{
		Name:                "bytes.transferred",
		Value:               30,
		Tags:                []string{"gpu_uuid:abc"},
		RateCalculationMode: PerSecondRateCalculation,
	}

	firstResult := calculator.ProcessMetrics([]*Metric{firstAbsolute, firstPerSecond}, t1, "gpu-1")
	secondResult := calculator.ProcessMetrics([]*Metric{secondAbsolute, secondPerSecond}, t2, "gpu-1")

	require.Empty(t, firstResult)
	require.Len(t, secondResult, 2)
	require.Equal(t, 0.0, secondAbsolute.Value)
	require.Equal(t, 0.0, secondPerSecond.Value)
}

func TestRateCalculatorDifferentRateKeysDoNotConflict(t *testing.T) {
	calculator := NewRateCalculator()
	t1 := time.Unix(100, 0)
	t2 := time.Unix(102, 0)

	firstBatch := []*Metric{
		{
			Name:                "metric.one",
			Value:               10,
			Tags:                []string{"gpu_uuid:abc"},
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
		{
			Name:                "metric.one",
			Value:               50,
			Tags:                []string{"gpu_uuid:def"},
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
		{
			Name:                "metric.two",
			Value:               100,
			Tags:                []string{"gpu_uuid:abc"},
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
	}

	secondBatch := []*Metric{
		{
			Name:                "metric.one",
			Value:               15,
			Tags:                []string{"gpu_uuid:abc"},
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
		{
			Name:                "metric.one",
			Value:               58,
			Tags:                []string{"gpu_uuid:def"},
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
		{
			Name:                "metric.two",
			Value:               130,
			Tags:                []string{"gpu_uuid:abc"},
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
	}

	firstResult := calculator.ProcessMetrics(firstBatch, t1, "gpu-1")
	secondResult := calculator.ProcessMetrics(secondBatch, t2, "gpu-1")

	require.Empty(t, firstResult)
	require.Len(t, secondResult, 3)
	require.Equal(t, 5.0, secondBatch[0].Value)
	require.Equal(t, 8.0, secondBatch[1].Value)
	require.Equal(t, 30.0, secondBatch[2].Value)
}

func TestRateCalculatorDifferentGPUUUIDsUseIndependentRateKeys(t *testing.T) {
	calculator := NewRateCalculator()
	t1 := time.Unix(100, 0)
	t2 := time.Unix(104, 0)

	gpu1First := &Metric{
		Name:                "bytes.transferred",
		Value:               10,
		Tags:                []string{"process:1234"},
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}
	gpu2First := &Metric{
		Name:                "bytes.transferred",
		Value:               100,
		Tags:                []string{"process:1234"},
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}

	gpu1FirstResult := calculator.ProcessMetrics([]*Metric{gpu1First}, t1, "gpu-1")
	gpu2FirstResult := calculator.ProcessMetrics([]*Metric{gpu2First}, t1, "gpu-2")

	gpu1Second := &Metric{
		Name:                "bytes.transferred",
		Value:               15,
		Tags:                []string{"process:1234"},
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}
	gpu2Second := &Metric{
		Name:                "bytes.transferred",
		Value:               130,
		Tags:                []string{"process:1234"},
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}

	gpu1SecondResult := calculator.ProcessMetrics([]*Metric{gpu1Second}, t2, "gpu-1")
	gpu2SecondResult := calculator.ProcessMetrics([]*Metric{gpu2Second}, t2, "gpu-2")

	require.Empty(t, gpu1FirstResult)
	require.Empty(t, gpu2FirstResult)
	require.Len(t, gpu1SecondResult, 1)
	require.Len(t, gpu2SecondResult, 1)
	require.Equal(t, 5.0, gpu1Second.Value)
	require.Equal(t, 30.0, gpu2Second.Value)
	require.Len(t, calculator.previousValues, 2)
	require.NotEqual(t, buildRateKey(gpu1Second, "gpu-1"), buildRateKey(gpu2Second, "gpu-2"))
}

func TestRateCalculatorDifferentAssociatedWorkloadsUseIndependentRateKeys(t *testing.T) {
	calculator := NewRateCalculator()
	t1 := time.Unix(100, 0)
	t2 := time.Unix(104, 0)

	process123First := &Metric{
		Name:                "process.core.usage",
		Value:               10,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
		AssociatedWorkloads: []workloadmeta.EntityID{{Kind: workloadmeta.KindProcess, ID: "123"}},
	}
	process456First := &Metric{
		Name:                "process.core.usage",
		Value:               100,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
		AssociatedWorkloads: []workloadmeta.EntityID{{Kind: workloadmeta.KindProcess, ID: "456"}},
	}

	firstResult := calculator.ProcessMetrics([]*Metric{process123First, process456First}, t1, "gpu-1")
	require.Empty(t, firstResult)

	process123Second := &Metric{
		Name:                "process.core.usage",
		Value:               15,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
		AssociatedWorkloads: []workloadmeta.EntityID{{Kind: workloadmeta.KindProcess, ID: "123"}},
	}
	process456Second := &Metric{
		Name:                "process.core.usage",
		Value:               130,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
		AssociatedWorkloads: []workloadmeta.EntityID{{Kind: workloadmeta.KindProcess, ID: "456"}},
	}

	secondResult := calculator.ProcessMetrics([]*Metric{process123Second, process456Second}, t2, "gpu-1")

	require.Len(t, secondResult, 2)
	require.Equal(t, 5.0, process123Second.Value)
	require.Equal(t, 30.0, process456Second.Value)
	require.Len(t, calculator.previousValues, 2)
	require.NotEqual(t, buildRateKey(process123Second, "gpu-1"), buildRateKey(process456Second, "gpu-1"))
}
