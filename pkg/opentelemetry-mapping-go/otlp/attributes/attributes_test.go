// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package attributes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	semconv1_12 "go.opentelemetry.io/otel/semconv/v1.12.0"
	semconv1_17 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv127 "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/otel/semconv/v1.6.1"
)

func TestTagsFromAttributes(t *testing.T) {
	attributeMap := map[string]interface{}{
		string(semconv127.ProcessExecutableNameKey):     "otelcol",
		string(semconv127.ProcessExecutablePathKey):     "/usr/bin/cmd/otelcol",
		string(semconv127.ProcessCommandKey):            "cmd/otelcol",
		string(semconv127.ProcessCommandLineKey):        "cmd/otelcol --config=\"/path/to/config.yaml\"",
		string(semconv127.ProcessPIDKey):                1,
		string(semconv127.ProcessOwnerKey):              "root",
		string(semconv127.OSTypeKey):                    "linux",
		string(semconv127.K8SDaemonSetNameKey):          "daemon_set_name",
		string(semconv127.AWSECSClusterARNKey):          "cluster_arn",
		string(semconv127.ContainerRuntimeKey):          "cro",
		"tags.datadoghq.com/service":                    "service_name",
		string(semconv127.DeploymentEnvironmentNameKey): "prod",
		string(semconv127.ContainerNameKey):             "custom",
		"datadog.container.tag.custom.team":             "otel",
		"kube_cronjob":                                  "cron",
	}
	attrs := pcommon.NewMap()
	attrs.FromRaw(attributeMap)

	assert.ElementsMatch(t, []string{
		fmt.Sprintf("%s:%s", string(semconv127.ProcessExecutableNameKey), "otelcol"),
		fmt.Sprintf("%s:%s", string(semconv127.OSTypeKey), "linux"),
		fmt.Sprintf("%s:%s", "kube_daemon_set", "daemon_set_name"),
		fmt.Sprintf("%s:%s", "ecs_cluster_name", "cluster_arn"),
		fmt.Sprintf("%s:%s", "service", "service_name"),
		fmt.Sprintf("%s:%s", "runtime", "cro"),
		fmt.Sprintf("%s:%s", "env", "prod"),
		fmt.Sprintf("%s:%s", "container_name", "custom"),
		fmt.Sprintf("%s:%s", "custom.team", "otel"),
		fmt.Sprintf("%s:%s", "kube_cronjob", "cron"),
	}, TagsFromAttributes(attrs))
}

func TestNewDeploymentEnvironmentNameConvention(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("deployment.environment.name", "staging")

	assert.Equal(t, []string{"env:staging"}, TagsFromAttributes(attrs))
}

func TestTagsFromAttributesEmpty(t *testing.T) {
	attrs := pcommon.NewMap()

	assert.Equal(t, []string{}, TagsFromAttributes(attrs))
}

