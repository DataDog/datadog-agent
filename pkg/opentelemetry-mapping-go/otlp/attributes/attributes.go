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

// Package attributes provides attributes for the OpenTelemetry Collector.
package attributes

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	semconv127 "go.opentelemetry.io/otel/semconv/v1.27.0"
	conventions "go.opentelemetry.io/otel/semconv/v1.6.1"
)

// customContainerTagPrefix defines the prefix for custom container tags.
const customContainerTagPrefix = "datadog.container.tag."

var (
	// coreMapping defines the mapping between OpenTelemetry semantic conventions
	// and Datadog Agent conventions for env, service and version.
	coreMapping = map[string]string{
		// Datadog conventions
		// https://docs.datadoghq.com/getting_started/tagging/unified_service_tagging/
		string(conventions.DeploymentEnvironmentKey):    "env",
		string(semconv127.ServiceNameKey):               "service",
		string(semconv127.ServiceVersionKey):            "version",
		string(semconv127.DeploymentEnvironmentNameKey): "env",
	}

	// ContainerMappings defines the mapping between OpenTelemetry semantic conventions
	// and Datadog Agent conventions for containers.
	ContainerMappings = map[string]string{
		// Containers
		string(semconv127.ContainerIDKey):        "container_id",
		string(semconv127.ContainerNameKey):      "container_name",
		string(semconv127.ContainerImageNameKey): "image_name",
		string(conventions.ContainerImageTagKey): "image_tag",
		string(semconv127.ContainerRuntimeKey):   "runtime",

		// Cloud conventions
		// https://www.datadoghq.com/blog/tagging-best-practices/
		string(semconv127.CloudProviderKey):         "cloud_provider",
		string(semconv127.CloudRegionKey):           "region",
		string(semconv127.CloudAvailabilityZoneKey): "zone",

		// ECS conventions
		// https://github.com/DataDog/datadog-agent/blob/e081bed/pkg/tagger/collectors/ecs_extract.go
		string(semconv127.AWSECSTaskFamilyKey):   "task_family",
		string(semconv127.AWSECSTaskARNKey):      "task_arn",
		string(semconv127.AWSECSClusterARNKey):   "ecs_cluster_name",
		string(semconv127.AWSECSTaskRevisionKey): "task_version",
		string(semconv127.AWSECSContainerARNKey): "ecs_container_name",

		// Kubernetes resource name (via semantic conventions)
		// https://github.com/DataDog/datadog-agent/blob/e081bed/pkg/util/kubernetes/const.go
		string(semconv127.K8SContainerNameKey):   "kube_container_name",
		string(semconv127.K8SClusterNameKey):     "kube_cluster_name",
		string(semconv127.K8SDeploymentNameKey):  "kube_deployment",
		string(semconv127.K8SReplicaSetNameKey):  "kube_replica_set",
		string(semconv127.K8SStatefulSetNameKey): "kube_stateful_set",
		string(semconv127.K8SDaemonSetNameKey):   "kube_daemon_set",
		string(semconv127.K8SJobNameKey):         "kube_job",
		string(semconv127.K8SCronJobNameKey):     "kube_cronjob",
		string(semconv127.K8SNamespaceNameKey):   "kube_namespace",
		string(semconv127.K8SPodNameKey):         "pod_name",
	}

	// Kubernetes mappings defines the mapping between Kubernetes conventions (both general and Datadog specific)
	// and Datadog Agent conventions. The Datadog Agent conventions can be found at
	// https://github.com/DataDog/datadog-agent/blob/e081bed/pkg/tagger/collectors/const.go and
	// https://github.com/DataDog/datadog-agent/blob/e081bed/pkg/util/kubernetes/const.go
	kubernetesMapping = map[string]string{
		// Standard Datadog labels
		"tags.datadoghq.com/env":     "env",
		"tags.datadoghq.com/service": "service",
		"tags.datadoghq.com/version": "version",

		// Standard Kubernetes labels
		"app.kubernetes.io/name":       "kube_app_name",
		"app.kubernetes.io/instance":   "kube_app_instance",
		"app.kubernetes.io/version":    "kube_app_version",
		"app.kuberenetes.io/component": "kube_app_component",
		"app.kubernetes.io/part-of":    "kube_app_part_of",
		"app.kubernetes.io/managed-by": "kube_app_managed_by",
	}

	// Kubernetes out of the box Datadog tags
	// https://docs.datadoghq.com/containers/kubernetes/tag/?tab=containerizedagent#out-of-the-box-tags
	// https://github.com/DataDog/datadog-agent/blob/d33d042d6786e8b85f72bb627fbf06ad8a658031/comp/core/tagger/taggerimpl/collectors/workloadmeta_extract.go
	// Note: if any OTel semantics happen to overlap with these tag names, they will also be added as Datadog tags.
	kubernetesDDTags = map[string]struct{}{
		"architecture":                {},
		"availability-zone":           {},
		"chronos_job":                 {},
		"chronos_job_owner":           {},
		"cluster_name":                {},
		"container_id":                {},
		"container_name":              {},
		"dd_remote_config_id":         {},
		"dd_remote_config_rev":        {},
		"display_container_name":      {},
		"docker_image":                {},
		"ecs_cluster_name":            {},
		"ecs_container_name":          {},
		"eks_fargate_node":            {},
		"env":                         {},
		"git.commit.sha":              {},
		"git.repository_url":          {},
		"image_id":                    {},
		"image_name":                  {},
		"image_tag":                   {},
		"kube_app_component":          {},
		"kube_app_instance":           {},
		"kube_app_managed_by":         {},
		"kube_app_name":               {},
		"kube_app_part_of":            {},
		"kube_app_version":            {},
		"kube_container_name":         {},
		"kube_cronjob":                {},
		"kube_daemon_set":             {},
		"kube_deployment":             {},
		"kube_job":                    {},
		"kube_namespace":              {},
		"kube_ownerref_kind":          {},
		"kube_ownerref_name":          {},
		"kube_priority_class":         {},
		"kube_qos":                    {},
		"kube_replica_set":            {},
		"kube_replication_controller": {},
		"kube_service":                {},
		"kube_stateful_set":           {},
		"language":                    {},
		"marathon_app":                {},
		"mesos_task":                  {},
		"nomad_dc":                    {},
		"nomad_group":                 {},
		"nomad_job":                   {},
		"nomad_namespace":             {},
		"nomad_task":                  {},
		"oshift_deployment":           {},
		"oshift_deployment_config":    {},
		"os_name":                     {},
		"os_version":                  {},
		"persistentvolumeclaim":       {},
		"pod_name":                    {},
		"pod_phase":                   {},
		"rancher_container":           {},
		"rancher_service":             {},
		"rancher_stack":               {},
		"region":                      {},
		"service":                     {},
		"short_image":                 {},
		"swarm_namespace":             {},
		"swarm_service":               {},
		"task_name":                   {},
		"task_family":                 {},
		"task_version":                {},
		"task_arn":                    {},
		"version":                     {},
	}

	// HTTPMappings defines the mapping between OpenTelemetry semantic conventions
	// and Datadog Agent conventions for HTTP attributes.
	HTTPMappings = map[string]string{
		string(semconv127.ClientAddressKey):          "http.client_ip",
		string(semconv127.HTTPResponseBodySizeKey):   "http.response.content_length",
		string(semconv127.HTTPResponseStatusCodeKey): "http.status_code",
		string(semconv127.HTTPRequestBodySizeKey):    "http.request.content_length",
		"http.request.header.referrer":               "http.referrer",
		string(semconv127.HTTPRequestMethodKey):      "http.method",
		string(semconv127.HTTPRouteKey):              "http.route",
		string(semconv127.NetworkProtocolVersionKey): "http.version",
		string(semconv127.ServerAddressKey):          "http.server_name",
		string(semconv127.URLFullKey):                "http.url",
		string(semconv127.UserAgentOriginalKey):      "http.useragent",
	}
)

