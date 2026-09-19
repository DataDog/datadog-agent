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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
	semconv1_27 "go.opentelemetry.io/otel/semconv/v1.27.0"
	semconv1_43 "go.opentelemetry.io/otel/semconv/v1.43.0"
	conventions "go.opentelemetry.io/otel/semconv/v1.6.1"
	"go.uber.org/zap"
)

// aca-scoped mock consumer tracking ConsumeHost/ConsumeTagSet calls, so tests
// can assert neither host-fallback attribution nor partial-dimension running
// metric emission ever happens for an unidentified Azure Container App.
type mockACAConsumer struct {
	mockFullConsumer
	hostCalls   []string
	tagSetCalls []struct {
		metricSuffix string
		tags         []string
	}
}

func (c *mockACAConsumer) ConsumeHost(host string) {
	c.hostCalls = append(c.hostCalls, host)
}

func (c *mockACAConsumer) ConsumeTagSet(metricSuffix string, tags []string) {
	c.tagSetCalls = append(c.tagSetCalls, struct {
		metricSuffix string
		tags         []string
	}{metricSuffix, tags})
}

func buildACAMetrics(resourceAttrs map[string]string) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	for k, v := range resourceAttrs {
		rm.Resource().Attributes().PutStr(k, v)
	}
	met := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	met.SetName("some.gauge")
	dp := met.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetIntValue(1)
	return md
}

func TestAzureContainerAppsRunningMetricNotEmittedWhenUnidentified(t *testing.T) {
	// Only resource_group is present: not enough to identify the ACA app
	// (name and subscription_id are missing).
	md := buildACAMetrics(map[string]string{
		string(conventions.CloudProviderKey):          conventions.CloudProviderAzure.Value.AsString(),
		string(conventions.CloudPlatformKey):          semconv1_43.CloudPlatformAzureContainerApps.Value.AsString(),
		string(semconv1_43.AzureResourceGroupNameKey): "my-rg",
	})

	tr := newTranslator(t, zap.NewNop())
	consumer := &mockACAConsumer{}
	_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
	require.NoError(t, err)

	assert.Empty(t, consumer.tagSetCalls, "running metric must not be emitted without a full name/resource_group/subscription_id identification")
	assert.Empty(t, consumer.hostCalls, "an unidentified ACA resource must not fall back to Collector/Agent host attribution")
}

func TestAzureContainerAppsRunningMetricEmittedWhenIdentified(t *testing.T) {
	md := buildACAMetrics(map[string]string{
		string(conventions.CloudProviderKey):          conventions.CloudProviderAzure.Value.AsString(),
		string(conventions.CloudPlatformKey):          semconv1_43.CloudPlatformAzureContainerApps.Value.AsString(),
		string(semconv1_27.ServiceNameKey):            "my-app",
		string(semconv1_27.CloudAccountIDKey):         "sub-123",
		string(semconv1_43.AzureResourceGroupNameKey): "my-rg",
	})

	tr := newTranslator(t, zap.NewNop())
	consumer := &mockACAConsumer{}
	_, err := tr.MapMetrics(context.Background(), md, consumer, nil)
	require.NoError(t, err)

	require.Len(t, consumer.tagSetCalls, 1)
	assert.Equal(t, "azurecontainerapps", consumer.tagSetCalls[0].metricSuffix)
	assert.ElementsMatch(t, []string{"name:my-app", "resource_group:my-rg", "subscription_id:sub-123"}, consumer.tagSetCalls[0].tags)
	assert.Empty(t, consumer.hostCalls, "an identified ACA resource must not also be host-attributed")
}