func TestContainerTagFromResourceAttributes(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		attributes := pcommon.NewMap()
		err := attributes.FromRaw(map[string]interface{}{
			string(semconv127.ContainerNameKey):         "sample_app",
			string(conventions.ContainerImageTagKey):    "sample_app_image_tag",
			string(semconv127.ContainerRuntimeKey):      "cro",
			string(semconv127.K8SContainerNameKey):      "kube_sample_app",
			string(semconv127.K8SReplicaSetNameKey):     "sample_replica_set",
			string(semconv127.K8SDaemonSetNameKey):      "sample_daemonset_name",
			string(semconv127.K8SPodNameKey):            "sample_pod_name",
			string(semconv127.CloudProviderKey):         "sample_cloud_provider",
			string(semconv127.CloudRegionKey):           "sample_region",
			string(semconv127.CloudAvailabilityZoneKey): "sample_zone",
			string(semconv127.AWSECSTaskFamilyKey):      "sample_task_family",
			string(semconv127.AWSECSClusterARNKey):      "sample_ecs_cluster_name",
			string(semconv127.AWSECSContainerARNKey):    "sample_ecs_container_name",
			"datadog.container.tag.custom.team":         "otel",
		})
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{
			"container_name":      "sample_app",
			"image_tag":           "sample_app_image_tag",
			"runtime":             "cro",
			"kube_container_name": "kube_sample_app",
			"kube_replica_set":    "sample_replica_set",
			"kube_daemon_set":     "sample_daemonset_name",
			"pod_name":            "sample_pod_name",
			"cloud_provider":      "sample_cloud_provider",
			"region":              "sample_region",
			"zone":                "sample_zone",
			"task_family":         "sample_task_family",
			"ecs_cluster_name":    "sample_ecs_cluster_name",
			"ecs_container_name":  "sample_ecs_container_name",
			"custom.team":         "otel",
		}, ContainerTagsFromResourceAttributes(attributes))
		fmt.Println(ContainerTagsFromResourceAttributes(attributes))
	})

	t.Run("conventions vs custom", func(t *testing.T) {
		attributes := pcommon.NewMap()
		err := attributes.FromRaw(map[string]interface{}{
			string(semconv127.ContainerNameKey):    "ok",
			"datadog.container.tag.container_name": "nok",
		})
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{
			"container_name": "ok",
		}, ContainerTagsFromResourceAttributes(attributes))
	})

	t.Run("invalid", func(t *testing.T) {
		attributes := pcommon.NewMap()
		err := attributes.FromRaw(map[string]interface{}{
			"empty_string_val": "",
			"":                 "empty_string_key",
			"custom_tag":       "example_custom_tag",
		})
		assert.NoError(t, err)
		slice := attributes.PutEmptySlice("datadog.container.tag.slice")
		slice.AppendEmpty().SetStr("value1")
		slice.AppendEmpty().SetStr("value2")
		assert.Equal(t, map[string]string{}, ContainerTagsFromResourceAttributes(attributes))
	})

	t.Run("empty", func(t *testing.T) {
		assert.Empty(t, ContainerTagsFromResourceAttributes(pcommon.NewMap()))
	})
}

func TestConsumeContainerTagsFromResource(t *testing.T) {
	res := pcommon.NewResource()
	res.SetDroppedAttributesCount(42)
	attrs := map[string]any{
		// OTel conventions
		string(semconv127.ContainerIDKey):        "test_container_id_otel",
		string(semconv127.ContainerNameKey):      "test_container_name_otel",
		string(semconv127.ContainerImageNameKey): "test_image_name_otel",
		string(conventions.ContainerImageTagKey): "test_image_tag_otel",

		// Custom tags
		"datadog.container.tag.container_id":        "test_container_id_custom",
		"datadog.container.tag.container_name":      "test_container_name_custom",
		"datadog.container.tag.runtime":             "test_runtime_custom",
		"datadog.container.tag.kube_container_name": "test_kube_container_name_custom",

		// Datadog conventions
		"container_id":      "test_container_id_dd",
		"image_name":        "test_image_name_dd",
		"runtime":           "test_runtime_dd",
		"kube_cluster_name": "test_kube_cluster_name_dd",

		// Non-container resource attribute
		string(semconv127.HostNameKey): "test_host_name",
	}
	err := res.Attributes().FromRaw(attrs)
	require.NoError(t, err)
	containerTags, newRes := ConsumeContainerTagsFromResource(res)
	assert.Equal(t, map[string]string{
		"container_id":        "test_container_id_otel",          // OTel > Custom, Datadog
		"container_name":      "test_container_name_otel",        // OTel > Custom
		"image_name":          "test_image_name_otel",            // OTel > Datadog
		"image_tag":           "test_image_tag_otel",             // OTel only
		"runtime":             "test_runtime_custom",             // Custom > Datadog
		"kube_container_name": "test_kube_container_name_custom", // Custom only
		"kube_cluster_name":   "test_kube_cluster_name_dd",       // Datadog only
	}, containerTags)
	assert.Equal(t, uint32(42), newRes.DroppedAttributesCount())
	newAttrs := newRes.Attributes().AsRaw()
	assert.NotEqual(t, attrs, newAttrs)
	assert.Equal(t, map[string]any{
		// Only keep OTel conventions to be used as span attributes
		string(semconv127.ContainerIDKey):        "test_container_id_otel",
		string(semconv127.ContainerNameKey):      "test_container_name_otel",
		string(semconv127.ContainerImageNameKey): "test_image_name_otel",
		string(conventions.ContainerImageTagKey): "test_image_tag_otel",

		// Of course, keep non-container resource attributes
		string(semconv127.HostNameKey): "test_host_name",
	}, newAttrs)
}