// TagsFromAttributes converts a selected list of attributes
// to a tag list that can be added to metrics.
func TagsFromAttributes(attrs pcommon.Map) []string {
	tags := make([]string, 0, attrs.Len())

	var processAttributes processAttributes
	var systemAttributes systemAttributes

	attrs.Range(func(key string, value pcommon.Value) bool {
		switch key {
		// Process attributes
		case string(semconv127.ProcessExecutableNameKey):
			processAttributes.ExecutableName = value.Str()
		case string(semconv127.ProcessExecutablePathKey):
			processAttributes.ExecutablePath = value.Str()
		case string(semconv127.ProcessCommandKey):
			processAttributes.Command = value.Str()
		case string(semconv127.ProcessCommandLineKey):
			processAttributes.CommandLine = value.Str()
		case string(semconv127.ProcessPIDKey):
			processAttributes.PID = value.Int()
		case string(semconv127.ProcessOwnerKey):
			processAttributes.Owner = value.Str()

		// System attributes
		case string(semconv127.OSTypeKey):
			systemAttributes.OSType = value.Str()
		}

		// core attributes mapping
		if datadogKey, found := coreMapping[key]; found && value.Str() != "" {
			tags = append(tags, fmt.Sprintf("%s:%s", datadogKey, value.Str()))
		}

		// Kubernetes labels mapping
		if datadogKey, found := kubernetesMapping[key]; found && value.Str() != "" {
			tags = append(tags, fmt.Sprintf("%s:%s", datadogKey, value.Str()))
		}

		// Kubernetes DD tags
		if _, found := kubernetesDDTags[key]; found {
			tags = append(tags, fmt.Sprintf("%s:%s", key, value.Str()))
		}
		return true
	})

	// Container Tag mappings
	ctags := ContainerTagsFromResourceAttributes(attrs)
	for key, val := range ctags {
		tags = append(tags, fmt.Sprintf("%s:%s", key, val))
	}

	tags = append(tags, processAttributes.extractTags()...)
	tags = append(tags, systemAttributes.extractTags()...)

	return tags
}

