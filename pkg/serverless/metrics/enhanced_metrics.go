// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package metrics

import (
	"math"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/proc"
	serverlessTags "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Latest Lambda pricing per https://aws.amazon.com/lambda/pricing/
	baseLambdaInvocationPrice = 0.0000002
	x86LambdaPricePerGbSecond = 0.0000166667
	armLambdaPricePerGbSecond = 0.0000133334
	msToSec                   = 0.001

	// tmp directory path
	tmpPath = "/tmp/"

	// Enhanced metrics
	maxMemoryUsedMetric       = "aws.lambda.enhanced.max_memory_used"
	memorySizeMetric          = "aws.lambda.enhanced.memorysize"
	runtimeDurationMetric     = "aws.lambda.enhanced.runtime_duration"
	billedDurationMetric      = "aws.lambda.enhanced.billed_duration"
	durationMetric            = "aws.lambda.enhanced.duration"
	postRuntimeDurationMetric = "aws.lambda.enhanced.post_runtime_duration"
	estimatedCostMetric       = "aws.lambda.enhanced.estimated_cost"
	initDurationMetric        = "aws.lambda.enhanced.init_duration"
	responseLatencyMetric     = "aws.lambda.enhanced.response_latency"
	responseDurationMetric    = "aws.lambda.enhanced.response_duration"
	producedBytesMetric       = "aws.lambda.enhanced.produced_bytes"
	// OutOfMemoryMetric is the name of the out of memory enhanced Lambda metric
	OutOfMemoryMetric = "aws.lambda.enhanced.out_of_memory"
	timeoutsMetric    = "aws.lambda.enhanced.timeouts"
	// ErrorsMetric is the name of the errors enhanced Lambda metric
	ErrorsMetric                 = "aws.lambda.enhanced.errors"
	invocationsMetric            = "aws.lambda.enhanced.invocations"
	asmInvocationsMetric         = "aws.lambda.enhanced.asm.invocations"
	cpuSystemTimeMetric          = "aws.lambda.enhanced.cpu_system_time"
	cpuUserTimeMetric            = "aws.lambda.enhanced.cpu_user_time"
	cpuTotalTimeMetric           = "aws.lambda.enhanced.cpu_total_time"
	cpuTotalUtilizationPctMetric = "aws.lambda.enhanced.cpu_total_utilization_pct"
	cpuTotalUtilizationMetric    = "aws.lambda.enhanced.cpu_total_utilization"
	numCoresMetric               = "aws.lambda.enhanced.num_cores"
	cpuMaxUtilizationMetric      = "aws.lambda.enhanced.cpu_max_utilization"
	cpuMinUtilizationMetric      = "aws.lambda.enhanced.cpu_min_utilization"
	rxBytesMetric                = "aws.lambda.enhanced.rx_bytes"
	txBytesMetric                = "aws.lambda.enhanced.tx_bytes"
	totalNetworkMetric           = "aws.lambda.enhanced.total_network"
	tmpUsedMetric                = "aws.lambda.enhanced.tmp_used"
	tmpMaxMetric                 = "aws.lambda.enhanced.tmp_max"
	fdMaxMetric                  = "aws.lambda.enhanced.fd_max"
	fdUseMetric                  = "aws.lambda.enhanced.fd_use"
	threadsMaxMetric             = "aws.lambda.enhanced.threads_max"
	threadsUseMetric             = "aws.lambda.enhanced.threads_use"
	enhancedMetricsEnvVar        = "DD_ENHANCED_METRICS"

	// Bottlecap
	failoverMetric = "datadog.serverless.extension.failover"
)

var enhancedMetricsDisabled = strings.ToLower(os.Getenv(enhancedMetricsEnvVar)) == "false"

func getOutOfMemorySubstrings() []string {
	return []string{
		"fatal error: runtime: out of memory",       // Go
		"java.lang.OutOfMemoryError",                // Java
		"JavaScript heap out of memory",             // Node
		"Runtime exited with error: signal: killed", // Node
		"MemoryError", // Python
		"failed to allocate memory (NoMemoryError)", // Ruby
		"OutOfMemoryException",                      // .NET
	}
}

