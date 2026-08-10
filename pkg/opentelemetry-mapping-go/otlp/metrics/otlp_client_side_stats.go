// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/sdktracestats"
)

const sdkTraceStatsSource = "otlp-intake-metrics"

func remapSDKTraceMetrics(logger *zap.Logger, consumer Consumer, otlpStatsOut chan<- []byte, host string, rattrs pcommon.Map, metric pmetric.Metric) {
	otlpConsumer, hasOTLPConsumer := consumer.(OTLPStatsConsumer)
	if otlpStatsOut == nil && !hasOTLPConsumer {
		logger.Debug("No APM stats destination configured; dropping SDK trace metric", zap.String(metricName, metric.Name()))
		return
	}

	payload, conversionErrors := sdktracestats.BuildSDKTraceStatsPayload(host, sdkTraceStatsSource, rattrs, metric)
	for _, conversionError := range conversionErrors {
		logger.Debug("Failed to build SDK trace duration stats",
			zap.Int("datapoint_index", conversionError.DataPointIndex),
			zap.Error(conversionError.Err),
		)
	}
	if payload == nil {
		return
	}

	raw, err := sdktracestats.MarshalStatsPayload(payload)
	if err != nil {
		logger.Debug("Failed to marshal SDK trace stats payload", zap.Error(err))
		return
	}
	if otlpStatsOut != nil {
		otlpStatsOut <- raw
		return
	}
	otlpConsumer.ConsumeOTLPStats(raw)
}
