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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOriginProductDetailFromScopeName(t *testing.T) {
	tests := []struct {
		scopeName string
		expected  OriginProductDetail
	}{
		{
			scopeName: "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/notsupportedreceiver",
			expected:  OriginProductDetailUnknown,
		},
		{
			scopeName: "otelcol/kubeletstatsreceiver",
			expected:  OriginProductDetailUnknown,
		},
		{
			scopeName: "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kubeletstatsreceiver",
			expected:  OriginProductDetailKubeletStatsReceiver,
		},
		{
			scopeName: "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/memory",
			expected:  OriginProductDetailHostMetricsReceiver,
		},
		{
			scopeName: "go.opentelemetry.io/otel/metric/example",
			expected:  OriginProductDetailUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.scopeName, func(t *testing.T) {
			service := originProductDetailFromScopeName(tt.scopeName)
			assert.Equal(t, tt.expected, service)
		})
	}
}

func TestOriginFull(t *testing.T) {
	translator := NewTestTranslator(t, WithOriginProduct(OriginProduct(42)))
	AssertTranslatorMap(t, translator,
		"test/otlp/origin/origin.json",
		"test/datadog/origin/origin.json",
	)
}
