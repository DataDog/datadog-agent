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

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

func TestHostsConsumed(t *testing.T) {
	tests := []struct {
		name     string
		otlpfile string
		ddogfile string
	}{
		{
			name:     "only stats metrics",
			otlpfile: "test/otlp/hosts/stats.json",
			ddogfile: "test/datadog/hosts/stats.json",
		},
		{
			name:     "stats metrics and other metrics",
			otlpfile: "test/otlp/hosts/stats_other.json",
			ddogfile: "test/datadog/hosts/stats_other.json",
		},
		{
			name:     "only runtime metrics",
			otlpfile: "test/otlp/hosts/runtime.json",
			ddogfile: "test/datadog/hosts/runtime.json",
		},
		{
			name:     "runtime metrics and other metrics",
			otlpfile: "test/otlp/hosts/runtime_other.json",
			ddogfile: "test/datadog/hosts/runtime_other.json",
		},
		{
			name:     "only stats and runtime metrics",
			otlpfile: "test/otlp/hosts/stats_and_runtime.json",
			ddogfile: "test/datadog/hosts/stats_and_runtime.json",
		},
	}

	for _, testinstance := range tests {
		t.Run(testinstance.name, func(t *testing.T) {
			set := componenttest.NewNopTelemetrySettings()
			attributesTranslator, err := attributes.NewTranslator(set)
			require.NoError(t, err)
			translator, err := NewDefaultTranslator(set, attributesTranslator,
				WithOriginProduct(OriginProductDatadogAgent),
			)
			require.NoError(t, err)
			AssertTranslatorMap(t, translator, testinstance.otlpfile, testinstance.ddogfile)
		})
	}
}
