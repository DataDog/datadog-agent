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
	semconv1_27 "go.opentelemetry.io/otel/semconv/v1.27.0"
	semconv1_6_1 "go.opentelemetry.io/otel/semconv/v1.6.1"
)

var DefaultOTLPServiceName = "otlpresourcenoservicename"
var DefaultOTLPEnvironmentName = "default"

// customContainerTagPrefix defines the prefix for custom container tags.
const customContainerTagPrefix = "datadog.container.tag."

var (
	// coreMapping defines the mapping between OpenTelemetry semantic conventions
	// and Datadog Agent conventions for env, service and version.
	coreMapping = map[string]string{
		// Datadog conventions
		// https://docs.datadoghq.com/getting_started/tagging/unified_service_tagging/
		string(semconv1_6_1.DeploymentEnvironmentKey):    "env",
		string(semconv1_27.ServiceNameKey):               "service",
		string(semconv1_27.ServiceVersionKey):            "version",
		string(semconv1_27.DeploymentEnvironmentNameKey): "env",
	}

	// ContainerMappings defines the mapping between OpenTelemetry semantic conventions
	// and Datadog Agent conventions for containers.
	ContainerMappings = map[string]string{
		// Containers
		string(semconv1_27.ContainerIDKey):        "container_id",
		string(semconv1_27.ContainerNameKey):      "container_name",
		string(semconv1_27.ContainerImageNameKey): "image_name",
		string(semconv1_6_1.ContainerImageTagKey): "image_tag",
		string(semconv1_27.ContainerRuntimeKey):   "runtime",

		// Cloud conventions
		// https://www.datadoghq.com/blog/tagging-best-practices/
		string(semconv1_27.CloudProviderKey):         "cloud_provider",
		string(semconv1_27.CloudRegionKey):           "region",
		string(semconv1_27.CloudAvailabilityZoneKey): "zone",

		// ECS conventions
		// https://github.com/DataDog/datadog-agent/blob/e081bed/pkg/tagger/collectors/ecs_extract.go
		string(semconv1_27.AWSECSTaskFamilyKey):   "task_family",
		string(semconv1_27.AWSECSTaskARNKey):      "task_arn",
		string(semconv1_27.AWSECSClusterARNKey):   "ecs_cluster_name",
		string(semconv1_27.AWSECSTaskRevisionKey): "task_version",
		string(semconv1_27.AWSECSContainerARNKey): "ecs_container_name",

		// Kubernetes resource name (via semantic conventions)
		// https://github.com/DataDog/datadog-agent/blob/e081bed/pkg/util/kubernetes/const.go
		string(semconv1_27.K8SContainerNameKey):   "kube_container_name",
		string(semconv1_27.K8SClusterNameKey):     "kube_cluster_name",
		string(semconv1_27.K8SDeploymentNameKey):  "kube_deployment",
		string(semconv1_27.K8SReplicaSetNameKey):  "kube_replica_set",
		string(semconv1_27.K8SStatefulSetNameKey): "kube_stateful_set",
		string(semconv1_27.K8SDaemonSetNameKey):   "kube_daemon_set",
		string(semconv1_27.K8SJobNameKey):         "kube_job",
		string(semconv1_27.K8SCronJobNameKey):     "kube_cronjob",
		string(semconv1_27.K8SNamespaceNameKey):   "kube_namespace",
		string(semconv1_27.K8SPodNameKey):         "pod_name",
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

	// TODO make the values consts (APM convention keys)
	// KeyHTTPClientIP = "http.client_ip"
	// HTTPKeyMappings defines the mapping between OpenTelemetry semantic conventions
	// and Datadog Agent conventions for HTTP attributes.
	HTTPKeyMappings = map[string]string{
		string(semconv1_27.ClientAddressKey):          "http.client_ip",
		string(semconv1_27.HTTPResponseBodySizeKey):   "http.response.content_length",
		string(semconv1_27.HTTPResponseStatusCodeKey): "http.status_code",
		string(semconv1_27.HTTPRequestBodySizeKey):    "http.request.content_length",
		"http.request.header.referrer":                "http.referrer",
		string(semconv1_27.HTTPRequestMethodKey):      "http.method",
		string(semconv1_27.HTTPRouteKey):              "http.route",
		string(semconv1_27.NetworkProtocolVersionKey): "http.version",
		string(semconv1_27.ServerAddressKey):          "http.server_name",
		string(semconv1_27.URLFullKey):                "http.url",
		string(semconv1_27.UserAgentOriginalKey):      "http.useragent",
	}
)

// ======== Datadog namespace constants ========

// Global singleton instance
var DDNamespaceKeys = newNamespace(
	"datadog.",
	"env",
	"service",
	"operation_name",
	"resource_name",
	"span.type",
	"span.kind",
	"",
	"",
	"",
	"http_status_code",
	"host",
	"container_id",
	"container_tags",
	"version",
	"db.name",
)

var APMConventionKeys = newNamespace(
	"",
	"env",
	"service.name",
	"operation.name",
	"resource.name",
	"span.type",
	"span.kind",
	"error.msg",
	"error.type",
	"error.stack",
	"http.status_code",
	"host",
	"container_id",
	"container_tags",
	"version",
	"db.name",
)

func newNamespace(
	prefix string,
	envKey string,
	serviceKey string,
	operationNameKey string,
	resourceNameKey string,
	spanTypeKey string,
	spanKindKey string,
	errorMsgKey string,
	errorTypeKey string,
	errorStackKey string,
	httpStatusCodeKey string,
	hostKey string,
	containerIDKey string,
	containerTagsKey string,
	versionKey string,
	dbNameKey string,
) namespace {
	return namespace{
		prefix:            prefix,
		envKey:            envKey,
		serviceKey:        serviceKey,
		operationNameKey:  operationNameKey,
		resourceNameKey:   resourceNameKey,
		spanTypeKey:       spanTypeKey,
		spanKindKey:       spanKindKey,
		errorMsgKey:       errorMsgKey,
		errorTypeKey:      errorTypeKey,
		errorStackKey:     errorStackKey,
		httpStatusCodeKey: httpStatusCodeKey,
		hostKey:           hostKey,
		containerIDKey:    containerIDKey,
		containerTagsKey:  containerTagsKey,
		versionKey:        versionKey,
		dbNameKey:         dbNameKey,
		// Precompute all full keys
		envFull:            prefix + envKey,
		serviceFull:        prefix + serviceKey,
		operationNameFull:  prefix + operationNameKey,
		resourceNameFull:   prefix + resourceNameKey,
		spanTypeFull:       prefix + spanTypeKey,
		spanKindFull:       prefix + spanKindKey,
		errorMsgFull:       prefix + errorMsgKey,
		errorTypeFull:      prefix + errorTypeKey,
		errorStackFull:     prefix + errorStackKey,
		httpStatusCodeFull: prefix + httpStatusCodeKey,
		hostFull:           prefix + hostKey,
		containerIDFull:    prefix + containerIDKey,
		containerTagsFull:  prefix + containerTagsKey,
		versionFull:        prefix + versionKey,
		dbNameFull:         prefix + dbNameKey,
	}
}

type namespace struct {
	prefix            string
	envKey            string
	serviceKey        string
	operationNameKey  string
	resourceNameKey   string
	spanTypeKey       string
	spanKindKey       string
	errorMsgKey       string
	errorTypeKey      string
	errorStackKey     string
	httpStatusCodeKey string
	hostKey           string
	containerIDKey    string
	containerTagsKey  string
	versionKey        string
	dbNameKey         string
	// Precomputed full keys (prefix + key)
	envFull            string
	serviceFull        string
	operationNameFull  string
	resourceNameFull   string
	spanTypeFull       string
	spanKindFull       string
	errorMsgFull       string
	errorTypeFull      string
	errorStackFull     string
	httpStatusCodeFull string
	hostFull           string
	containerIDFull    string
	containerTagsFull  string
	versionFull        string
	dbNameFull         string
}

func (ns namespace) Prefix() string {
	return ns.prefix
}

func (ns namespace) Env() string {
	return ns.envFull
}

func (ns namespace) Service() string {
	return ns.serviceFull
}

func (ns namespace) OperationName() string {
	return ns.operationNameFull
}

func (ns namespace) ResourceName() string {
	return ns.resourceNameFull
}

func (ns namespace) SpanType() string {
	return ns.spanTypeFull
}

func (ns namespace) SpanKind() string {
	return ns.spanKindFull
}

func (ns namespace) HTTPStatusCode() string {
	return ns.httpStatusCodeFull
}

func (ns namespace) Host() string {
	return ns.hostFull
}

func (ns namespace) ContainerID() string {
	return ns.containerIDFull
}

func (ns namespace) ContainerTags() string {
	return ns.containerTagsFull
}

func (ns namespace) Version() string {
	return ns.versionFull
}

func (ns namespace) DBName() string {
	return ns.dbNameFull
}

func (ns namespace) ErrorMsg() string {
	return ns.errorMsgFull
}

func (ns namespace) ErrorType() string {
	return ns.errorTypeFull
}

func (ns namespace) ErrorStack() string {
	return ns.errorStackFull
}
