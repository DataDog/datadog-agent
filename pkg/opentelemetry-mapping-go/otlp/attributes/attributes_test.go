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
	semconv127 "go.opentelemetry.io/collector/semconv/v1.27.0"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"
)

func TestTagsFromAttributes(t *testing.T) {
	attributeMap := map[string]interface{}{
		semconv127.AttributeProcessExecutableName:  "otelcol",
		semconv127.AttributeProcessExecutablePath:  "/usr/bin/cmd/otelcol",
		semconv127.AttributeProcessCommand:         "cmd/otelcol",
		semconv127.AttributeProcessCommandLine:     "cmd/otelcol --config=\"/path/to/config.yaml\"",
		semconv127.AttributeProcessPID:             1,
		semconv127.AttributeProcessOwner:           "root",
		semconv127.AttributeOSType:                 "linux",
		semconv127.AttributeK8SDaemonSetName:       "daemon_set_name",
		semconv127.AttributeAWSECSClusterARN:       "cluster_arn",
		semconv127.AttributeContainerRuntime:       "cro",
		"tags.datadoghq.com/service":               "service_name",
		conventions.AttributeDeploymentEnvironment: "prod",
		semconv127.AttributeContainerName:          "custom",
		"datadog.container.tag.custom.team":        "otel",
		"kube_cronjob":                             "cron",
	}
	attrs := pcommon.NewMap()
	attrs.FromRaw(attributeMap)

	assert.ElementsMatch(t, []string{
		fmt.Sprintf("%s:%s", semconv127.AttributeProcessExecutableName, "otelcol"),
		fmt.Sprintf("%s:%s", semconv127.AttributeOSType, "linux"),
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
			semconv127.AttributeContainerName:         "sample_app",
			conventions.AttributeContainerImageTag:    "sample_app_image_tag",
			semconv127.AttributeContainerRuntime:      "cro",
			semconv127.AttributeK8SContainerName:      "kube_sample_app",
			semconv127.AttributeK8SReplicaSetName:     "sample_replica_set",
			semconv127.AttributeK8SDaemonSetName:      "sample_daemonset_name",
			semconv127.AttributeK8SPodName:            "sample_pod_name",
			semconv127.AttributeCloudProvider:         "sample_cloud_provider",
			semconv127.AttributeCloudRegion:           "sample_region",
			semconv127.AttributeCloudAvailabilityZone: "sample_zone",
			semconv127.AttributeAWSECSTaskFamily:      "sample_task_family",
			semconv127.AttributeAWSECSClusterARN:      "sample_ecs_cluster_name",
			semconv127.AttributeAWSECSContainerARN:    "sample_ecs_container_name",
			"datadog.container.tag.custom.team":       "otel",
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
			semconv127.AttributeContainerName:      "ok",
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
		semconv127.AttributeContainerName:         "sample_app",
		conventions.AttributeContainerImageTag:    "sample_app_image_tag",
		semconv127.AttributeContainerRuntime:      "cro",
		semconv127.AttributeK8SContainerName:      "kube_sample_app",
		semconv127.AttributeK8SReplicaSetName:     "sample_replica_set",
		semconv127.AttributeK8SDaemonSetName:      "sample_daemonset_name",
		semconv127.AttributeK8SPodName:            "sample_pod_name",
		semconv127.AttributeCloudProvider:         "sample_cloud_provider",
		semconv127.AttributeCloudRegion:           "sample_region",
		semconv127.AttributeCloudAvailabilityZone: "sample_zone",
		semconv127.AttributeAWSECSTaskFamily:      "sample_task_family",
		semconv127.AttributeAWSECSClusterARN:      "sample_ecs_cluster_name",
		semconv127.AttributeAWSECSContainerARN:    "sample_ecs_container_name",
		"custom_tag":                              "example_custom_tag",
		"":                                        "empty_string_key",
		"empty_string_val":                        "",
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
					semconv127.AttributeContainerID: "container_id_goes_here",
					semconv127.AttributeK8SPodUID:   "k8s_pod_uid_goes_here",
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
					semconv127.AttributeContainerID: "container_id_goes_here",
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
					semconv127.AttributeK8SPodUID: "k8s_pod_uid_goes_here",
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