// GenerateEnhancedMetricsFromRuntimeDoneLogArgs are the arguments required for
// the GenerateEnhancedMetricsFromRuntimeDoneLog func
type GenerateEnhancedMetricsFromRuntimeDoneLogArgs struct {
	Start            time.Time
	End              time.Time
	ResponseLatency  float64
	ResponseDuration float64
	ProducedBytes    float64
	Tags             []string
	Demux            aggregator.Demultiplexer
}

// GenerateEnhancedMetricsFromRuntimeDoneLog generates the runtime duration metric
func GenerateEnhancedMetricsFromRuntimeDoneLog(args GenerateEnhancedMetricsFromRuntimeDoneLogArgs) {
	// first check if both date are set
	if args.Start.IsZero() || args.End.IsZero() {
		log.Debug("Impossible to compute aws.lambda.enhanced.runtime_duration due to an invalid interval")
	} else {
		duration := args.End.Sub(args.Start).Milliseconds()
		args.Demux.AggregateSample(metrics.MetricSample{
			Name:       runtimeDurationMetric,
			Value:      float64(duration),
			Mtype:      metrics.DistributionType,
			Tags:       args.Tags,
			SampleRate: 1,
			Timestamp:  float64(args.End.UnixNano()) / float64(time.Second),
		})
	}
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       responseLatencyMetric,
		Value:      args.ResponseLatency,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  float64(args.End.UnixNano()) / float64(time.Second),
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       responseDurationMetric,
		Value:      args.ResponseDuration,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  float64(args.End.UnixNano()) / float64(time.Second),
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       producedBytesMetric,
		Value:      args.ProducedBytes,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  float64(args.End.UnixNano()) / float64(time.Second),
	})
}

// ContainsOutOfMemoryLog determines whether a runtime specific out of memory string is found in the log line
func ContainsOutOfMemoryLog(logString string) bool {
	for _, substring := range getOutOfMemorySubstrings() {
		if strings.Contains(logString, substring) {
			log.Debug("Found out of memory substring in function log line")
			return true
		}
	}
	return false
}

// GenerateOutOfMemoryEnhancedMetrics generates enhanced metrics specific to an out of memory error
func GenerateOutOfMemoryEnhancedMetrics(time time.Time, tags []string, demux aggregator.Demultiplexer) {
	SendOutOfMemoryEnhancedMetric(tags, time, demux)
	SendErrorsEnhancedMetric(tags, time, demux)
}

// GenerateEnhancedMetricsFromReportLogArgs provides the arguments required for
// the GenerateEnhancedMetricsFromReportLog func
type GenerateEnhancedMetricsFromReportLogArgs struct {
	InitDurationMs   float64
	DurationMs       float64
	BilledDurationMs int
	MemorySizeMb     int
	MaxMemoryUsedMb  int
	RuntimeStart     time.Time
	RuntimeEnd       time.Time
	T                time.Time
	Tags             []string
	Demux            aggregator.Demultiplexer
}

