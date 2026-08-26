// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package reporterimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStdoutReporterCountsEveryReportKind(t *testing.T) {
	tel := telemetryimpl.GetCompatComponent()
	tel.Reset()
	t.Cleanup(tel.Reset)

	reporter := &stdoutReporter{
		emittedCounter: newReportsEmittedCounter(tel),
	}

	for _, kind := range []observerdef.CorrelatorEventKind{
		observerdef.CorrelatorEventEpisodeStarted,
		observerdef.CorrelatorEventEpisodeEnded,
		observerdef.CorrelatorEventCorrelationDetected,
	} {
		emitted := reporter.Report(reporterOutputWithEvent(kind))
		require.True(t, emitted)
	}

	assert.Equal(t, 1.0, reportCounterValue(t, tel, reportKindEpisodeStarted))
	assert.Equal(t, 1.0, reportCounterValue(t, tel, reportKindEpisodeEnded))
	assert.Equal(t, 1.0, reportCounterValue(t, tel, reportKindCorrelation))
}

func reporterOutputWithEvent(kind observerdef.CorrelatorEventKind) reporterdef.ReportOutput {
	return reporterdef.ReportOutput{
		CorrelatorEvents: []observerdef.CorrelatorEvent{{
			Kind: kind,
			Correlation: observerdef.ActiveCorrelation{
				Pattern: "test-pattern",
			},
		}},
	}
}

func reportCounterValue(t *testing.T, tel telemetry.Component, kind string) float64 {
	t.Helper()

	metricFamilies, err := tel.Gather(false)
	require.NoError(t, err)

	for _, family := range metricFamilies {
		if family.GetName() != "observer__"+telemetryReportsEmitted {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := metric.GetLabel()
			if len(labels) == 1 && labels[0].GetName() == "kind" && labels[0].GetValue() == kind {
				return metric.GetCounter().GetValue()
			}
		}
	}

	t.Fatalf("counter %q with kind=%q not found", telemetryReportsEmitted, kind)
	return 0
}
