// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package jmxclient

import (
	_ "embed"
)

// defaultJMXMetricsYAML contains the embedded default JMX metrics configuration
// This is loaded when CollectDefaultMetrics is set to true in the check configuration
//
//go:embed default-jmx-metrics.yaml
var defaultJMXMetricsYAML []byte
