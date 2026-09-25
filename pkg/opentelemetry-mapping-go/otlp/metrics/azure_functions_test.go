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
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

type mockAzureFunctionsConsumer struct {
	mockFullConsumer
	hosts       []string
	tagSetCalls []struct {
		metricSuffix string
		tags         []string
	}
}

func (c *mockAzureFunctionsConsumer) ConsumeHost(host string) {
	c.hosts = append(c.hosts, host)
}

func (c *mockAzureFunctionsConsumer) ConsumeTagSet(metricSuffix string, tags []string) {
	c.tagSetCalls = append(c.tagSetCalls, struct {
		metricSuffix string
		tags         []string
	}{metricSuffix, tags})
}

func azureFunctionsMetrics(resourceAttrs map[string]string) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	for key, value := range resourceAttrs {
		rm.Resource().Attributes().PutStr(key, value)
	}
	metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("azure.functions.requests")
	metric.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(1)
	return md
}

func azureFunctionsTranslators(t *testing.T) map[string]Provider {
	t.Helper()
	settings := componenttest.NewNopTelemetrySettings()
	attributesTranslator, err := attributes.NewTranslator(settings)
	require.NoError(t, err)
	minimal, err := NewMinimalTranslator(zap.NewNop(), attributesTranslator)
	require.NoError(t, err)
	return map[string]Provider{
		"full":    NewTestTranslator(t),
		"minimal": minimal,
	}
}

func TestAzureFunctionsRunningMetricTranslation(t *testing.T) {
	for translatorName, translator := range azureFunctionsTranslators(t) {
		for _, platform := range []string{"azure.functions", "azure_functions"} {
			t.Run(translatorName+"/"+platform, func(t *testing.T) {
				consumer := &mockAzureFunctionsConsumer{}
				_, err := translator.MapMetrics(t.Context(), azureFunctionsMetrics(map[string]string{
					"cloud.platform":            platform,
					"cloud.account.id":          "subscription-1",
					"azure.resource_group.name": "resource-group-1",
					"service.name":              "function-app-1",
					"faas.instance":             "instance-1",
					"host.id":                   "ignored-host",
				}), consumer, nil)
				require.NoError(t, err)

				require.Len(t, consumer.tagSetCalls, 1)
				assert.Equal(t, "azurefunctions", consumer.tagSetCalls[0].metricSuffix)
				assert.ElementsMatch(t, []string{
					"subscription_id:subscription-1",
					"resource_group:resource-group-1",
					"name:function-app-1",
					"instance:instance-1",
				}, consumer.tagSetCalls[0].tags)
				assert.Empty(t, consumer.hosts)
			})
		}
	}
}

func TestAzureFunctionsRunningMetricNotEmittedForIncompleteIdentity(t *testing.T) {
	for translatorName, translator := range azureFunctionsTranslators(t) {
		t.Run(translatorName, func(t *testing.T) {
			consumer := &mockAzureFunctionsConsumer{}
			_, err := translator.MapMetrics(t.Context(), azureFunctionsMetrics(map[string]string{
				"cloud.platform":            "azure.functions",
				"cloud.account.id":          "subscription-1",
				"azure.resource_group.name": "resource-group-1",
				"service.name":              "function-app-1",
				"faas.instance":             "",
				"host.id":                   "fallback-host",
			}), consumer, nil)
			require.NoError(t, err)

			assert.Empty(t, consumer.tagSetCalls)
			assert.Equal(t, []string{"fallback-host"}, consumer.hosts)
		})
	}
}