// GenerateEnhancedMetricsFromReportLog generates enhanced metrics from a LogTypePlatformReport log message
func GenerateEnhancedMetricsFromReportLog(args GenerateEnhancedMetricsFromReportLogArgs) {
	timestamp := float64(args.T.UnixNano()) / float64(time.Second)
	billedDuration := float64(args.BilledDurationMs)
	memorySize := float64(args.MemorySizeMb)
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       maxMemoryUsedMetric,
		Value:      float64(args.MaxMemoryUsedMb),
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       memorySizeMetric,
		Value:      memorySize,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       billedDurationMetric,
		Value:      billedDuration * msToSec,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       durationMetric,
		Value:      args.DurationMs * msToSec,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       estimatedCostMetric,
		Value:      calculateEstimatedCost(billedDuration, memorySize, serverlessTags.ResolveRuntimeArch()),
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
	if args.RuntimeStart.IsZero() || args.RuntimeEnd.IsZero() {
		log.Debug("Impossible to compute aws.lambda.enhanced.post_runtime_duration due to an invalid interval")
	} else {
		postRuntimeDuration := args.DurationMs - float64(args.RuntimeEnd.Sub(args.RuntimeStart).Milliseconds())
		args.Demux.AggregateSample(metrics.MetricSample{
			Name:       postRuntimeDurationMetric,
			Value:      postRuntimeDuration,
			Mtype:      metrics.DistributionType,
			Tags:       args.Tags,
			SampleRate: 1,
			Timestamp:  timestamp,
		})
	}
	if args.InitDurationMs > 0 {
		args.Demux.AggregateSample(metrics.MetricSample{
			Name:       initDurationMetric,
			Value:      args.InitDurationMs * msToSec,
			Mtype:      metrics.DistributionType,
			Tags:       args.Tags,
			SampleRate: 1,
			Timestamp:  timestamp,
		})
	}
}

// SendOutOfMemoryEnhancedMetric sends an enhanced metric representing a function running out of memory at a given time
func SendOutOfMemoryEnhancedMetric(tags []string, t time.Time, demux aggregator.Demultiplexer) {
	incrementEnhancedMetric(OutOfMemoryMetric, tags, float64(t.UnixNano())/float64(time.Second), demux, false)
}

// SendErrorsEnhancedMetric sends an enhanced metric representing an error at a given time
func SendErrorsEnhancedMetric(tags []string, t time.Time, demux aggregator.Demultiplexer) {
	incrementEnhancedMetric(ErrorsMetric, tags, float64(t.UnixNano())/float64(time.Second), demux, false)
}

// SendTimeoutEnhancedMetric sends an enhanced metric representing a timeout at the current time
func SendTimeoutEnhancedMetric(tags []string, demux aggregator.Demultiplexer) {
	incrementEnhancedMetric(timeoutsMetric, tags, float64(time.Now().UnixNano())/float64(time.Second), demux, false)
}

// SendInvocationEnhancedMetric sends an enhanced metric representing an invocation at the current time
func SendInvocationEnhancedMetric(tags []string, demux aggregator.Demultiplexer) {
	incrementEnhancedMetric(invocationsMetric, tags, float64(time.Now().UnixNano())/float64(time.Second), demux, false)
}

// SendASMInvocationEnhancedMetric sends an enhanced metric representing an appsec supported invocation at the current time
// Metric is sent even if enhanced metrics are disabled
func SendASMInvocationEnhancedMetric(tags []string, demux aggregator.Demultiplexer) {
	incrementEnhancedMetric(asmInvocationsMetric, tags, float64(time.Now().UnixNano())/float64(time.Second), demux, true)
}

type generateCPUEnhancedMetricsArgs struct {
	UserCPUTimeMs   float64
	SystemCPUTimeMs float64
	Uptime          float64
	Tags            []string
	Demux           aggregator.Demultiplexer
	Time            float64
}

type GenerateCPUUtilizationEnhancedMetricArgs struct {
	IndividualCPUIdleTimes       map[string]float64
	IndividualCPUIdleOffsetTimes map[string]float64
	IdleTimeMs                   float64
	IdleTimeOffsetMs             float64
	UptimeMs                     float64
	UptimeOffsetMs               float64
	Tags                         []string
	Demux                        aggregator.Demultiplexer
	Time                         float64
}

// generateCPUEnhancedMetrics generates enhanced metrics for CPU time spent running the function in kernel mode,
// in user mode, and in total
func generateCPUEnhancedMetrics(args generateCPUEnhancedMetricsArgs) {
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       cpuSystemTimeMetric,
		Value:      args.SystemCPUTimeMs,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       cpuUserTimeMetric,
		Value:      args.UserCPUTimeMs,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       cpuTotalTimeMetric,
		Value:      args.SystemCPUTimeMs + args.UserCPUTimeMs,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
}