func TestContainerTagFromAttributes(t *testing.T) {
	attributeMap := map[string]string{
		string(semconv127.ContainerNameKey):         "sample_app",
		string(conventions.ContainerImageTagKey):    "sample_app_image_tag",
		string(semconv127.ContainerRuntimeKey):      "cro",
		string(semconv127.K8SContainerNameKey):      "kube_sample_app",
		string(semconv127.K8SReplicaSetNameKey):     "sample_replica_set",
		string(semconv127.K8SDaemonSetNameKey):      "sample_daemonset_name",
		string(semconv127.K8SPodNameKey):            "sample_pod_name",
		string(semconv127.CloudProviderKey):         "sample_cloud_provider",
		string(semconv127.CloudRegionKey):           "sample_region",
		string(semconv127.CloudAvailabilityZoneKey): "sample_zone",
		string(semconv127.AWSECSTaskFamilyKey):      "sample_task_family",
		string(semconv127.AWSECSClusterARNKey):      "sample_ecs_cluster_name",
		string(semconv127.AWSECSContainerARNKey):    "sample_ecs_container_name",
		"custom_tag":                                "example_custom_tag",
		"":                                          "empty_string_key",
		"empty_string_val":                          "",
	}

	assert.Equal(t, map[string]string{
		"container_name":      "sample_app",
		"image_tag":           "sample_app_image_tag",
		"runtime":             "cro",
		"kube_container_name": "kube_sample_app",
		"kube_replica_set":    "sample_replica_set",
		"kube_daemon_set":     "sample_daemonset_name",
		"pod_name":            "sample_pod_name",
		"cloud_provider":      "sample_cloud_provider",
		"region":              "sample_region",
		"zone":                "sample_zone",
		"task_family":         "sample_task_family",
		"ecs_cluster_name":    "sample_ecs_cluster_name",
		"ecs_container_name":  "sample_ecs_container_name",
	}, ContainerTagFromAttributes(attributeMap))
}

func TestContainerTagFromAttributesEmpty(t *testing.T) {
	assert.Empty(t, ContainerTagFromAttributes(map[string]string{}))
}

func TestOriginIDFromAttributes(t *testing.T) {
	tests := []struct {
		name     string
		attrs    pcommon.Map
		originID string
	}{
		{
			name: "pod UID and container ID",
			attrs: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.FromRaw(map[string]interface{}{
					string(conventions.ContainerIDKey): "container_id_goes_here",
					string(conventions.K8SPodUIDKey):   "k8s_pod_uid_goes_here",
				})
				return attributes
			}(),
			originID: "container_id://container_id_goes_here",
		},
		{
			name: "only container ID",
			attrs: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.FromRaw(map[string]interface{}{
					string(conventions.ContainerIDKey): "container_id_goes_here",
				})
				return attributes
			}(),
			originID: "container_id://container_id_goes_here",
		},
		{
			name: "only pod UID",
			attrs: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.FromRaw(map[string]interface{}{
					string(semconv127.K8SPodUIDKey): "k8s_pod_uid_goes_here",
				})
				return attributes
			}(),
			originID: "kubernetes_pod_uid://k8s_pod_uid_goes_here",
		},
		{
			name:  "none",
			attrs: pcommon.NewMap(),
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			originID := OriginIDFromAttributes(testInstance.attrs)
			assert.Equal(t, testInstance.originID, originID)
		})
	}
}

