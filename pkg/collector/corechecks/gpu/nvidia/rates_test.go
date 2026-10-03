// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestBuildRateKeySortsTags(t *testing.T) {
	metricA := &Metric{
		baseSample: baseSample{tags: []string{"b:2", "a:1"}},
		Name:       "metric.name",
	}
	metricB := &Metric{
		baseSample: baseSample{tags: []string{"a:1", "b:2"}},
		Name:       "metric.name",
	}

	require.Equal(t, buildRateKey(metricA, "gpu-1"), buildRateKey(metricB, "gpu-1"))
	require.Equal(t, []string{"b:2", "a:1"}, metricA.Tags(), "input tags should not be mutated")
}

func TestBuildRateKeySortsAssociatedWorkloads(t *testing.T) {
	metricA := &Metric{
		baseSample: baseSample{associatedWorkloads: []workloadmeta.EntityID{
			{Kind: workloadmeta.KindContainer, ID: "container-1"},
			{Kind: workloadmeta.KindProcess, ID: "123"},
		}},
		Name: "metric.name",
	}
	metricB := &Metric{
		baseSample: baseSample{associatedWorkloads: []workloadmeta.EntityID{
			{Kind: workloadmeta.KindProcess, ID: "123"},
			{Kind: workloadmeta.KindContainer, ID: "container-1"},
		}},
		Name: "metric.name",
	}

	require.Equal(t, buildRateKey(metricA, "gpu-1"), buildRateKey(metricB, "gpu-1"))
	require.Equal(t, []workloadmeta.EntityID{
		{Kind: workloadmeta.KindContainer, ID: "container-1"},
		{Kind: workloadmeta.KindProcess, ID: "123"},
	}, metricA.AssociatedWorkloads(), "input workloads should not be mutated")
}

func TestRateCalculatorNoRateCalculationLeavesMetricUntouched(t *testing.T) {
	calculator := NewRateCalculator()
	now := time.Unix(100, 0)
	metric := &Metric{
		baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
		Name:                "test.metric",
		Value:               42,
		RateCalculationMode: NoRateCalculation,
	}

	result := requireMetrics(t, calculator.ProcessSamples([]Sample{metric}, now, "gpu-1"))

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
		baseSample:          baseSample{tags: key},
		Name:                "errors.total",
		Value:               10,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}
	second := &Metric{
		baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
		Name:                "errors.total",
		Value:               16,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}

	firstResult := requireMetrics(t, calculator.ProcessSamples([]Sample{first}, t1, "gpu-1"))
	require.Empty(t, firstResult)

	secondResult := requireMetrics(t, calculator.ProcessSamples([]Sample{second}, t2, "gpu-1"))
	require.Len(t, secondResult, 1)
	require.Same(t, second, secondResult[0])
	require.Equal(t, 6.0, second.Value)
}

func TestRateCalculatorPerSecond(t *testing.T) {
	calculator := NewRateCalculator()
	t1 := time.Unix(100, 0)
	t2 := time.Unix(104, 0)

	first := &Metric{
		baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
		Name:                "bytes.transferred",
		Value:               20,
		RateCalculationMode: PerSecondRateCalculation,
	}
	second := &Metric{
		baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
		Name:                "bytes.transferred",
		Value:               36,
		RateCalculationMode: PerSecondRateCalculation,
	}

	firstResult := requireMetrics(t, calculator.ProcessSamples([]Sample{first}, t1, "gpu-1"))
	require.Empty(t, firstResult)

	secondResult := requireMetrics(t, calculator.ProcessSamples([]Sample{second}, t2, "gpu-1"))
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
		baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
		Name:                "bytes.transferred",
		Value:               20,
		RateCalculationMode: PerSecondRateCalculation,
	}
	sameTimestamp := &Metric{
		baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
		Name:                "bytes.transferred",
		Value:               30,
		RateCalculationMode: PerSecondRateCalculation,
	}
	earlierTimestamp := &Metric{
		baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
		Name:                "bytes.transferred",
		Value:               40,
		RateCalculationMode: PerSecondRateCalculation,
	}

	firstResult := requireMetrics(t, calculator.ProcessSamples([]Sample{first}, t1, "gpu-1"))
	sameTimestampResult := requireMetrics(t, calculator.ProcessSamples([]Sample{sameTimestamp}, t2, "gpu-1"))
	earlierTimestampResult := requireMetrics(t, calculator.ProcessSamples([]Sample{earlierTimestamp}, t3, "gpu-1"))

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
		baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
		Name:                "errors.total",
		Value:               20,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}
	secondAbsolute := &Metric{
		baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
		Name:                "errors.total",
		Value:               15,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}

	firstPerSecond := &Metric{
		baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
		Name:                "bytes.transferred",
		Value:               40,
		RateCalculationMode: PerSecondRateCalculation,
	}
	secondPerSecond := &Metric{
		baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
		Name:                "bytes.transferred",
		Value:               30,
		RateCalculationMode: PerSecondRateCalculation,
	}

	firstResult := calculator.ProcessSamples([]Sample{firstAbsolute, firstPerSecond}, t1, "gpu-1")
	secondResult := calculator.ProcessSamples([]Sample{secondAbsolute, secondPerSecond}, t2, "gpu-1")

	require.Empty(t, firstResult)
	require.Len(t, secondResult, 2)
	require.Equal(t, 0.0, secondAbsolute.Value)
	require.Equal(t, 0.0, secondPerSecond.Value)
}