// OriginIDFromAttributes gets the origin IDs from resource attributes.
// If not found, an empty string is returned for each of them.
func OriginIDFromAttributes(attrs pcommon.Map) (originID string) {
	// originID is always empty. Container ID is preferred over Kubernetes pod UID.
	// Prefixes come from pkg/util/kubernetes/kubelet and pkg/util/containers.
	if containerID, ok := attrs.Get(string(conventions.ContainerIDKey)); ok {
		originID = "container_id://" + containerID.AsString()
	} else if podUID, ok := attrs.Get(string(conventions.K8SPodUIDKey)); ok {
		originID = "kubernetes_pod_uid://" + podUID.AsString()
	}
	return
}

// ContainerTagsFromResourceAttributes extracts container tags from the given
// set of resource attributes. Container tags are extracted via semantic
// conventions. Customer container tags are extracted via resource attributes
// prefixed by datadog.container.tag. Custom container tag values of a different type
// than ValueTypeStr will be ignored.
// In the case of duplicates between semantic conventions and custom resource attributes
// (e.g. container.id, datadog.container.tag.container_id) the semantic convention takes
// precedence.
func ContainerTagsFromResourceAttributes(attrs pcommon.Map) map[string]string {
	ddtags := make(map[string]string)
	attrs.Range(func(key string, value pcommon.Value) bool {
		// Semantic Conventions
		if datadogKey, found := ContainerMappings[key]; found && value.Str() != "" {
			ddtags[datadogKey] = value.Str()
		}
		// Custom (datadog.container.tag namespace)
		if strings.HasPrefix(key, customContainerTagPrefix) {
			customKey := strings.TrimPrefix(key, customContainerTagPrefix)
			if customKey != "" && value.Str() != "" {
				// Do not replace if set via semantic conventions mappings.
				if _, found := ddtags[customKey]; !found {
					ddtags[customKey] = value.Str()
				}
			}
		}
		return true
	})
	return ddtags
}

// ContainerTagFromAttributes extracts the value of _dd.tags.container from the given
// set of attributes.
// Deprecated: Deprecated in favor of ContainerTagFromResourceAttributes.
func ContainerTagFromAttributes(attr map[string]string) map[string]string {
	ddtags := make(map[string]string)
	for key, val := range attr {
		datadogKey, found := ContainerMappings[key]
		if !found {
			continue
		}
		ddtags[datadogKey] = val
	}
	return ddtags
}