func TestGetOperationName(t *testing.T) {
	tests := []struct {
		name     string
		attrs    map[string]string
		spanKind ptrace.SpanKind
		expected string
	}{
		{
			name:     "operation.name",
			spanKind: ptrace.SpanKindInternal,
			attrs: map[string]string{
				"operation.name": "test-operation-name",
			},
			expected: "test-operation-name",
		},
		{
			name:     "http.server.request",
			spanKind: ptrace.SpanKindServer,
			attrs: map[string]string{
				string(semconv127.HTTPRequestMethodKey): "GET",
			},
			expected: "http.server.request",
		},
		{
			name:     "http.client.request",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv127.HTTPRequestMethodKey): "GET",
			},
			expected: "http.client.request",
		},
		{
			name:     "db.query",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv1_12.DBSystemKey): "redis",
			},
			expected: "redis.query",
		},
		{
			name:     "messaging.operation",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv1_12.MessagingSystemKey):    "kafka",
				string(semconv1_12.MessagingOperationKey): "send",
			},
			expected: "kafka.send",
		},
		{
			name:     "aws.client.request",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv1_12.RPCSystemKey): "aws-api",
			},
			expected: "aws.client.request",
		},
		{
			name:     "aws.service.request",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv1_12.RPCSystemKey):  "aws-api",
				string(semconv1_12.RPCServiceKey): "lambda",
			},
			expected: "aws.lambda.request",
		},
		{
			name:     "grpc.client.request",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv1_12.RPCSystemKey): "grpc",
			},
			expected: "grpc.client.request",
		},
		{
			name:     "grpc.server.request",
			spanKind: ptrace.SpanKindServer,
			attrs: map[string]string{
				string(semconv1_12.RPCSystemKey): "grpc",
			},
			expected: "grpc.server.request",
		},
		{
			name:     "faas.client.request",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv1_12.FaaSInvokedProviderKey): "gcp",
				string(semconv1_12.FaaSInvokedNameKey):     "foo",
			},
			expected: "gcp.foo.invoke",
		},
		{
			name:     "faas.server.request",
			spanKind: ptrace.SpanKindServer,
			attrs: map[string]string{
				string(semconv1_12.FaaSTriggerKey): "timer",
			},
			expected: "timer.invoke",
		},
		{
			name:     "graphql.server.request",
			spanKind: ptrace.SpanKindServer,
			attrs: map[string]string{
				"graphql.operation.type": "query",
			},
			expected: "graphql.server.request",
		},
		{
			name:     "network.protocol.name - server",
			spanKind: ptrace.SpanKindServer,
			attrs: map[string]string{
				"network.protocol.name": "http",
			},
			expected: "http.server.request",
		},
		{
			name:     "network.protocol.name - client",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				"network.protocol.name": "http",
			},
			expected: "http.client.request",
		},
		{
			name:     "default - specified span kind",
			spanKind: ptrace.SpanKindProducer,
			expected: "Producer",
		},
		{
			name:     "default",
			spanKind: ptrace.SpanKindInternal,
			expected: "Internal",
		},
	}
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			for k, v := range testInstance.attrs {
				attrs.PutStr(k, v)
			}
			operationName := GetOperationName(attrs, testInstance.spanKind)
			assert.Equal(t, testInstance.expected, operationName)
		})
	}
}

