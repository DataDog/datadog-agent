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

type mockAzureContainerAppsConsumer struct {
	mockFullConsumer
	hosts       []string
	tagSetCalls []struct {
		metricSuffix string
		tags         []string
	}
}

func (c *mockAzureContainerAppsConsumer) ConsumeHost(host string) {
	c.hosts = append(c.hosts, host)
}

func (c *mockAzureContainerAppsConsumer) ConsumeTagSet(metricSuffix string, tags []string) {
	c.tagSetCalls = append(c.tagSetCalls, struct {
		metricSuffix string
		tags         []string
	}{metricSuffix, tags})
}

func azureContainerAppsMetrics(resourceAttrs map[string]string) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	for key, value := range resourceAttrs {
		rm.Resource().Attributes().PutStr(key, value)
	}
	metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("azure.container_apps.requests")
	metric.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(1)
	return md
}

func azureContainerAppsTranslators(t *testing.T) map[string]Provider {
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

func TestAzureContainerAppsRunningMetricTranslation(t *testing.T) {
	for translatorName, translator := range azureContainerAppsTranslators(t) {
		for _, platform := range []string{"azure.container_apps", "azure_container_apps"} {
			t.Run(translatorName+"/"+platform, func(t *testing.T) {
				consumer := &mockAzureContainerAppsConsumer{}
				_, err := translator.MapMetrics(t.Context(), azureContainerAppsMetrics(map[string]string{
					"cloud.platform":                  platform,
					"cloud.account.id":                "subscription-1",
					"azure.resource_group.name":       "resource-group-1",
					"service.name":                    "container-app-1",
					"azure.container_app.instance.id": "replica-1",
					"host.id":                         "ignored-host",
				}), consumer, nil)
				require.NoError(t, err)

				require.Len(t, consumer.tagSetCalls, 1)
				assert.Equal(t, "azurecontainerapps", consumer.tagSetCalls[0].metricSuffix)
				assert.ElementsMatch(t, []string{
					"subscription_id:subscription-1",
					"resource_group:resource-group-1",
					"name:container-app-1",
					"replica:replica-1",
				}, consumer.tagSetCalls[0].tags)
				assert.Empty(t, consumer.hosts)
			})
		}
	}
}

func TestAzureContainerAppsRunningMetricNotEmittedForIncompleteIdentity(t *testing.T) {
	for translatorName, translator := range azureContainerAppsTranslators(t) {
		t.Run(translatorName, func(t *testing.T) {
			consumer := &mockAzureContainerAppsConsumer{}
			_, err := translator.MapMetrics(t.Context(), azureContainerAppsMetrics(map[string]string{
				"cloud.platform":                  "azure.container_apps",
				"cloud.account.id":                "subscription-1",
				"azure.resource_group.name":       "resource-group-1",
				"service.name":                    "container-app-1",
				"azure.container_app.instance.id": "",
				"host.id":                         "fallback-host",
			}), consumer, nil)
			require.NoError(t, err)

			assert.Empty(t, consumer.tagSetCalls)
			assert.Equal(t, []string{"fallback-host"}, consumer.hosts)
		})
	}
}
