//go:build linux_bpf
// +build linux_bpf

package runtime

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// CompilationResult enumerates runtime compilation success & failure modes
type CompilationResult int

const (
	notAttempted CompilationResult = iota
	compilationSuccess
	kernelVersionErr
	verificationError
	outputDirErr
	outputFileErr
	newCompilerErr
	compilationErr
	resultReadErr
	headerFetchErr
	compiledOutputFound
	inputHashError
)

type RuntimeCompilationTelemetry struct {
	compilationEnabled  bool
	compilationResult   CompilationResult
	compilationDuration time.Duration
}

func newRuntimeCompilationTelemetry() RuntimeCompilationTelemetry {
	return RuntimeCompilationTelemetry{
		compilationEnabled: false,
		compilationResult:  notAttempted,
	}
}

func (tm *RuntimeCompilationTelemetry) CompilationEnabled() bool {
	return tm.compilationEnabled
}

func (tm *RuntimeCompilationTelemetry) CompilationResult() int32 {
	return int32(tm.compilationResult)
}

func (tm *RuntimeCompilationTelemetry) CompilationDurationNS() int64 {
	return tm.compilationDuration.Nanoseconds()
}

func (tm *RuntimeCompilationTelemetry) SendMetrics(metricPrefix string, client statsd.ClientInterface, tags []string) {
	compilationEnabledMetric := metricPrefix + ".enabled"
	compilationResultMetric := metricPrefix + ".compilation_result"
	compilationDurationtMetric := metricPrefix + ".compilation_duration"

	var enabled float64 = 0
	if tm.compilationEnabled {
		enabled = 1
	}

	if err := client.Gauge(compilationEnabledMetric, enabled, tags, 1); err != nil {
		log.Errorf("error sending %s metric: %s", compilationEnabledMetric, err)
	}

	if !tm.compilationEnabled {
		return
	}

	if err := client.Gauge(compilationResultMetric, float64(tm.compilationResult), tags, 1); err != nil {
		log.Errorf("error sending %s metric: %s", compilationResultMetric, err)
	}
	if err := client.Gauge(compilationDurationtMetric, float64(tm.compilationDuration), tags, 1); err != nil {
		log.Errorf("error sending %s metric: %s", compilationDurationtMetric, err)
	}
}
