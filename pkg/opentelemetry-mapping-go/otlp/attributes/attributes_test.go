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
	"strings"
	"testing"

	semconv1_17 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv1_27 "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/otel/semconv/v1.6.1"

	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

func TestTagsFromAttributes(t *testing.T) {
	attributeMap := map[string]interface{}{
		string(semconv1_27.ProcessExecutableNameKey):     "otelcol",
		string(semconv1_27.ProcessExecutablePathKey):     "/usr/bin/cmd/otelcol",
		string(semconv1_27.ProcessCommandKey):            "cmd/otelcol",
		string(semconv1_27.ProcessCommandLineKey):        "cmd/otelcol --config=\"/path/to/config.yaml\"",
		string(semconv1_27.ProcessPIDKey):                1,
		string(semconv1_27.ProcessOwnerKey):              "root",
		string(semconv1_27.OSTypeKey):                    "linux",
		string(semconv1_27.K8SDaemonSetNameKey):          "daemon_set_name",
		string(semconv1_27.AWSECSClusterARNKey):          "cluster_arn",
		string(semconv1_27.ContainerRuntimeKey):          "cro",
		"tags.datadoghq.com/service":                     "service_name",
		string(semconv1_27.DeploymentEnvironmentNameKey): "prod",
		string(semconv1_27.ContainerNameKey):             "custom",
		"datadog.container.tag.custom.team":              "otel",
		"kube_cronjob":                                   "cron",
	}
	attrs := pcommon.NewMap()
	attrs.FromRaw(attributeMap)

	assert.ElementsMatch(t, []string{
		fmt.Sprintf("%s:%s", string(semconv1_27.ProcessExecutableNameKey), "otelcol"),
		fmt.Sprintf("%s:%s", string(semconv1_27.OSTypeKey), "linux"),
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
			string(semconv1_27.ContainerNameKey):         "sample_app",
			string(conventions.ContainerImageTagKey):     "sample_app_image_tag",
			string(semconv1_27.ContainerRuntimeKey):      "cro",
			string(semconv1_27.K8SContainerNameKey):      "kube_sample_app",
			string(semconv1_27.K8SReplicaSetNameKey):     "sample_replica_set",
			string(semconv1_27.K8SDaemonSetNameKey):      "sample_daemonset_name",
			string(semconv1_27.K8SPodNameKey):            "sample_pod_name",
			string(semconv1_27.CloudProviderKey):         "sample_cloud_provider",
			string(semconv1_27.CloudRegionKey):           "sample_region",
			string(semconv1_27.CloudAvailabilityZoneKey): "sample_zone",
			string(semconv1_27.AWSECSTaskFamilyKey):      "sample_task_family",
			string(semconv1_27.AWSECSClusterARNKey):      "sample_ecs_cluster_name",
			string(semconv1_27.AWSECSContainerARNKey):    "sample_ecs_container_name",
			"datadog.container.tag.custom.team":          "otel",
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
			string(semconv1_27.ContainerNameKey):   "ok",
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

func TestContainerTagFromAttributes(t *testing.T) {
	attributeMap := map[string]string{
		string(semconv1_27.ContainerNameKey):         "sample_app",
		string(conventions.ContainerImageTagKey):     "sample_app_image_tag",
		string(semconv1_27.ContainerRuntimeKey):      "cro",
		string(semconv1_27.K8SContainerNameKey):      "kube_sample_app",
		string(semconv1_27.K8SReplicaSetNameKey):     "sample_replica_set",
		string(semconv1_27.K8SDaemonSetNameKey):      "sample_daemonset_name",
		string(semconv1_27.K8SPodNameKey):            "sample_pod_name",
		string(semconv1_27.CloudProviderKey):         "sample_cloud_provider",
		string(semconv1_27.CloudRegionKey):           "sample_region",
		string(semconv1_27.CloudAvailabilityZoneKey): "sample_zone",
		string(semconv1_27.AWSECSTaskFamilyKey):      "sample_task_family",
		string(semconv1_27.AWSECSClusterARNKey):      "sample_ecs_cluster_name",
		string(semconv1_27.AWSECSContainerARNKey):    "sample_ecs_container_name",
		"custom_tag":       "example_custom_tag",
		"":                 "empty_string_key",
		"empty_string_val": "",
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
					string(semconv1_27.K8SPodUIDKey): "k8s_pod_uid_goes_here",
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

func TestGetOTelAttrFromEitherMap(t *testing.T) {
	for _, tt := range []struct {
		name      string
		map1      map[string]string
		map2      map[string]string
		normalize bool
		keys      []string
		expected  string
	}{
		{
			name:     "key in map1",
			map1:     map[string]string{"test.key": "value1"},
			map2:     map[string]string{"test.key": "value2"},
			keys:     []string{"test.key"},
			expected: "value1",
		},
		{
			name:     "key only in map2",
			map1:     map[string]string{},
			map2:     map[string]string{"test.key": "value2"},
			keys:     []string{"test.key"},
			expected: "value2",
		},
		{
			name:     "multiple keys, first match",
			map1:     map[string]string{"key1": "value1_map1"},
			map2:     map[string]string{"key1": "value1_map2", "key2": "value2_map2"},
			keys:     []string{"key1", "key2"},
			expected: "value1_map1",
		},
		{
			name:      "normalization enabled",
			map1:      map[string]string{"test.key": "  VALUE "},
			map2:      map[string]string{},
			keys:      []string{"test.key"},
			normalize: true,
			expected:  "_value",
		},
		{
			name:     "no match",
			map1:     map[string]string{"other.key": "value1"},
			map2:     map[string]string{"another.key": "value2"},
			keys:     []string{"missing.key"},
			expected: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pmap1 := pcommon.NewMap()
			for k, v := range tt.map1 {
				pmap1.PutStr(k, v)
			}
			pmap2 := pcommon.NewMap()
			for k, v := range tt.map2 {
				pmap2.PutStr(k, v)
			}
			actual := GetOTelAttrFromEitherMap(pmap1, pmap2, tt.normalize, tt.keys...)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetHostname(t *testing.T) {
	for _, tt := range []struct {
		name                                   string
		rattrs                                 map[string]string
		fallbackHost                           string
		expected                               string
		ignoreMissingDatadogFields             bool
		useDatadogNamespaceIfPresent           bool
		expectedFoundHostNameFromOTelSemantics bool
	}{
		{
			name:                                   "datadog.host.name",
			rattrs:                                 map[string]string{"datadog.host.name": "test-host"},
			expected:                               "test-host",
			useDatadogNamespaceIfPresent:           true,
			expectedFoundHostNameFromOTelSemantics: true,
		},
		{
			name:                                   "_dd.hostname",
			rattrs:                                 map[string]string{"_dd.hostname": "test-host"},
			expected:                               "test-host",
			useDatadogNamespaceIfPresent:           true,
			expectedFoundHostNameFromOTelSemantics: false,
		},
		{
			name:                                   "fallback hostname",
			fallbackHost:                           "test-host",
			expected:                               "test-host",
			useDatadogNamespaceIfPresent:           true,
			expectedFoundHostNameFromOTelSemantics: false,
		},
		{
			name:                                   "ignore missing datadog fields",
			rattrs:                                 map[string]string{string(semconv1_17.HostNameKey): "test-host"},
			expected:                               "",
			ignoreMissingDatadogFields:             true,
			useDatadogNamespaceIfPresent:           true,
			expectedFoundHostNameFromOTelSemantics: true,
		},
		{
			name:                                   "read from datadog fields",
			rattrs:                                 map[string]string{DDNamespaceKeys.Host(): "test-host", string(semconv1_17.HostNameKey): "test-host-semconv117"},
			expected:                               "test-host",
			useDatadogNamespaceIfPresent:           true,
			expectedFoundHostNameFromOTelSemantics: true,
		},
		{
			name:                                   "use datadog namespace if present - false",
			rattrs:                                 map[string]string{string(semconv1_17.HostNameKey): "semconv-host"},
			useDatadogNamespaceIfPresent:           false,
			expected:                               "semconv-host",
			expectedFoundHostNameFromOTelSemantics: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := pcommon.NewMap()
			for k, v := range tt.rattrs {
				res.PutStr(k, v)
			}
			actual, foundHostNameFromOTelSemantics := GetHostname(res, nil, tt.ignoreMissingDatadogFields, tt.useDatadogNamespaceIfPresent, &tt.fallbackHost)
			assert.Equal(t, tt.expectedFoundHostNameFromOTelSemantics, foundHostNameFromOTelSemantics)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name                         string
		rattrs                       map[string]string
		expected                     string
		ignoreMissingDatadogFields   bool
		useDatadogNamespaceIfPresent bool
	}{
		{
			name:                         "neither set",
			expected:                     "default",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "only in resource (semconv127)",
			rattrs:                       map[string]string{string(semconv1_27.DeploymentEnvironmentNameKey): "env-res-127"},
			expected:                     "env-res-127",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "only in resource (semconv117)",
			rattrs:                       map[string]string{string(semconv1_17.DeploymentEnvironmentKey): "env-res"},
			expected:                     "env-res",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "normalization",
			rattrs:                       map[string]string{string(semconv1_17.DeploymentEnvironmentKey): "  ENV "},
			expected:                     "_env",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "ignore missing datadog fields",
			rattrs:                       map[string]string{string(semconv1_17.DeploymentEnvironmentKey): "env-span"},
			expected:                     "default",
			ignoreMissingDatadogFields:   true,
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "read from datadog fields",
			rattrs:                       map[string]string{DDNamespaceKeys.Env(): "env-res", string(semconv1_17.DeploymentEnvironmentKey): "env-res-semconv117"},
			expected:                     "env-res",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "use datadog namespace if present - false",
			rattrs:                       map[string]string{string(semconv1_17.DeploymentEnvironmentKey): "semconv-env"},
			useDatadogNamespaceIfPresent: false,
			expected:                     "semconv-env",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := pcommon.NewMap()
			for k, v := range tt.rattrs {
				res.PutStr(k, v)
			}
			assert.Equal(t, tt.expected, GetEnv(res, tt.ignoreMissingDatadogFields, tt.useDatadogNamespaceIfPresent, nil))
		})
	}
}

func TestGetService(t *testing.T) {
	for _, tt := range []struct {
		name                         string
		rattrs                       map[string]string
		normalize                    bool
		expected                     string
		useDatadogNamespaceIfPresent bool
	}{
		{
			name:                         "service not set",
			expected:                     "otlpresourcenoservicename",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "normal service in resource",
			rattrs:                       map[string]string{string(conventions.ServiceNameKey): "svc"},
			expected:                     "svc",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "truncate long service",
			rattrs:                       map[string]string{string(conventions.ServiceNameKey): strings.Repeat("a", normalizeutil.MaxServiceLen+1)},
			normalize:                    true,
			expected:                     strings.Repeat("a", normalizeutil.MaxServiceLen),
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "invalid service",
			rattrs:                       map[string]string{string(conventions.ServiceNameKey): "%$^"},
			normalize:                    true,
			expected:                     normalizeutil.DefaultServiceName,
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "use datadog namespace if present - false",
			rattrs:                       map[string]string{string(conventions.ServiceNameKey): "semconv-svc"},
			useDatadogNamespaceIfPresent: false,
			expected:                     "semconv-svc",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := pcommon.NewMap()
			for k, v := range tt.rattrs {
				res.PutStr(k, v)
			}
			actual := GetService(res, tt.normalize, false, tt.useDatadogNamespaceIfPresent, nil)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetResource(t *testing.T) {
	for _, tt := range []struct {
		name                         string
		sattrs                       map[string]string
		normalize                    bool
		expected                     string
		useDatadogNamespaceIfPresent bool
	}{
		{
			name:                         "resource not set",
			expected:                     "span_name",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "normal resource",
			sattrs:                       map[string]string{"resource.name": "res"},
			expected:                     "res",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "HTTP request method resource",
			sattrs:                       map[string]string{"http.request.method": "GET"},
			expected:                     "GET",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "HTTP method and route resource",
			sattrs:                       map[string]string{string(semconv1_27.HTTPRequestMethodKey): "GET", string(semconv1_27.HTTPRouteKey): "/"},
			expected:                     "GET /",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "GraphQL with no type",
			sattrs:                       map[string]string{"graphql.operation.name": "myQuery"},
			expected:                     "span_name",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "GraphQL with only type",
			sattrs:                       map[string]string{"graphql.operation.type": "query"},
			expected:                     "query",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "GraphQL with type and name",
			sattrs:                       map[string]string{"graphql.operation.type": "query", "graphql.operation.name": "myQuery"},
			expected:                     "query myQuery",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name: "SQL statement resource",
			sattrs: map[string]string{
				string(semconv1_27.DBSystemKey):    "mysql",
				string(semconv1_17.DBStatementKey): "SELECT * FROM table WHERE id = 12345",
			},
			expected: "SELECT * FROM table WHERE id = 12345",
		},
		{
			name: "Redis command resource",
			sattrs: map[string]string{
				string(semconv1_27.DBSystemKey):    "redis",
				string(semconv1_27.DBQueryTextKey): "SET key value",
			},
			expected: "SET key value",
		},
		{
			name:     "messaging.operation",
			sattrs:   map[string]string{"messaging.operation": "process"},
			expected: "process",
		},
		{
			name:     "rpc.method",
			sattrs:   map[string]string{"rpc.method": "span_method", "rpc.service": "span_service"},
			expected: "span_method span_service",
		},
		{
			name:     "GraphQL type",
			sattrs:   map[string]string{"graphql.operation.type": "query", "graphql.operation.name": "myQuery"},
			expected: "query myQuery",
		},
		{
			name:     "DB statement",
			sattrs:   map[string]string{"db.system": "mysql", "db.statement": "SELECT * FROM span"},
			expected: "SELECT * FROM span",
		},
		{
			name:                         "fallback to span name if nothing set",
			sattrs:                       map[string]string{},
			expected:                     "span_name",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "use datadog namespace if present - false",
			sattrs:                       map[string]string{DDNamespaceKeys.ResourceName(): "datadog-resource", "resource.name": "semconv-resource"},
			useDatadogNamespaceIfPresent: false,
			expected:                     "semconv-resource",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := pcommon.NewMap()
			for k, v := range tt.sattrs {
				span.PutStr(k, v)
			}
			fallback := "span_name"
			actual := GetResourceName(ptrace.SpanKindServer, span, false, tt.useDatadogNamespaceIfPresent, &fallback)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetSpanType(t *testing.T) {
	for _, tt := range []struct {
		name                         string
		spanKind                     ptrace.SpanKind
		sattrs                       map[string]string
		expected                     string
		useDatadogNamespaceIfPresent bool
	}{
		{
			name:     "override with span.type attr",
			spanKind: ptrace.SpanKindInternal,
			sattrs:   map[string]string{"span.type": "my-type"},
			expected: "my-type",
		},
		{
			name:     "web span",
			spanKind: ptrace.SpanKindServer,
			expected: "web",
		},
		{
			name:     "redis span",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{string(conventions.DBSystemKey): "redis"},
			expected: spanTypeRedis,
		},
		{
			name:     "memcached span",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{string(conventions.DBSystemKey): "memcached"},
			expected: spanTypeMemcached,
		},
		{
			name:     "sql db client span",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{string(conventions.DBSystemKey): conventions.DBSystemPostgreSQL.Value.AsString()},
			expected: spanTypeSQL,
		},
		{
			name:     "elastic db client span",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{string(conventions.DBSystemKey): conventions.DBSystemElasticsearch.Value.AsString()},
			expected: spanTypeElasticsearch,
		},
		{
			name:     "opensearch db client span",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{string(conventions.DBSystemKey): semconv1_17.DBSystemOpensearch.Value.AsString()},
			expected: spanTypeOpenSearch,
		},
		{
			name:     "cassandra db client span",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{string(conventions.DBSystemKey): conventions.DBSystemCassandra.Value.AsString()},
			expected: spanTypeCassandra,
		},
		{
			name:     "other db client span",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{string(conventions.DBSystemKey): conventions.DBSystemCouchDB.Value.AsString()},
			expected: spanTypeDB,
		},
		{
			name:     "http client span",
			spanKind: ptrace.SpanKindClient,
			expected: "http",
		},
		{
			name:     "other custom span",
			spanKind: ptrace.SpanKindInternal,
			expected: "custom",
		},
		{
			name:     "span.type only in span",
			sattrs:   map[string]string{"span.type": "span-type"},
			expected: "span-type",
		},
		{
			name:     "db.system only in span",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{"db.system": "redis"},
			expected: spanTypeRedis,
		},
		{
			name:                         "use datadog namespace if present - false",
			spanKind:                     ptrace.SpanKindInternal,
			sattrs:                       map[string]string{DDNamespaceKeys.SpanType(): "datadog-type", "span.type": "semconv-type"},
			useDatadogNamespaceIfPresent: false,
			expected:                     "semconv-type",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			span := pcommon.NewMap()
			for k, v := range tt.sattrs {
				span.PutStr(k, v)
			}
			actual := GetSpanType(tt.spanKind, span, false, tt.useDatadogNamespaceIfPresent, nil)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetVersion(t *testing.T) {
	tests := []struct {
		name                         string
		rattrs                       map[string]string
		expected                     string
		ignoreMissingDatadogFields   bool
		useDatadogNamespaceIfPresent bool
	}{
		{
			name:     "unset",
			expected: "",
		},
		{
			name:     "only in resource",
			rattrs:   map[string]string{string(semconv1_27.ServiceVersionKey): "v1"},
			expected: "v1",
		},
		{
			name:     "normalization",
			rattrs:   map[string]string{string(semconv1_27.ServiceVersionKey): "  V1 "},
			expected: "_v1",
		},
		{
			name:                       "ignore missing datadog fields",
			rattrs:                     map[string]string{string(semconv1_27.ServiceVersionKey): "v4"},
			expected:                   "",
			ignoreMissingDatadogFields: true,
		},
		{
			name:                         "read from datadog fields",
			rattrs:                       map[string]string{DDNamespaceKeys.Version(): "v4", string(semconv1_27.ServiceVersionKey): "v4-semconv127"},
			expected:                     "v4",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "use datadog namespace if present - false",
			rattrs:                       map[string]string{DDNamespaceKeys.Version(): "datadog-version", string(semconv1_27.ServiceVersionKey): "semconv-version"},
			useDatadogNamespaceIfPresent: false,
			expected:                     "semconv-version",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := pcommon.NewMap()
			for k, v := range tt.rattrs {
				res.PutStr(k, v)
			}
			assert.Equal(t, tt.expected, GetVersion(res, tt.ignoreMissingDatadogFields, tt.useDatadogNamespaceIfPresent, nil))
		})
	}
}

func TestGetStatusCode(t *testing.T) {
	tests := []struct {
		name                         string
		sattrs                       map[string]uint32
		expected                     uint32
		ignoreMissingDatadogFields   bool
		useDatadogNamespaceIfPresent bool
	}{
		{
			name:     "neither set",
			expected: 0,
		},
		{
			name: "only semconv117.HTTPStatusCodeKey",
			sattrs: map[string]uint32{
				string(semconv1_17.HTTPStatusCodeKey): 200,
			},
			expected: 200,
		},
		{
			name: "both semconv117.HTTPStatusCodeKey and http.response.status_code, semconv117.HTTPStatusCodeKey wins",
			sattrs: map[string]uint32{
				string(semconv1_17.HTTPStatusCodeKey): 200,
				"http.response.status_code":           201,
			},
			expected: 200,
		},
		{
			name: "only semconv117.HTTPStatusCodeKey",
			sattrs: map[string]uint32{
				string(semconv1_17.HTTPStatusCodeKey): 201,
			},
			expected: 201,
		},
		{
			name: "both semconv117.HTTPStatusCodeKey and http.response.status_code, semconv117.HTTPStatusCodeKey wins",
			sattrs: map[string]uint32{
				string(semconv1_17.HTTPStatusCodeKey): 201,
				"http.response.status_code":           202,
			},
			expected: 201,
		},
		{
			name:                       "ignore missing datadog fields",
			sattrs:                     map[string]uint32{string(semconv1_17.HTTPStatusCodeKey): 205},
			expected:                   0,
			ignoreMissingDatadogFields: true,
		},
		{
			name:                         "read from datadog fields",
			sattrs:                       map[string]uint32{DDNamespaceKeys.HTTPStatusCode(): 206, string(semconv1_17.HTTPStatusCodeKey): 210},
			expected:                     206,
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "use datadog namespace if present - false",
			sattrs:                       map[string]uint32{string(semconv1_27.HTTPResponseStatusCodeKey): 400, DDNamespaceKeys.HTTPStatusCode(): 300},
			useDatadogNamespaceIfPresent: false,
			expected:                     400,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := pcommon.NewMap()
			for k, v := range tt.sattrs {
				span.PutInt(k, int64(v))
			}
			actual, err := GetStatusCode(span, tt.ignoreMissingDatadogFields, tt.useDatadogNamespaceIfPresent, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetContainerID(t *testing.T) {
	tests := []struct {
		name                         string
		rattrs                       map[string]string
		expected                     string
		ignoreMissingDatadogFields   bool
		useDatadogNamespaceIfPresent bool
	}{
		{
			name:     "neither set",
			expected: "",
		},
		{
			name:     "basic",
			rattrs:   map[string]string{string(semconv1_17.ContainerIDKey): "cid-res"},
			expected: "cid-res",
		},
		{
			name:     "normalization",
			rattrs:   map[string]string{string(semconv1_17.ContainerIDKey): "  CID "},
			expected: "_cid",
		},
		{
			name:                       "ignore missing datadog fields",
			rattrs:                     map[string]string{string(semconv1_17.ContainerIDKey): "cid-span"},
			expected:                   "",
			ignoreMissingDatadogFields: true,
		},
		{
			name:                         "read from datadog fields",
			rattrs:                       map[string]string{DDNamespaceKeys.ContainerID(): "cid-res", string(semconv1_17.ContainerIDKey): "cid-res-semconv117"},
			expected:                     "cid-res",
			useDatadogNamespaceIfPresent: true,
		},
		{
			name:                         "use datadog namespace if present - false",
			rattrs:                       map[string]string{string(semconv1_17.ContainerIDKey): "semconv-container", DDNamespaceKeys.ContainerID(): "datadog-container"},
			useDatadogNamespaceIfPresent: false,
			expected:                     "semconv-container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := pcommon.NewMap()
			for k, v := range tt.rattrs {
				res.PutStr(k, v)
			}
			assert.Equal(t, tt.expected, GetContainerID(res, tt.ignoreMissingDatadogFields, tt.useDatadogNamespaceIfPresent, nil))
		})
	}
}

func TestGetContainerTags(t *testing.T) {
	for _, tt := range []struct {
		name     string
		resAttrs map[string]string
		tagKeys  []string
		expected []string
	}{
		{
			name: "container tags from resource attrs",
			resAttrs: map[string]string{
				string(semconv1_27.ContainerIDKey):        "cid",
				string(semconv1_27.ContainerNameKey):      "cname",
				string(semconv1_27.ContainerImageNameKey): "ciname",
				string(conventions.ContainerImageTagKey):  "citag",
				"az":                                      "my-az",
			},
			tagKeys: []string{
				"az",
				string(semconv1_27.ContainerIDKey),
				string(semconv1_27.ContainerNameKey),
				string(semconv1_27.ContainerImageNameKey),
				string(conventions.ContainerImageTagKey),
			},
			expected: []string{
				"az:my-az",
				"container_id:cid",
				"container_name:cname",
				"image_name:ciname",
				"image_tag:citag",
			},
		},
		{
			name: "custom datadog container tags",
			resAttrs: map[string]string{
				"datadog.container.tag.custom.team": "otel-team",
				"datadog.container.tag.env":         "custom-env",
			},
			tagKeys: []string{
				"custom.team",
				"env",
			},
			expected: []string{
				"custom.team:otel-team",
				"env:custom-env",
			},
		},
		{
			name: "semantic conventions take precedence over custom tags",
			resAttrs: map[string]string{
				string(semconv1_27.ContainerIDKey):   "semconv-cid",
				"datadog.container.tag.container_id": "custom-cid",
			},
			tagKeys: []string{
				string(semconv1_27.ContainerIDKey),
			},
			expected: []string{
				"container_id:semconv-cid",
			},
		},
		{
			name: "additional tags not in ContainerMappings",
			resAttrs: map[string]string{
				"custom_key": "custom_value",
			},
			tagKeys: []string{"custom_key"},
			expected: []string{
				"custom_key:custom_value",
			},
		},
		{
			name: "empty tag keys",
			resAttrs: map[string]string{
				string(semconv1_27.ContainerIDKey): "cid",
			},
			tagKeys:  []string{},
			expected: []string{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resAttrs := pcommon.NewMap()
			for k, v := range tt.resAttrs {
				resAttrs.PutStr(k, v)
			}
			actual := GetContainerTags(resAttrs, tt.tagKeys)
			assert.ElementsMatch(t, tt.expected, actual)
		})
	}
}

func TestGetOperationName(t *testing.T) {
	tests := []struct {
		name                         string
		spanKind                     ptrace.SpanKind
		sattrs                       map[string]string
		ignoreMissingDatadogFields   bool
		useDatadogNamespaceIfPresent bool
		fallbackOverride             *string
		expected                     string
	}{
		{
			name:                         "datadog.name takes precedence",
			spanKind:                     ptrace.SpanKindServer,
			sattrs:                       map[string]string{"operation.name": "op-name", DDNamespaceKeys.OperationName(): "custom-name"},
			useDatadogNamespaceIfPresent: true,
			expected:                     "custom-name",
		},
		{
			name:     "operation.name attribute",
			spanKind: ptrace.SpanKindServer,
			sattrs:   map[string]string{"operation.name": "custom-op"},
			expected: "custom-op",
		},
		{
			name:     "http server request",
			spanKind: ptrace.SpanKindServer,
			sattrs:   map[string]string{"http.request.method": "GET"},
			expected: "http.server.request",
		},
		{
			name:     "http client request",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{"http.request.method": "POST"},
			expected: "http.client.request",
		},
		{
			name:     "database query",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{string(conventions.DBSystemKey): "mysql"},
			expected: "mysql.query",
		},
		{
			name:     "messaging operation",
			spanKind: ptrace.SpanKindProducer,
			sattrs:   map[string]string{string(conventions.MessagingSystemKey): "kafka", string(conventions.MessagingOperationKey): "send"},
			expected: "kafka.send",
		},
		{
			name:     "rpc grpc server",
			spanKind: ptrace.SpanKindServer,
			sattrs:   map[string]string{string(conventions.RPCSystemKey): "grpc"},
			expected: "grpc.server.request",
		},
		{
			name:     "rpc grpc client",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{string(conventions.RPCSystemKey): "grpc"},
			expected: "grpc.client.request",
		},
		{
			name:     "aws service client",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{string(conventions.RPCSystemKey): "aws-api", string(conventions.RPCServiceKey): "s3"},
			expected: "aws.s3.request",
		},
		{
			name:     "faas client invoke",
			spanKind: ptrace.SpanKindClient,
			sattrs:   map[string]string{string(conventions.FaaSInvokedProviderKey): "aws", string(conventions.FaaSInvokedNameKey): "my-function"},
			expected: "aws.my-function.invoke",
		},
		{
			name:     "faas server invoke",
			spanKind: ptrace.SpanKindServer,
			sattrs:   map[string]string{string(conventions.FaaSTriggerKey): "http"},
			expected: "http.invoke",
		},
		{
			name:     "graphql server request",
			spanKind: ptrace.SpanKindServer,
			sattrs:   map[string]string{"graphql.operation.type": "query"},
			expected: "graphql.server.request",
		},
		{
			name:     "generic server request",
			spanKind: ptrace.SpanKindServer,
			expected: "server.request",
		},
		{
			name:     "generic client request",
			spanKind: ptrace.SpanKindClient,
			expected: "client.request",
		},
		{
			name:     "protocol server request",
			spanKind: ptrace.SpanKindServer,
			sattrs:   map[string]string{"network.protocol.name": "tcp"},
			expected: "tcp.server.request",
		},
		{
			name:                       "ignore missing datadog fields - no operation name",
			spanKind:                   ptrace.SpanKindServer,
			sattrs:                     map[string]string{"operation.name": "custom-op"},
			ignoreMissingDatadogFields: true,
			expected:                   "server",
		},
		{
			name:                         "fallback override",
			spanKind:                     ptrace.SpanKindInternal,
			ignoreMissingDatadogFields:   true,
			useDatadogNamespaceIfPresent: true,
			fallbackOverride:             func() *string { s := "fallback-name"; return &s }(),
			expected:                     "fallback-name",
		},
		{
			name:                         "span kind fallback",
			spanKind:                     ptrace.SpanKindConsumer,
			ignoreMissingDatadogFields:   true,
			useDatadogNamespaceIfPresent: true,
			expected:                     "consumer",
		},
		{
			name:                         "use datadog namespace if present - false",
			spanKind:                     ptrace.SpanKindServer,
			sattrs:                       map[string]string{"operation.name": "semconv-name", DDNamespaceKeys.OperationName(): "datadog-name"},
			useDatadogNamespaceIfPresent: false,
			expected:                     "semconv-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := pcommon.NewMap()
			for k, v := range tt.sattrs {
				span.PutStr(k, v)
			}
			actual := GetOperationName(tt.spanKind, span, tt.ignoreMissingDatadogFields, tt.useDatadogNamespaceIfPresent, tt.fallbackOverride)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetPeerTagsFromOTelAttributes(t *testing.T) {
	tests := []struct {
		name        string
		signalAttrs map[string]string
		resAttrs    map[string]string
		peerTagKeys map[string]struct{}
		expected    []string
	}{
		{
			name:        "no peer tag keys",
			signalAttrs: map[string]string{"key1": "value1"},
			resAttrs:    map[string]string{"key2": "value2"},
			peerTagKeys: nil,
			expected:    []string{},
		},
		{
			name:        "empty peer tag keys",
			signalAttrs: map[string]string{"key1": "value1"},
			resAttrs:    map[string]string{"key2": "value2"},
			peerTagKeys: map[string]struct{}{},
			expected:    []string{},
		},
		{
			name:        "peer tags from signal attrs only",
			signalAttrs: map[string]string{"peer.service": "user-service", "other.key": "other-value"},
			resAttrs:    map[string]string{"key2": "value2"},
			peerTagKeys: map[string]struct{}{"peer.service": {}},
			expected:    []string{"peer.service:user-service"},
		},
		{
			name:        "peer tags from resource attrs only",
			signalAttrs: map[string]string{"key1": "value1"},
			resAttrs:    map[string]string{"peer.service": "db-service", "other.key": "other-value"},
			peerTagKeys: map[string]struct{}{"peer.service": {}},
			expected:    []string{"peer.service:db-service"},
		},
		{
			name:        "signal attrs override resource attrs",
			signalAttrs: map[string]string{"peer.service": "signal-service"},
			resAttrs:    map[string]string{"peer.service": "resource-service"},
			peerTagKeys: map[string]struct{}{"peer.service": {}},
			expected:    []string{"peer.service:signal-service"},
		},
		{
			name:        "multiple peer tags",
			signalAttrs: map[string]string{"peer.service": "user-service", "db.name": "mydb"},
			resAttrs:    map[string]string{"peer.hostname": "db-host"},
			peerTagKeys: map[string]struct{}{"peer.service": {}, "peer.hostname": {}, "db.name": {}},
			expected:    []string{"peer.service:user-service", "peer.hostname:db-host", "db.name:mydb"},
		},
		{
			name:        "normalization applied",
			signalAttrs: map[string]string{"peer.service": "  USER SERVICE "},
			resAttrs:    map[string]string{},
			peerTagKeys: map[string]struct{}{"peer.service": {}},
			expected:    []string{"peer.service:__user_service_"},
		},
		{
			name:        "only requested keys included",
			signalAttrs: map[string]string{"peer.service": "user-service", "ignored.key": "ignored-value"},
			resAttrs:    map[string]string{"peer.hostname": "db-host", "another.ignored": "ignored"},
			peerTagKeys: map[string]struct{}{"peer.service": {}},
			expected:    []string{"peer.service:user-service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signalAttrs := pcommon.NewMap()
			for k, v := range tt.signalAttrs {
				signalAttrs.PutStr(k, v)
			}
			resAttrs := pcommon.NewMap()
			for k, v := range tt.resAttrs {
				resAttrs.PutStr(k, v)
			}
			actual := GetSpecifiedKeysFromOTelAttributes(signalAttrs, resAttrs, tt.peerTagKeys)
			assert.ElementsMatch(t, tt.expected, actual)
		})
	}
}

func TestNilFallbacks(t *testing.T) {
	signalAttrs := pcommon.NewMap()
	resAttrs := pcommon.NewMap()

	t.Run("GetEnv", func(t *testing.T) {
		env := GetEnv(signalAttrs, true, true, nil)
		assert.Equal(t, DefaultOTLPEnvironmentName, env)
	})

	t.Run("GetService", func(t *testing.T) {
		service := GetService(resAttrs, true, true, true, nil)
		assert.Equal(t, DefaultOTLPServiceName, service)
	})

	t.Run("GetResourceName", func(t *testing.T) {
		resource := GetResourceName(ptrace.SpanKindClient, signalAttrs, true, true, nil)
		assert.Equal(t, "", resource)
	})

	t.Run("GetOperationName", func(t *testing.T) {
		operation := GetOperationName(ptrace.SpanKindInternal, signalAttrs, true, true, nil)
		assert.Equal(t, "internal", operation)
	})

	t.Run("GetSpanType", func(t *testing.T) {
		spanType := GetSpanType(ptrace.SpanKindServer, signalAttrs, true, true, nil)
		assert.Equal(t, "custom", spanType)
	})

	t.Run("GetVersion", func(t *testing.T) {
		version := GetVersion(signalAttrs, true, true, nil)
		assert.Equal(t, "", version)
	})

	t.Run("GetStatusCode", func(t *testing.T) {
		statusCode, err := GetStatusCode(signalAttrs, true, true, nil)
		assert.NoError(t, err)
		assert.Equal(t, uint32(0), statusCode)
	})

	t.Run("GetContainerID", func(t *testing.T) {
		containerID := GetContainerID(resAttrs, true, true, nil)
		assert.Equal(t, "", containerID)
	})

	t.Run("GetHostname", func(t *testing.T) {
		hostname, _ := GetHostname(resAttrs, nil, true, true, nil)
		assert.Equal(t, "", hostname)
	})
}