func TestRateCalculatorDifferentRateKeysDoNotConflict(t *testing.T) {
	calculator := NewRateCalculator()
	t1 := time.Unix(100, 0)
	t2 := time.Unix(102, 0)

	firstBatch := []Sample{
		&Metric{
			baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
			Name:                "metric.one",
			Value:               10,
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
		&Metric{
			baseSample:          baseSample{tags: []string{"gpu_uuid:def"}},
			Name:                "metric.one",
			Value:               50,
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
		&Metric{
			baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
			Name:                "metric.two",
			Value:               100,
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
	}

	secondBatch := []Sample{
		&Metric{
			baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
			Name:                "metric.one",
			Value:               15,
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
		&Metric{
			baseSample:          baseSample{tags: []string{"gpu_uuid:def"}},
			Name:                "metric.one",
			Value:               58,
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
		&Metric{
			baseSample:          baseSample{tags: []string{"gpu_uuid:abc"}},
			Name:                "metric.two",
			Value:               130,
			RateCalculationMode: AbsoluteDeltaRateCalculation,
		},
	}

	firstResult := calculator.ProcessSamples(firstBatch, t1, "gpu-1")
	secondResult := calculator.ProcessSamples(secondBatch, t2, "gpu-1")

	require.Empty(t, firstResult)
	require.Len(t, secondResult, 3)
	secondMetrics := requireMetrics(t, secondBatch)
	require.Equal(t, 5.0, secondMetrics[0].Value)
	require.Equal(t, 8.0, secondMetrics[1].Value)
	require.Equal(t, 30.0, secondMetrics[2].Value)
}

func TestRateCalculatorDifferentGPUUUIDsUseIndependentRateKeys(t *testing.T) {
	calculator := NewRateCalculator()
	t1 := time.Unix(100, 0)
	t2 := time.Unix(104, 0)

	gpu1First := &Metric{
		baseSample:          baseSample{tags: []string{"process:1234"}},
		Name:                "bytes.transferred",
		Value:               10,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}
	gpu2First := &Metric{
		baseSample:          baseSample{tags: []string{"process:1234"}},
		Name:                "bytes.transferred",
		Value:               100,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}

	gpu1FirstResult := calculator.ProcessSamples([]Sample{gpu1First}, t1, "gpu-1")
	gpu2FirstResult := calculator.ProcessSamples([]Sample{gpu2First}, t1, "gpu-2")

	gpu1Second := &Metric{
		baseSample:          baseSample{tags: []string{"process:1234"}},
		Name:                "bytes.transferred",
		Value:               15,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}
	gpu2Second := &Metric{
		baseSample:          baseSample{tags: []string{"process:1234"}},
		Name:                "bytes.transferred",
		Value:               130,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}

	gpu1SecondResult := calculator.ProcessSamples([]Sample{gpu1Second}, t2, "gpu-1")
	gpu2SecondResult := calculator.ProcessSamples([]Sample{gpu2Second}, t2, "gpu-2")

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
		baseSample:          baseSample{associatedWorkloads: []workloadmeta.EntityID{{Kind: workloadmeta.KindProcess, ID: "123"}}},
		Name:                "process.core.usage",
		Value:               10,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}
	process456First := &Metric{
		baseSample:          baseSample{associatedWorkloads: []workloadmeta.EntityID{{Kind: workloadmeta.KindProcess, ID: "456"}}},
		Name:                "process.core.usage",
		Value:               100,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}

	firstResult := calculator.ProcessSamples([]Sample{process123First, process456First}, t1, "gpu-1")
	require.Empty(t, firstResult)

	process123Second := &Metric{
		baseSample:          baseSample{associatedWorkloads: []workloadmeta.EntityID{{Kind: workloadmeta.KindProcess, ID: "123"}}},
		Name:                "process.core.usage",
		Value:               15,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}
	process456Second := &Metric{
		baseSample:          baseSample{associatedWorkloads: []workloadmeta.EntityID{{Kind: workloadmeta.KindProcess, ID: "456"}}},
		Name:                "process.core.usage",
		Value:               130,
		RateCalculationMode: AbsoluteDeltaRateCalculation,
	}

	secondResult := calculator.ProcessSamples([]Sample{process123Second, process456Second}, t2, "gpu-1")

	require.Len(t, secondResult, 2)
	require.Equal(t, 5.0, process123Second.Value)
	require.Equal(t, 30.0, process456Second.Value)
	require.Len(t, calculator.previousValues, 2)
	require.NotEqual(t, buildRateKey(process123Second, "gpu-1"), buildRateKey(process456Second, "gpu-1"))
}
