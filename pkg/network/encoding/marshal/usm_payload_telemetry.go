// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

// GetUSMPayloadTelemetry returns a map with all metrics that are meant to be sent as payload telemetry.
//
// In order to emit a new payload metric, make sure to use the option `telemetry.OptPayloadTelemetry`.
// Example:
// myMetric := telemetry.NewMetric("metric_name", telemetry.OptPayloadTelemetry)
//
// Finally, make sure to add an entry to pkg/network/event_common.go for the sake of documentation.
func GetUSMPayloadTelemetry() map[string]int64 {
	all := telemetry.ReportPayloadTelemetry("")
	result := make(map[string]int64, len(all))

	// Only return metrics that are explicitly declared in pkg/network/event_common.go
	for _, metricName := range network.USMPayloadTelemetry {
		v, ok := all[string(metricName)]
		if !ok {
			continue
		}

		result[string(metricName)] = v
	}

	return result
}
