// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// anomalySeverityLogger is an example ScorerListener that logs every severity
// transition produced by the anomaly scorer and emits it as a telemetry gauge.
// It subscribes to all transitions (Low, Medium, High) in both directions.
//
// Register at observer startup via:
//
//	obs.SubscribeScorer(observerdef.AnomalyScorerConfiguration{
//	    Listener: newAnomalySeverityLogger(stateGauge),
//	})
type anomalySeverityLogger struct {
	// stateGauge is set on every severity transition:
	// value = numeric severity level (0=Low, 1=Medium, 2=High),
	// tags  = scorer name and transition direction.
	stateGauge telemetry.Gauge
}

func newAnomalySeverityLogger(stateGauge telemetry.Gauge) *anomalySeverityLogger {
	return &anomalySeverityLogger{stateGauge: stateGauge}
}

// OnSeverityTransition implements observerdef.ScorerListener.
func (l *anomalySeverityLogger) OnSeverityTransition(evt observerdef.SeverityEvent) {
	direction := "escalation"
	if evt.Direction == observerdef.ScorerEventDeescalation {
		direction = "deescalation"
	}
	pkglog.Infof("[observer] anomaly scorer %s to %s (was %s, t=%d)",
		direction,
		severityLevelName(evt.ToLevel),
		severityLevelName(evt.FromLevel),
		evt.Timestamp,
	)
	l.stateGauge.Set(float64(evt.ToLevel), "anomaly_scorer", direction)
}

// severityLevelName returns a human-readable label for a SeverityLevel.
func severityLevelName(l observerdef.SeverityLevel) string {
	switch l {
	case observerdef.SeverityLow:
		return "Low"
	case observerdef.SeverityMedium:
		return "Medium"
	case observerdef.SeverityHigh:
		return "High"
	default:
		return fmt.Sprintf("SeverityLevel(%d)", int(l))
	}
}
