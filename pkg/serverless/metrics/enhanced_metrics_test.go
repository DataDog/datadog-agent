// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessTags "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestGenerateEnhancedMetricsFromFunctionLogOutOfMemory(t *testing.T) {
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}
	reportLogTime := time.Now()
	isOOM := ContainsOutOfMemoryLog("JavaScript heap out of memory")
	if isOOM {
		GenerateOutOfMemoryEnhancedMetrics(reportLogTime, tags, demux)
	}

	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(2, 0, 100*time.Millisecond)
	assert.True(t, isOOM)
	assert.Len(t, generatedMetrics, 2, "two enhanced metrics should have been generated")
	assert.Len(t, timedMetrics, 0)
	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       OutOfMemoryMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       ErrorsMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}})
}

func TestGenerateEnhancedMetricsFromFunctionLogNoMetric(t *testing.T) {
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}
	isOOM := ContainsOutOfMemoryLog("Task timed out after 30.03 seconds")
	if isOOM {
		GenerateOutOfMemoryEnhancedMetrics(time.Now(), tags, demux)
	}

	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.False(t, isOOM)
	assert.Len(t, generatedMetrics, 0, "no metrics should have been generated")
	assert.Len(t, timedMetrics, 0)
}

func TestGenerateEnhancedMetricsFromReportLogColdStart(t *testing.T) {
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}
	reportLogTime := time.Now()
	runtimeStartTime := reportLogTime.Add(-20 * time.Millisecond)
	runtimeEndTime := reportLogTime.Add(-10 * time.Millisecond)
	args := GenerateEnhancedMetricsFromReportLogArgs{
		InitDurationMs:   100.0,
		DurationMs:       1000.0,
		BilledDurationMs: 800.0,
		MemorySizeMb:     1024.0,
		MaxMemoryUsedMb:  256.0,
		RuntimeStart:     runtimeStartTime,
		RuntimeEnd:       runtimeEndTime,
		T:                reportLogTime,
		Tags:             tags,
		Demux:            demux,
	}
	go GenerateEnhancedMetricsFromReportLog(args)

	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(7, 0, 100*time.Millisecond)

	assert.Equal(t, generatedMetrics[:7], []metrics.MetricSample{{
		Name:       maxMemoryUsedMetric,
		Value:      256.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       memorySizeMetric,
		Value:      1024.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       billedDurationMetric,
		Value:      0.80,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       durationMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       estimatedCostMetric,
		Value:      calculateEstimatedCost(800.0, 1024.0, serverlessTags.ResolveRuntimeArch()),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       postRuntimeDurationMetric,
		Value:      990.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       initDurationMetric,
		Value:      0.1,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}})
	assert.Len(t, timedMetrics, 0)
}

func TestGenerateEnhancedMetricsFromReportLogNoColdStart(t *testing.T) {
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}
	reportLogTime := time.Now()
	runtimeStartTime := reportLogTime.Add(-20 * time.Millisecond)
	runtimeEndTime := reportLogTime.Add(-10 * time.Millisecond)
	args := GenerateEnhancedMetricsFromReportLogArgs{
		InitDurationMs:   0,
		DurationMs:       1000.0,
		BilledDurationMs: 800.0,
		MemorySizeMb:     1024.0,
		MaxMemoryUsedMb:  256.0,
		RuntimeStart:     runtimeStartTime,
		RuntimeEnd:       runtimeEndTime,
		T:                reportLogTime,
		Tags:             tags,
		Demux:            demux,
	}
	go GenerateEnhancedMetricsFromReportLog(args)

	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(6, 0, 0100*time.Millisecond)

	assert.Equal(t, generatedMetrics[:6], []metrics.MetricSample{{
		Name:       maxMemoryUsedMetric,
		Value:      256.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       memorySizeMetric,
		Value:      1024.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       billedDurationMetric,
		Value:      0.80,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       durationMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       estimatedCostMetric,
		Value:      calculateEstimatedCost(800.0, 1024.0, serverlessTags.ResolveRuntimeArch()),
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       postRuntimeDurationMetric,
		Value:      990.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(reportLogTime.UnixNano()) / float64(time.Second),
	}})
	assert.Len(t, timedMetrics, 0)
}