func SendFailoverReasonMetric(tags []string, demux aggregator.Demultiplexer) {
	demux.AggregateSample(metrics.MetricSample{
		Name:       failoverMetric,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  float64(time.Now().UnixNano()) / float64(time.Second),
	})
}

// SendCPUEnhancedMetrics sends CPU enhanced metrics for the invocation
func SendCPUEnhancedMetrics(cpuOffsetData *proc.CPUData, uptimeOffset float64, tags []string, demux aggregator.Demultiplexer) {
	if enhancedMetricsDisabled {
		return
	}
	cpuData, err := proc.GetCPUData()
	if err != nil {
		log.Debug("Could not emit CPU enhanced metrics")
		return
	}

	now := float64(time.Now().UnixNano()) / float64(time.Second)
	generateCPUEnhancedMetrics(generateCPUEnhancedMetricsArgs{
		UserCPUTimeMs:   cpuData.TotalUserTimeMs - cpuOffsetData.TotalUserTimeMs,
		SystemCPUTimeMs: cpuData.TotalSystemTimeMs - cpuOffsetData.TotalSystemTimeMs,
		Tags:            tags,
		Demux:           demux,
		Time:            now,
	})

	perCoreData := cpuData.IndividualCPUIdleTimes
	if perCoreData != nil {
		uptimeMs, err := proc.GetUptime()
		if err != nil {
			log.Debug("Could not emit CPU enhanced metrics")
			return
		}
		GenerateCPUUtilizationEnhancedMetrics(GenerateCPUUtilizationEnhancedMetricArgs{
			cpuData.IndividualCPUIdleTimes,
			cpuOffsetData.IndividualCPUIdleTimes,
			cpuData.TotalIdleTimeMs,
			cpuOffsetData.TotalIdleTimeMs,
			uptimeMs,
			uptimeOffset,
			tags,
			demux,
			now,
		})
	}

}

func GenerateCPUUtilizationEnhancedMetrics(args GenerateCPUUtilizationEnhancedMetricArgs) {
	maxIdleTime := 0.0
	minIdleTime := math.MaxFloat64
	for cpuName, cpuIdleTime := range args.IndividualCPUIdleTimes {
		adjustedIdleTime := cpuIdleTime - args.IndividualCPUIdleOffsetTimes[cpuName]
		// Maximally utilized CPU is the one with the least time spent in the idle process
		if adjustedIdleTime < minIdleTime {
			minIdleTime = adjustedIdleTime
		}
		// Minimally utilized CPU is the one with the most time spent in the idle process
		if adjustedIdleTime >= maxIdleTime {
			maxIdleTime = adjustedIdleTime
		}
	}

	adjustedUptime := args.UptimeMs - args.UptimeOffsetMs

	maxUtilizedPercent := 100 * (adjustedUptime - minIdleTime) / adjustedUptime
	minUtilizedPercent := 100 * (adjustedUptime - maxIdleTime) / adjustedUptime

	numberCPUs := float64(len(args.IndividualCPUIdleTimes))
	adjustedIdleTime := args.IdleTimeMs - args.IdleTimeOffsetMs
	totalUtilizedDecimal := (adjustedUptime*numberCPUs - adjustedIdleTime) / (adjustedUptime * numberCPUs)
	totalUtilizedPercent := 100 * totalUtilizedDecimal
	totalUtilizedCores := numberCPUs * totalUtilizedDecimal

	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       cpuTotalUtilizationPctMetric,
		Value:      totalUtilizedPercent,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       cpuTotalUtilizationMetric,
		Value:      totalUtilizedCores,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       numCoresMetric,
		Value:      float64(len(args.IndividualCPUIdleTimes)),
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       cpuMaxUtilizationMetric,
		Value:      maxUtilizedPercent,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       cpuMinUtilizationMetric,
		Value:      minUtilizedPercent,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
}

