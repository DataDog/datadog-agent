// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && !serverless && test

package otlp

import (
	"github.com/DataDog/datadog-agent/pkg/otlp/internal/testutil"
)

func getTestPipelineConfig() PipelineConfig {
	return PipelineConfig{
		OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 4317, 4318),
		TracePort:          5003,
		MetricsEnabled:     true,
		TracesEnabled:      true,
		LogsEnabled:        true,
		Metrics:            map[string]interface{}{},
	}
}