func TestGetResourceName(t *testing.T) {
	tests := []struct {
		name     string
		spanKind ptrace.SpanKind
		attrs    map[string]string
		fallback string
		expected string
	}{
		{
			name:     "resource.name",
			spanKind: ptrace.SpanKindServer,
			attrs: map[string]string{
				"resource.name": "test-resource-name",
			},
			expected: "test-resource-name",
		},
		{
			name:     "http - use HTTP route for server spans",
			spanKind: ptrace.SpanKindServer,
			attrs: map[string]string{
				string(semconv127.HTTPRequestMethodKey): "GET",
				string(semconv127.HTTPRouteKey):         "/",
			},
			expected: "GET /",
		},
		{
			name:     "http - use HTTP template for client spans",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv127.HTTPRequestMethodKey): "GET",
				string(semconv127.URLTemplateKey):       "http://example.com/path?query=value",
			},
			expected: "GET http://example.com/path?query=value",
		},
		{
			name:     "messaging operation",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv1_12.MessagingOperationKey):   "send",
				string(semconv1_12.MessagingDestinationKey): "example.com",
			},
			expected: "send example.com",
		}, {
			name:     "rpc",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv1_12.RPCMethodKey):  "get",
				string(semconv1_12.RPCServiceKey): "example.com",
			},
			expected: "get example.com",
		},
		{
			name:     "graphql",
			spanKind: ptrace.SpanKindServer,
			attrs: map[string]string{
				string(semconv1_17.GraphqlOperationTypeKey): "query",
				string(semconv1_17.GraphqlOperationNameKey): "myQuery",
			},
			expected: "query myQuery",
		},
		{
			name:     "db statement (old convention)",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv1_12.DBSystemKey):    "redis",
				string(semconv1_12.DBStatementKey): "SELECT * FROM resource",
			},
			expected: "SELECT * FROM resource",
		},
		{
			name:     "db query",
			spanKind: ptrace.SpanKindClient,
			attrs: map[string]string{
				string(semconv1_12.DBSystemKey):   "redis",
				string(semconv127.DBQueryTextKey): "SELECT * FROM resource",
			},
			expected: "SELECT * FROM resource",
		},
		{
			name:     "fallback name",
			spanKind: ptrace.SpanKindClient,
			fallback: "fallback-name",
			expected: "fallback-name",
		},
	}
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			for k, v := range testInstance.attrs {
				attrs.PutStr(k, v)
			}
			resourceName := GetResourceName(attrs, testInstance.spanKind, testInstance.fallback)
			assert.Equal(t, testInstance.expected, resourceName)
		})
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name     string
		attrs    map[string]string
		expected string
	}{
		{
			name: "deployment.environment.name",
			attrs: map[string]string{
				string(semconv127.DeploymentEnvironmentNameKey): "prod",
			},
			expected: "prod",
		},
		{
			name: "deployment.environment",
			attrs: map[string]string{
				string(semconv1_17.DeploymentEnvironmentKey): "prod",
			},
			expected: "prod",
		}, {
			name: "newer convention takes precedence",
			attrs: map[string]string{
				string(semconv127.DeploymentEnvironmentNameKey): "prod-127",
				string(semconv1_17.DeploymentEnvironmentKey):    "prod-117",
			},
			expected: "prod-127",
		},
		{
			name:     "normalization",
			attrs:    map[string]string{string(semconv127.DeploymentEnvironmentNameKey): "  ENV "},
			expected: "_env",
		},
		{
			name:     "default",
			expected: DefaultEnvName,
		},
		{
			name: "default if explicitly empty",
			attrs: map[string]string{
				string(semconv127.DeploymentEnvironmentNameKey): "",
			},
			expected: DefaultEnvName,
		},
	}
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			for k, v := range testInstance.attrs {
				attrs.PutStr(k, v)
			}
			env := GetEnv(attrs)
			assert.Equal(t, testInstance.expected, env)
		})
	}
}

func TestGetSpanType(t *testing.T) {
	tests := []struct {
		name     string
		spanKind ptrace.SpanKind
		attrs    map[string]string
		expected string
	}{
		{
			name:     "explicit span.type",
			spanKind: ptrace.SpanKindInternal,
			attrs:    map[string]string{"span.type": "test"},
			expected: "test",
		},
		{
			name:     "default server span",
			spanKind: ptrace.SpanKindServer,
			expected: "web",
		},
		{
			name:     "default (no server/client span kind)",
			spanKind: ptrace.SpanKindInternal,
			expected: "custom",
		},
		{
			name:     "client span, no db.system",
			spanKind: ptrace.SpanKindClient,
			attrs:    map[string]string{},
			expected: "http",
		},
		{
			name:     "client span, db.system",
			spanKind: ptrace.SpanKindClient,
			attrs:    map[string]string{string(conventions.DBSystemKey): "redis"},
			expected: "redis",
		},
		{
			name:     "client span, db.system not in known types",
			spanKind: ptrace.SpanKindClient,
			attrs:    map[string]string{string(conventions.DBSystemKey): "other"},
			expected: "db",
		},
	}
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			for k, v := range testInstance.attrs {
				attrs.PutStr(k, v)
			}
			actual := GetSpanType(testInstance.spanKind, attrs)
			assert.Equal(t, testInstance.expected, actual)
		})
	}
}
func TestGetVersion(t *testing.T) {
	tests := []struct {
		name     string
		attrs    map[string]string
		expected string
	}{
		{
			name:     "service.version",
			attrs:    map[string]string{string(semconv127.ServiceVersionKey): "1.0.0"},
			expected: "1.0.0",
		},
		{
			name:     "default",
			expected: "",
		},
	}
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			for k, v := range testInstance.attrs {
				attrs.PutStr(k, v)
			}
			actual := GetVersion(attrs)
			assert.Equal(t, testInstance.expected, actual)
		})
	}
}

func TestGetStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		attrs    map[string]int
		expected uint32
	}{
		{
			name:     "http.status_code",
			attrs:    map[string]int{string(semconv1_17.HTTPStatusCodeKey): 200},
			expected: 200,
		},
		{
			name:     "http.response.status_code",
			attrs:    map[string]int{string(semconv127.HTTPResponseStatusCodeKey): 200},
			expected: 200,
		},
		{
			name:     "default",
			expected: 0,
		},
	}
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			for k, v := range testInstance.attrs {
				attrs.PutInt(k, int64(v))
			}
			actual := GetStatusCode(attrs)
			assert.Equal(t, testInstance.expected, actual)
		})
	}
}

func TestGetContainerID(t *testing.T) {
	tests := []struct {
		name     string
		attrs    map[string]string
		expected string
	}{
		{
			name:     "container.id",
			attrs:    map[string]string{string(semconv127.ContainerIDKey): "test-container-id"},
			expected: "test-container-id",
		},
		{
			name:     "default",
			expected: "",
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			for k, v := range testInstance.attrs {
				attrs.PutStr(k, v)
			}
			actual := GetContainerID(attrs)
			assert.Equal(t, testInstance.expected, actual)
		})
	}
}

func TestGetSpecifiedKeysFromOTelAttributes(t *testing.T) {
	tests := []struct {
		name         string
		sattrs       map[string]string
		rattrs       map[string]string
		keysToReturn map[string]struct{}
		expected     []string
	}{
		{
			name:         "explicit keys",
			sattrs:       map[string]string{"test-key": "test-value"},
			rattrs:       map[string]string{},
			keysToReturn: map[string]struct{}{"test-key": {}},
			expected:     []string{"test-key:test-value"},
		},
		{
			name:         "key not specified",
			sattrs:       map[string]string{"test-key": "test-value"},
			rattrs:       map[string]string{},
			keysToReturn: map[string]struct{}{},
			expected:     []string{},
		},
		{
			name:         "key specified is not present keysToReturn",
			sattrs:       map[string]string{"other-key": "test-value"},
			rattrs:       map[string]string{},
			keysToReturn: map[string]struct{}{"test-key": {}},
			expected:     []string{},
		},
		{
			name:         "can search in resource attributes",
			rattrs:       map[string]string{"test-key": "resource-value"},
			keysToReturn: map[string]struct{}{"test-key": {}},
			expected:     []string{"test-key:resource-value"},
		},
		{
			name:         "signal takes precedence over resource",
			sattrs:       map[string]string{"test-key": "signal-value", "only-in-signal": "signal-value"},
			rattrs:       map[string]string{"test-key": "resource-value", "only-in-resource": "resource-value"},
			keysToReturn: map[string]struct{}{"test-key": {}, "only-in-signal": {}, "only-in-resource": {}},
			expected:     []string{"test-key:signal-value", "only-in-signal:signal-value", "only-in-resource:resource-value"},
		},
	}
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			signalAttrs := pcommon.NewMap()
			for k, v := range testInstance.sattrs {
				signalAttrs.PutStr(k, v)
			}
			resourceAttrs := pcommon.NewMap()
			for k, v := range testInstance.rattrs {
				resourceAttrs.PutStr(k, v)
			}
			got := GetSpecifiedKeysFromOTelAttributes(signalAttrs, resourceAttrs, testInstance.keysToReturn)
			assert.ElementsMatch(t, testInstance.expected, got)
		})
	}
}

func TestGetHost(t *testing.T) {
	for _, tt := range []struct {
		name         string
		attrs        map[string]string
		fallbackHost string
		expected     string
	}{
		{
			name:     "datadog.host.name",
			attrs:    map[string]string{"datadog.host.name": "test-host"},
			expected: "test-host",
		},
		{
			name:     "_dd.hostname",
			attrs:    map[string]string{"_dd.hostname": "test-host"},
			expected: "test-host",
		},
		{
			name:         "fallback hostname",
			fallbackHost: "test-host",
			expected:     "test-host",
		},
		{
			name:     "reject invalid host",
			attrs:    map[string]string{"datadog.host.name": "localhost"},
			expected: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			for k, v := range tt.attrs {
				attrs.PutStr(k, v)
			}
			actual := GetHost(attrs, tt.fallbackHost)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