func SendNetworkEnhancedMetrics(networkOffsetData *proc.NetworkData, tags []string, demux aggregator.Demultiplexer) {
	if enhancedMetricsDisabled {
		return
	}

	networkData, err := proc.GetNetworkData()
	if err != nil {
		log.Debug("Could not emit network enhanced metrics")
		return
	}

	now := float64(time.Now().UnixNano()) / float64(time.Second)
	generateNetworkEnhancedMetrics(generateNetworkEnhancedMetricArgs{
		networkOffsetData.RxBytes,
		networkData.RxBytes,
		networkOffsetData.TxBytes,
		networkData.TxBytes,
		tags,
		demux,
		now,
	})
}

type generateNetworkEnhancedMetricArgs struct {
	RxBytesOffset float64
	RxBytes       float64
	TxBytesOffset float64
	TxBytes       float64
	Tags          []string
	Demux         aggregator.Demultiplexer
	Time          float64
}

func generateNetworkEnhancedMetrics(args generateNetworkEnhancedMetricArgs) {
	adjustedRxBytes := args.RxBytes - args.RxBytesOffset
	adjustedTxBytes := args.TxBytes - args.TxBytesOffset
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       rxBytesMetric,
		Value:      adjustedRxBytes,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       txBytesMetric,
		Value:      adjustedTxBytes,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       totalNetworkMetric,
		Value:      adjustedRxBytes + adjustedTxBytes,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
}

type generateTmpEnhancedMetricsArgs struct {
	TmpMax  float64
	TmpUsed float64
	Tags    []string
	Demux   aggregator.Demultiplexer
	Time    float64
}

// generateTmpEnhancedMetrics generates enhanced metrics for space used and available in the /tmp directory
func generateTmpEnhancedMetrics(args generateTmpEnhancedMetricsArgs) {
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       tmpUsedMetric,
		Value:      args.TmpUsed,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       tmpMaxMetric,
		Value:      args.TmpMax,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
}

func SendTmpEnhancedMetrics(sendMetrics chan bool, tags []string, metricAgent *ServerlessMetricAgent) {
	if enhancedMetricsDisabled {
		return
	}

	bsize, blocks, bavail, err := statfs(tmpPath)
	if err != nil {
		log.Debugf("Could not emit tmp enhanced metrics. %v", err)
		return
	}
	tmpMax := blocks * bsize
	tmpUsed := bsize * (blocks - bavail)

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case _, open := <-sendMetrics:
			if !open {
				generateTmpEnhancedMetrics(generateTmpEnhancedMetricsArgs{
					TmpMax:  tmpMax,
					TmpUsed: tmpUsed,
					Tags:    tags,
					Demux:   metricAgent.Demux,
					Time:    float64(time.Now().UnixNano()) / float64(time.Second),
				})
				return
			}
		case <-ticker.C:
			bsize, blocks, bavail, err = statfs(tmpPath)
			if err != nil {
				log.Debugf("Could not emit tmp enhanced metrics. %v", err)
				return
			}
			tmpUsed = math.Max(tmpUsed, bsize*(blocks-bavail))
		}
	}

}

type generateFdEnhancedMetricsArgs struct {
	FdMax float64
	FdUse float64
	Tags  []string
	Demux aggregator.Demultiplexer
	Time  float64
}

type generateThreadEnhancedMetricsArgs struct {
	ThreadsMax float64
	ThreadsUse float64
	Tags       []string
	Demux      aggregator.Demultiplexer
	Time       float64
}

// generateFdEnhancedMetrics generates enhanced metrics for the maximum number of file descriptors available and in use
func generateFdEnhancedMetrics(args generateFdEnhancedMetricsArgs) {
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       fdMaxMetric,
		Value:      args.FdMax,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       fdUseMetric,
		Value:      args.FdUse,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
}

// generateThreadEnhancedMetrics generates enhanced metrics for the maximum number of threads available and in use
func generateThreadEnhancedMetrics(args generateThreadEnhancedMetricsArgs) {
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       threadsMaxMetric,
		Value:      args.ThreadsMax,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
	args.Demux.AggregateSample(metrics.MetricSample{
		Name:       threadsUseMetric,
		Value:      args.ThreadsUse,
		Mtype:      metrics.DistributionType,
		Tags:       args.Tags,
		SampleRate: 1,
		Timestamp:  args.Time,
	})
}

