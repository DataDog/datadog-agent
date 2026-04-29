// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package nvidia

import (
	"testing"
	"time"

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

	require.Equal(t, buildRateKey(metricA), buildRateKey(metricB))
	require.Equal(t, []string{"b:2", "a:1"}, metricA.Tags, "input tags should not be mutated")
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

	calculator.ProcessMetrics([]*Metric{metric}, now)

	require.Equal(t, 42.0, metric.Value)
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

	calculator.ProcessMetrics([]*Metric{first}, t1)
	require.Equal(t, 0.0, first.Value)

	calculator.ProcessMetrics([]*Metric{second}, t2)
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

	calculator.ProcessMetrics([]*Metric{first}, t1)
	require.Equal(t, 0.0, first.Value)

	calculator.ProcessMetrics([]*Metric{second}, t2)
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

	calculator.ProcessMetrics([]*Metric{first}, t1)
	calculator.ProcessMetrics([]*Metric{sameTimestamp}, t2)
	calculator.ProcessMetrics([]*Metric{earlierTimestamp}, t3)

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

	calculator.ProcessMetrics([]*Metric{firstAbsolute, firstPerSecond}, t1)
	calculator.ProcessMetrics([]*Metric{secondAbsolute, secondPerSecond}, t2)

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

	calculator.ProcessMetrics(firstBatch, t1)
	calculator.ProcessMetrics(secondBatch, t2)

	require.Equal(t, 5.0, secondBatch[0].Value)
	require.Equal(t, 8.0, secondBatch[1].Value)
	require.Equal(t, 30.0, secondBatch[2].Value)
}
