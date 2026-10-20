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
			name: "Cloud Run service resource name fallback",
			attributes: map[string]any{
				"cloud.provider":    "gcp",
				"cloud.platform":    "gcp_cloud_run",
				"cloud.resource_id": "//run.googleapis.com/projects/project-1/locations/us-central1/services/my-service",
				"faas.instance":     "instance-1",
				"faas.version":      "revision-1",
			},
			wantSuffix: "cloudrun",
		},
		{
			name: "Cloud Functions resource name fallback",
			attributes: map[string]any{
				"cloud.provider":    "gcp",
				"cloud.platform":    "gcp_cloud_functions",
				"cloud.resource_id": "//cloudfunctions.googleapis.com/projects/project-1/locations/us-central1/functions/my-service",
				"faas.instance":     "instance-1",
				"faas.version":      "revision-1",
			},
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

func TestCompleteServerlessSourcesWithoutRunningMetricsDoNotFallbackToHost(t *testing.T) {
	tests := []struct {
		name       string
		attributes map[string]any
	}{
		{
			name: "AWS Lambda",
			attributes: map[string]any{
				"cloud.provider": "aws",
				"cloud.platform": "aws_lambda",
				"faas.id":        "arn:aws:lambda:us-east-1:123456789012:function:orders",
			},
		},
		{
			name: "Cloud Run Jobs",
			attributes: map[string]any{
				"cloud.provider":              "gcp",
				"cloud.platform":              "gcp_cloud_run",
				"cloud.resource_id":           "//run.googleapis.com/projects/project-1/locations/us-central1/jobs/daily-job",
				"faas.instance":               "instance-1",
				"gcp.cloud_run.job.execution": "daily-job-abc",
			},
		},
		{
			name: "GKE Autopilot",
			attributes: map[string]any{
				"cloud.provider":   "gcp",
				"cloud.platform":   "gcp_kubernetes_engine",
				"cloud.account.id": "project-1",
				"k8s.cluster.name": "autopilot-cluster",
				"host.name":        "gk3-autopilot-cluster-pool-abc",
			},
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
					assert.Empty(t, consumer.hostCalls)
					assert.Empty(t, consumer.tagSetCalls)
				})
			}
		})
	}
}

func TestServerlessSourceIdentityDistinguishesMetricSeries(t *testing.T) {
	tests := []struct {
		name       string
		resources  []map[string]any
		wantTags   [][]string
		uniqueTags []string
	}{
		{
			name: "AWS Lambda",
			resources: []map[string]any{
				{
					"cloud.provider": "aws",
					"cloud.platform": "aws_lambda",
					"faas.id":        "arn:aws:lambda:us-east-1:123456789012:function:orders",
				},
				{
					"cloud.provider": "aws",
					"cloud.platform": "aws_lambda",
					"faas.id":        "arn:aws:lambda:us-east-1:123456789012:function:payments",
				},
			},
			wantTags: [][]string{
				{
					"function_arn:arn:aws:lambda:us-east-1:123456789012:function:orders",
					"function_name:orders",
					"region:us-east-1",
					"aws_account:123456789012",
				},
				{
					"function_arn:arn:aws:lambda:us-east-1:123456789012:function:payments",
					"function_name:payments",
					"region:us-east-1",
					"aws_account:123456789012",
				},
			},
			uniqueTags: []string{"function_name:orders", "function_name:payments"},
		},
		{
			name: "Cloud Run Jobs",
			resources: []map[string]any{
				{
					"cloud.provider":              "gcp",
					"cloud.platform":              "gcp_cloud_run",
					"cloud.resource_id":           "//run.googleapis.com/projects/project-1/locations/us-central1/jobs/daily-job",
					"faas.instance":               "instance-1",
					"gcp.cloud_run.job.execution": "daily-job-abc",
				},
				{
					"cloud.provider":              "gcp",
					"cloud.platform":              "gcp_cloud_run",
					"cloud.resource_id":           "//run.googleapis.com/projects/project-1/locations/us-central1/jobs/hourly-job",
					"faas.instance":               "instance-2",
					"gcp.cloud_run.job.execution": "hourly-job-def",
				},
			},
			wantTags: [][]string{
				{
					"project_id:project-1",
					"location:us-central1",
					"job_name:daily-job",
					"instance:instance-1",
				},
				{
					"project_id:project-1",
					"location:us-central1",
					"job_name:hourly-job",
					"instance:instance-2",
				},
			},
			uniqueTags: []string{"job_name:daily-job", "job_name:hourly-job"},
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
					translator := newGCPServerlessTranslator(t, minimal)
					for _, resourceAttrs := range tt.resources {
						_, err := translator.MapMetrics(
							context.Background(), gcpServerlessMetrics(t, resourceAttrs, false), consumer, nil,
						)
						require.NoError(t, err)
					}

					require.Len(t, consumer.metrics, len(tt.resources))
					assert.Empty(t, consumer.hostCalls)
					assert.Empty(t, consumer.tagSetCalls)
					for i, wantTags := range tt.wantTags {
						assert.Empty(t, consumer.metrics[i].host)
						for _, wantTag := range wantTags {
							assert.Contains(t, consumer.metrics[i].tags, wantTag)
						}
						assert.Contains(t, consumer.metrics[i].tags, tt.uniqueTags[i])
						assert.NotContains(t, consumer.metrics[1-i].tags, tt.uniqueTags[i])
					}
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