func TestSendTimeoutEnhancedMetric(t *testing.T) {
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}

	go SendTimeoutEnhancedMetric(tags, demux)

	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(1, 0, 100*time.Millisecond)

	assert.Equal(t, generatedMetrics[:1], []metrics.MetricSample{{
		Name:       timeoutsMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		// compare the generated timestamp to itself because we can't know its value
		Timestamp: generatedMetrics[0].Timestamp,
	}})
	assert.Len(t, timedMetrics, 0)
}

func TestSendInvocationEnhancedMetric(t *testing.T) {
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}

	go SendInvocationEnhancedMetric(tags, demux)

	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(1, 0, 100*time.Millisecond)

	assert.Equal(t, generatedMetrics[:1], []metrics.MetricSample{{
		Name:       invocationsMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		// compare the generated timestamp to itself because we can't know its value
		Timestamp: generatedMetrics[0].Timestamp,
	}})
	assert.Len(t, timedMetrics, 0)
}

func TestDisableEnhancedMetrics(t *testing.T) {
	os.Setenv("DD_ENHANCED_METRICS", "false")
	defer os.Setenv("DD_ENHANCED_METRICS", "true")
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}

	go SendInvocationEnhancedMetric(tags, demux)

	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(1, 0, 100*time.Millisecond)

	assert.Len(t, generatedMetrics, 0)
	assert.Len(t, timedMetrics, 0)
}

func TestSendOutOfMemoryEnhancedMetric(t *testing.T) {
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}
	mockTime := time.Now()
	go SendOutOfMemoryEnhancedMetric(tags, mockTime, demux)

	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(1, 0, 100*time.Millisecond)

	assert.Equal(t, generatedMetrics[:1], []metrics.MetricSample{{
		Name:       OutOfMemoryMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(mockTime.UnixNano()) / float64(time.Second),
	}})
	assert.Len(t, timedMetrics, 0)
}

func TestSendErrorsEnhancedMetric(t *testing.T) {
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}
	mockTime := time.Now()
	go SendErrorsEnhancedMetric(tags, mockTime, demux)

	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(1, 0, 100*time.Millisecond)

	assert.Equal(t, generatedMetrics[:1], []metrics.MetricSample{{
		Name:       ErrorsMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(mockTime.UnixNano()) / float64(time.Second),
	}})
	assert.Len(t, timedMetrics, 0)
}

func TestCalculateEstimatedCost(t *testing.T) {
	// Latest Lambda pricing and billing examples from https://aws.amazon.com/lambda/pricing/
	// two different architects: X86_64 and Arm64
	const freeTierX86ComputeCost = x86LambdaPricePerGbSecond * 400000
	const freeTierArmComputeCost = armLambdaPricePerGbSecond * 400000
	const freeTierRequestCost = baseLambdaInvocationPrice * 1000000
	const freeTierX86CostAdjustment = freeTierX86ComputeCost + freeTierRequestCost
	const freeTierArmCostAdjustment = freeTierArmComputeCost + freeTierRequestCost

	// The case of X86_64
	// Example 1: If you allocated 512MB of memory to your function, executed it 3 million times in one month,
	// and it ran for 1 second each time, your charges would be $18.74
	estimatedCost := 3000000.0 * calculateEstimatedCost(1000.0, 512.0, serverlessTags.X86LambdaPlatform)
	assert.InDelta(t, 18.74, estimatedCost-freeTierX86CostAdjustment, 0.01)
	// Example 2: If you allocated 128MB of memory to your function, executed it 30 million times in one month,
	// and it ran for 200ms each time, your charges would be $11.63
	estimatedCost = 30000000.0 * calculateEstimatedCost(200.0, 128.0, serverlessTags.X86LambdaPlatform)
	assert.InDelta(t, 11.63, estimatedCost-freeTierX86CostAdjustment, 0.01)

	// The case of Amd64, which is an extension of X86_64
	// Example 1: If you allocated 512MB of memory to your function, executed it 3 million times in one month,
	// and it ran for 1 second each time, your charges would be $18.74
	estimatedCost = 3000000.0 * calculateEstimatedCost(1000.0, 512.0, serverlessTags.AmdLambdaPlatform)
	assert.InDelta(t, 18.74, estimatedCost-freeTierX86CostAdjustment, 0.01)
	// Example 2: If you allocated 128MB of memory to your function, executed it 30 million times in one month,
	// and it ran for 200ms each time, your charges would be $11.63
	estimatedCost = 30000000.0 * calculateEstimatedCost(200.0, 128.0, serverlessTags.AmdLambdaPlatform)
	assert.InDelta(t, 11.63, estimatedCost-freeTierX86CostAdjustment, 0.01)

	// The case of Arm86
	// Example 1: If you allocated 512MB of memory to your function, executed it 3 million times in one month,
	// and it ran for 1 second each time, your charges would be $15.07
	estimatedCost = 3000000.0 * calculateEstimatedCost(1000.0, 512.0, serverlessTags.ArmLambdaPlatform)
	assert.InDelta(t, 15.07, estimatedCost-freeTierArmCostAdjustment, 0.01)
	// Example 2: If you allocated 128MB of memory to your function, executed it 30 million times in one month,
	// and it ran for 200ms each time, your charges would be $10.47
	estimatedCost = 30000000.0 * calculateEstimatedCost(200.0, 128.0, serverlessTags.ArmLambdaPlatform)
	assert.InDelta(t, 10.47, estimatedCost-freeTierArmCostAdjustment, 0.01)
}

