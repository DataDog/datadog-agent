// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import "go.opentelemetry.io/collector/featuregate"

// metricsV3SeriesGate gates DDOT's v3 series opt-in for a staged, operator-controlled
// rollout. Alpha (off by default); enable with
// --feature-gates=+datadog.otelagent.MetricsV3Series.
var metricsV3SeriesGate = featuregate.GlobalRegistry().MustRegister(
	"datadog.otelagent.MetricsV3Series",
	featuregate.StageAlpha,
	featuregate.WithRegisterDescription("Route DDOT time-series metrics to the v3 metrics intake."),
	featuregate.WithRegisterReferenceURL("https://datadoghq.atlassian.net/browse/OTAGENT-1130"),
)