func SendProcessEnhancedMetrics(sendMetrics chan bool, tags []string, metricAgent *ServerlessMetricAgent) {
	if enhancedMetricsDisabled {
		return
	}

	pids := proc.GetPidList(proc.ProcPath)

	fdMaxData, err := proc.GetFileDescriptorMaxData(pids)
	if err != nil {
		log.Debug("Could not emit file descriptor enhanced metrics. %v", err)
		return
	}

	fdUseData, err := proc.GetFileDescriptorUseData(pids)
	if err != nil {
		log.Debugf("Could not emit file descriptor enhanced metrics. %v", err)
		return
	}

	threadsMaxData, err := proc.GetThreadsMaxData(pids)
	if err != nil {
		log.Debugf("Could not emit thread enhanced metrics. %v", err)
		return
	}

	threadsUseData, err := proc.GetThreadsUseData(pids)
	if err != nil {
		log.Debugf("Could not emit thread enhanced metrics. %v", err)
		return
	}

	fdMax := fdMaxData.MaximumFileHandles
	fdUse := fdUseData.UseFileHandles
	threadsMax := threadsMaxData.ThreadsMax
	threadsUse := threadsUseData.ThreadsUse

	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case _, open := <-sendMetrics:
			if !open {
				generateFdEnhancedMetrics(generateFdEnhancedMetricsArgs{
					FdMax: fdMax,
					FdUse: fdUse,
					Tags:  tags,
					Demux: metricAgent.Demux,
					Time:  float64(time.Now().UnixNano()) / float64(time.Second),
				})
				generateThreadEnhancedMetrics(generateThreadEnhancedMetricsArgs{
					ThreadsMax: threadsMax,
					ThreadsUse: threadsUse,
					Tags:       tags,
					Demux:      metricAgent.Demux,
					Time:       float64(time.Now().UnixNano()) / float64(time.Second),
				})
				return
			}
		case <-ticker.C:
			pids := proc.GetPidList(proc.ProcPath)

			fdUseData, err := proc.GetFileDescriptorUseData(pids)
			if err != nil {
				log.Debugf("Could not emit file descriptor enhanced metrics. %v", err)
				return
			}
			fdUse = math.Max(fdUse, fdUseData.UseFileHandles)

			threadsUseData, err := proc.GetThreadsUseData(pids)
			if err != nil {
				log.Debugf("Could not emit thread enhanced metric. %v", err)
				return
			}
			threadsUse = math.Max(threadsUse, threadsUseData.ThreadsUse)
		}
	}
}

// incrementEnhancedMetric sends an enhanced metric with a value of 1 to the metrics channel
func incrementEnhancedMetric(name string, tags []string, timestamp float64, demux aggregator.Demultiplexer, force bool) {
	// TODO - pass config here, instead of directly looking up var
	if !force && enhancedMetricsDisabled {
		return
	}
	demux.AggregateSample(metrics.MetricSample{
		Name:       name,
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  timestamp,
	})
}

// calculateEstimatedCost returns the estimated cost in USD of a Lambda invocation
func calculateEstimatedCost(billedDurationMs float64, memorySizeMb float64, architecture string) float64 {
	billedDurationSeconds := billedDurationMs / 1000.0
	memorySizeGb := memorySizeMb / 1024.0
	gbSeconds := billedDurationSeconds * memorySizeGb
	// round the final float result because float math could have float point imprecision
	// on some arch. (i.e. 1.00000000000002 values)
	return math.Round((baseLambdaInvocationPrice+(gbSeconds*getLambdaPricePerGbSecond(architecture)))*10e12) / 10e12
}

// get the lambda price per Gb second based on the runtime platform
func getLambdaPricePerGbSecond(architecture string) float64 {
	switch architecture {
	case serverlessTags.ArmLambdaPlatform:
		// for arm64
		return armLambdaPricePerGbSecond
	default:
		// for x86 and amd64
		return x86LambdaPricePerGbSecond
	}
}