func TestGenerateEnhancedMetricsFromRuntimeDoneLogNoStartDate(t *testing.T) {
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}
	startTime := time.Time{}
	endTime := time.Now()
	args := GenerateEnhancedMetricsFromRuntimeDoneLogArgs{
		Start:            startTime,
		End:              endTime,
		ResponseLatency:  19,
		ResponseDuration: 3,
		ProducedBytes:    53,
		Tags:             tags,
		Demux:            demux,
	}
	go GenerateEnhancedMetricsFromRuntimeDoneLog(args)
	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(3, 0, 100*time.Millisecond)
	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       responseLatencyMetric,
		Value:      19,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       responseDurationMetric,
		Value:      3,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       producedBytesMetric,
		Value:      53,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.UnixNano()) / float64(time.Second),
	}})
	assert.Len(t, timedMetrics, 0)
}

func TestGenerateEnhancedMetricsFromRuntimeDoneLogNoEndDate(t *testing.T) {
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}
	startTime := time.Now()
	endTime := time.Time{}
	args := GenerateEnhancedMetricsFromRuntimeDoneLogArgs{
		Start:            startTime,
		End:              endTime,
		ResponseLatency:  19,
		ResponseDuration: 3,
		ProducedBytes:    53,
		Tags:             tags,
		Demux:            demux,
	}
	go GenerateEnhancedMetricsFromRuntimeDoneLog(args)
	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(3, 0, 100*time.Millisecond)
	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       responseLatencyMetric,
		Value:      19,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       responseDurationMetric,
		Value:      3,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       producedBytesMetric,
		Value:      53,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.UnixNano()) / float64(time.Second),
	}})
	assert.Len(t, timedMetrics, 0)
}

func TestGenerateEnhancedMetricsFromRuntimeDoneLogOK(t *testing.T) {
	demux := createDemultiplexer(t)
	tags := []string{"functionname:test-function"}
	startTime := time.Date(2020, 01, 01, 01, 01, 01, 500000000, time.UTC)
	endTime := time.Date(2020, 01, 01, 01, 01, 01, 653000000, time.UTC) //153 ms later
	args := GenerateEnhancedMetricsFromRuntimeDoneLogArgs{
		Start:            startTime,
		End:              endTime,
		ResponseLatency:  19,
		ResponseDuration: 3,
		ProducedBytes:    53,
		Tags:             tags,
		Demux:            demux,
	}
	go GenerateEnhancedMetricsFromRuntimeDoneLog(args)
	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(4, 0, 100*time.Millisecond)
	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       runtimeDurationMetric,
		Value:      153,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       responseLatencyMetric,
		Value:      19,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       responseDurationMetric,
		Value:      3,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.UnixNano()) / float64(time.Second),
	}, {
		Name:       producedBytesMetric,
		Value:      53,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(endTime.UnixNano()) / float64(time.Second),
	}})
	assert.Len(t, timedMetrics, 0)
}

func createDemultiplexer(t *testing.T) demultiplexer.FakeSamplerMock {
	return fxutil.Test[demultiplexer.FakeSamplerMock](t, logimpl.MockModule(), compressionimpl.MockModule(), demultiplexerimpl.FakeSamplerMockModule(), hostnameimpl.MockModule())
}
