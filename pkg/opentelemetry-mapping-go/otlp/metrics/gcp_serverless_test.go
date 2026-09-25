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
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

type gcpTagSetCall struct {
	metricSuffix string
	tags         []string
}

type mockGCPServerlessConsumer struct {
	mockFullConsumer
	hostCalls   []string
	tagSetCalls []gcpTagSetCall
}

func (c *mockGCPServerlessConsumer) ConsumeHost(host string) {
	c.hostCalls = append(c.hostCalls, host)
}

func (c *mockGCPServerlessConsumer) ConsumeTagSet(metricSuffix string, tags []string) {
	c.tagSetCalls = append(c.tagSetCalls, gcpTagSetCall{metricSuffix: metricSuffix, tags: tags})
}

func newGCPServerlessTranslator(t *testing.T, minimal bool) Provider {
	t.Helper()
	set := componenttest.NewNopTelemetrySettings()
	attributesTranslator, err := attributes.NewTranslator(set)
	require.NoError(t, err)
	options := []TranslatorOption{WithFallbackSourceProvider(testProvider("collector-fallback-host"))}
	if minimal {
		tr, err := NewMinimalTranslator(zap.NewNop(), attributesTranslator, options...)
		require.NoError(t, err)
		return tr
	}
	tr, err := NewDefaultTranslator(set, attributesTranslator, options...)
	require.NoError(t, err)
	return tr
}

func gcpServerlessMetrics(t *testing.T, resourceAttrs map[string]any, apmOnly bool) pmetric.Metrics {
	t.Helper()
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	require.NoError(t, rm.Resource().Attributes().FromRaw(resourceAttrs))
	met := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	if apmOnly {
		met.SetName(keyStatsPayload)
		met.SetEmptySum()
		return md
	}
	met.SetName("some.gauge")
	met.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(1)
	return md
}

func completeGCPServerlessAttributes(platform string) map[string]any {
	return map[string]any{
		"cloud.provider":   "gcp",
		"cloud.platform":   platform,
		"cloud.account.id": "project-1",
		"cloud.region":     "us-central1",
		"faas.name":        "my-service",
		"faas.instance":    "instance-1",
		"faas.version":     "revision-1",
		"host.name":        "resource-host",
	}
}

func TestGCPServerlessTranslation(t *testing.T) {
	tests := []struct {
		name       string
		attributes map[string]any
		wantSuffix string
	}{
		{
			name:       "Cloud Run service",
			attributes: completeGCPServerlessAttributes("gcp_cloud_run"),
			wantSuffix: "cloudrun",
		},
		{
			name:       "Cloud Functions v2",
			attributes: completeGCPServerlessAttributes("gcp_cloud_functions"),
			wantSuffix: "cloudrunfunctions",
		},
		{
			name: "incomplete identity",
			attributes: func() map[string]any {
				attrs := completeGCPServerlessAttributes("gcp_cloud_run")
				delete(attrs, "faas.instance")
				return attrs
			}(),
		},
		{
			name: "job execution present but empty",
			attributes: func() map[string]any {
				attrs := completeGCPServerlessAttributes("gcp_cloud_run")
				attrs["gcp.cloud_run.job.execution"] = ""
				return attrs
			}(),
		},
		{
			name: "job task index present and zero",
			attributes: func() map[string]any {
				attrs := completeGCPServerlessAttributes("gcp_cloud_run")
				attrs["gcp.cloud_run.job.task_index"] = int64(0)
				return attrs
			}(),
		},
	}

	for _, minimal := range []bool{false, true} {
		translatorName := "default"
		if minimal {
			translatorName = "minimal"
		}
		t.Run(translatorName, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					consumer := &mockGCPServerlessConsumer{}
					_, err := newGCPServerlessTranslator(t, minimal).MapMetrics(
						context.Background(), gcpServerlessMetrics(t, tt.attributes, false), consumer, nil,
					)
					require.NoError(t, err)

					require.Len(t, consumer.metrics, 1)
					assert.Empty(t, consumer.metrics[0].host)
					assert.Empty(t, consumer.hostCalls, "GCP serverless resources must not use the configured fallback host")
					if tt.wantSuffix == "" {
						assert.Empty(t, consumer.tagSetCalls, "invalid or out-of-scope resources must not emit a workload running metric")
						return
					}
					require.Len(t, consumer.tagSetCalls, 1)
					assert.Equal(t, tt.wantSuffix, consumer.tagSetCalls[0].metricSuffix)
					assert.ElementsMatch(t, []string{
						"instance:instance-1",
						"service_name:my-service",
						"project_id:project-1",
						"location:us-central1",
					}, consumer.tagSetCalls[0].tags)
					assert.Contains(t, consumer.metrics[0].tags, "revision_name:revision-1")
				})
			}
		})
	}
}

func TestGCPServerlessAPMOnlyDoesNotConsumeSource(t *testing.T) {
	for _, minimal := range []bool{false, true} {
		translatorName := "default"
		if minimal {
			translatorName = "minimal"
		}
		t.Run(translatorName, func(t *testing.T) {
			consumer := &mockGCPServerlessConsumer{}
			_, err := newGCPServerlessTranslator(t, minimal).MapMetrics(
				context.Background(),
				gcpServerlessMetrics(t, completeGCPServerlessAttributes("gcp_cloud_run"), true),
				consumer,
				nil,
			)
			require.NoError(t, err)
			assert.Empty(t, consumer.hostCalls)
			assert.Empty(t, consumer.tagSetCalls)
		})
	}
}
