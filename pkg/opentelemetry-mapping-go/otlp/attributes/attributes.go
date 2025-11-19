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
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv1_12 "go.opentelemetry.io/otel/semconv/v1.12.0"
	semconv1_17 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv1_27 "go.opentelemetry.io/otel/semconv/v1.27.0"
	semconv1_6_1 "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
)

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

	// HTTPMappings defines the mapping between OpenTelemetry semantic conventions
	// and Datadog Agent conventions for HTTP attributes.
	HTTPMappings = map[string]string{
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

// TagsFromAttributes converts a selected list of attributes
// to a tag list that can be added to metrics.
func TagsFromAttributes(attrs pcommon.Map) []string {
	tags := make([]string, 0, attrs.Len())

	var processAttributes processAttributes
	var systemAttributes systemAttributes

	attrs.Range(func(key string, value pcommon.Value) bool {
		switch key {
		// Process attributes
		case string(semconv1_27.ProcessExecutableNameKey):
			processAttributes.ExecutableName = value.Str()
		case string(semconv1_27.ProcessExecutablePathKey):
			processAttributes.ExecutablePath = value.Str()
		case string(semconv1_27.ProcessCommandKey):
			processAttributes.Command = value.Str()
		case string(semconv1_27.ProcessCommandLineKey):
			processAttributes.CommandLine = value.Str()
		case string(semconv1_27.ProcessPIDKey):
			processAttributes.PID = value.Int()
		case string(semconv1_27.ProcessOwnerKey):
			processAttributes.Owner = value.Str()

		// System attributes
		case string(semconv1_27.OSTypeKey):
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
	if containerID, ok := attrs.Get(string(semconv1_6_1.ContainerIDKey)); ok {
		originID = "container_id://" + containerID.AsString()
	} else if podUID, ok := attrs.Get(string(semconv1_6_1.K8SPodUIDKey)); ok {
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

// GetOTelAttrVal returns the matched value as a string in the input map with the given keys.
// If there are multiple keys present, the first matched one is returned.
// If normalize is true, normalize the return value with NormalizeTagValue.
func GetOTelAttrVal(attrs pcommon.Map, normalize bool, keys ...string) string {
	val := ""
	for _, key := range keys {
		attrval, exists := attrs.Get(key)
		if exists {
			val = attrval.AsString()
			break
		}
	}

	if normalize {
		val = normalizeutil.NormalizeTagValue(val)
	}

	return val
}

// GetOTelAttrFromEitherMap returns the matched value as a string in either attribute map with the given keys.
// If there are multiple keys present, the first matched one is returned.
// If the key is present in both maps, map1 takes precedence.
// If normalize is true, normalize the return value with NormalizeTagValue.
func GetOTelAttrFromEitherMap(map1 pcommon.Map, map2 pcommon.Map, normalize bool, keys ...string) string {
	if val := GetOTelAttrVal(map1, normalize, keys...); val != "" {
		return val
	}
	return GetOTelAttrVal(map2, normalize, keys...)
}

// GetOperationName returns the DD operation name based on OTel signal attributes.
func GetOperationName(
	signalAttrs pcommon.Map,
	spanKind ptrace.SpanKind,
) string {
	if operationName := GetOTelAttrVal(signalAttrs, true, "operation.name"); operationName != "" {
		return operationName
	}

	isClient := spanKind == ptrace.SpanKindClient
	isServer := spanKind == ptrace.SpanKindServer

	// http
	if method := GetOTelAttrVal(signalAttrs, false, string(semconv1_27.HTTPRequestMethodKey), string(semconv1_12.HTTPMethodKey)); method != "" {
		if isServer {
			return "http.server.request"
		}
		if isClient {
			return "http.client.request"
		}
	}

	// database
	if v := GetOTelAttrVal(signalAttrs, true, string(semconv1_12.DBSystemKey)); v != "" && isClient {
		return v + ".query"
	}

	// messaging
	system := GetOTelAttrVal(signalAttrs, true, string(semconv1_12.MessagingSystemKey))
	op := GetOTelAttrVal(signalAttrs, true, string(semconv1_12.MessagingOperationKey))
	if system != "" && op != "" {
		switch spanKind {
		case ptrace.SpanKindClient, ptrace.SpanKindServer, ptrace.SpanKindConsumer, ptrace.SpanKindProducer:
			return system + "." + op
		}
	}

	// RPC & AWS
	rpcValue := GetOTelAttrVal(signalAttrs, true, string(semconv1_12.RPCSystemKey))
	isRPC := rpcValue != ""
	isAws := isRPC && (rpcValue == "aws-api")
	// AWS client
	if isAws && isClient {
		if service := GetOTelAttrVal(signalAttrs, true, string(semconv1_12.RPCServiceKey)); service != "" {
			return "aws." + service + ".request"
		}
		return "aws.client.request"
	}

	// RPC client
	if isRPC && isClient {
		return rpcValue + ".client.request"
	}
	// RPC server
	if isRPC && isServer {
		return rpcValue + ".server.request"
	}

	// FAAS client
	provider := GetOTelAttrVal(signalAttrs, true, string(semconv1_12.FaaSInvokedProviderKey))
	invokedName := GetOTelAttrVal(signalAttrs, true, string(semconv1_12.FaaSInvokedNameKey))
	if provider != "" && invokedName != "" && isClient {
		return provider + "." + invokedName + ".invoke"
	}

	// FAAS server
	trigger := GetOTelAttrVal(signalAttrs, true, string(semconv1_12.FaaSTriggerKey))
	if trigger != "" && isServer {
		return trigger + ".invoke"
	}

	// GraphQL
	if GetOTelAttrVal(signalAttrs, true, "graphql.operation.type") != "" {
		return "graphql.server.request"
	}

	// if nothing matches, checking for generic http server/client
	protocol := GetOTelAttrVal(signalAttrs, true, "network.protocol.name")
	if isServer {
		if protocol != "" {
			return protocol + ".server.request"
		}
		return "server.request"
	} else if isClient {
		if protocol != "" {
			return protocol + ".client.request"
		}
		return "client.request"
	}

	if spanKind != ptrace.SpanKindUnspecified {
		return spanKind.String()
	}
	return ptrace.SpanKindInternal.String()
}

// GetResourceName returns the DD resource name based on OTel signal attributes.
func GetResourceName(signalAttrs pcommon.Map, spanKind ptrace.SpanKind, fallbackName string) (resName string) {
	defer func() {
		if len(resName) > normalizeutil.MaxResourceLen {
			resName = resName[:normalizeutil.MaxResourceLen]
		}
	}()
	if m := GetOTelAttrVal(signalAttrs, false, "resource.name"); m != "" {
		resName = m
		return
	}

	if m := GetOTelAttrVal(signalAttrs, false, string(semconv1_27.HTTPRequestMethodKey), string(semconv1_12.HTTPMethodKey)); m != "" {
		if m == "_OTHER" {
			m = "HTTP"
		}
		// use the HTTP method + route (if available)
		resName = m
		switch spanKind {
		case ptrace.SpanKindServer:
			if route := GetOTelAttrVal(signalAttrs, false, string(semconv1_27.HTTPRouteKey)); route != "" {
				resName = resName + " " + route
			}
		case ptrace.SpanKindClient:
			if template := GetOTelAttrVal(signalAttrs, false, string(semconv1_27.URLTemplateKey)); template != "" {
				resName = resName + " " + template
			}
		}
		return
	}

	if m := GetOTelAttrVal(signalAttrs, false, string(semconv1_12.MessagingOperationKey)); m != "" {
		resName = m
		// use the messaging operation
		if dest := GetOTelAttrVal(signalAttrs, false, string(semconv1_12.MessagingDestinationKey), string(semconv1_17.MessagingDestinationNameKey)); dest != "" {
			resName = resName + " " + dest
		}
		return
	}

	if m := GetOTelAttrVal(signalAttrs, false, string(semconv1_12.RPCMethodKey)); m != "" {
		resName = m
		// use the RPC method
		if svc := GetOTelAttrVal(signalAttrs, false, string(semconv1_12.RPCServiceKey)); svc != "" {
			// ...and service if available
			resName = resName + " " + svc
		}
		return
	}

	// Enrich GraphQL query resource names.
	// See https://github.com/open-telemetry/semantic-conventions/blob/v1.29.0/docs/graphql/graphql-spans.md
	if m := GetOTelAttrVal(signalAttrs, false, string(semconv1_17.GraphqlOperationTypeKey)); m != "" {
		resName = m
		if name := GetOTelAttrVal(signalAttrs, false, string(semconv1_17.GraphqlOperationNameKey)); name != "" {
			resName = resName + " " + name
		}
		return
	}

	if m := GetOTelAttrVal(signalAttrs, false, string(semconv1_12.DBSystemKey)); m != "" {
		// Since traces are obfuscated by span.Resource in pkg/trace/agent/obfuscate.go, we should use span.Resource as the resource name.
		// https://github.com/DataDog/datadog-agent/blob/62619a69cff9863f5b17215847b853681e36ff15/pkg/trace/agent/obfuscate.go#L32
		if dbStatement := GetOTelAttrVal(signalAttrs, false, string(semconv1_12.DBStatementKey)); dbStatement != "" {
			resName = dbStatement
			return
		}
		if dbQuery := GetOTelAttrVal(signalAttrs, false, string(semconv1_27.DBQueryTextKey)); dbQuery != "" {
			resName = dbQuery
			return
		}
	}

	resName = fallbackName
	return
}

// GetService returns the DD service name based on OTel resource attributes.
func GetService(resourceAttrs pcommon.Map, normalize bool) string {
	// No need to normalize with NormalizeTagValue since we will do NormalizeService later
	svc := GetOTelAttrVal(resourceAttrs, false, string(semconv1_12.ServiceNameKey))
	if svc == "" {
		svc = DefaultServiceName
	}
	if normalize {
		newsvc, err := normalizeutil.NormalizeService(svc, "")
		switch err {
		case normalizeutil.ErrTooLong:
			log.Debugf("Fixing malformed trace. Service is too long (reason:service_truncate), truncating span.service to length=%d: %s", normalizeutil.MaxServiceLen, svc)
		case normalizeutil.ErrInvalid:
			log.Debugf("Fixing malformed trace. Service is invalid (reason:service_invalid), replacing invalid span.service=%s with fallback span.service=%s", svc, newsvc)
		}
		svc = newsvc
	}
	return svc
}

// GetEnv returns the environment based on OTel resource attributes.
func GetEnv(resourceAttrs pcommon.Map) string {
	if env := GetOTelAttrVal(resourceAttrs, true, string(semconv1_27.DeploymentEnvironmentNameKey), string(semconv1_12.DeploymentEnvironmentKey)); env != "" {
		return env
	}
	return DefaultEnvName
}

// GetSpanType returns the DD span type based on OTel span kind and attributes.
func GetSpanType(spanKind ptrace.SpanKind, signalattrs pcommon.Map) string {
	if typ := GetOTelAttrVal(signalattrs, false, "span.type"); typ != "" {
		return typ
	}

	switch spanKind {
	case ptrace.SpanKindServer:
		return "web"
	case ptrace.SpanKindClient:
		db := GetOTelAttrVal(signalattrs, true, string(semconv1_6_1.DBSystemKey))
		if db == "" {
			return "http"
		}
		return getDBSpanType(db)
	default:
		return "custom"
	}
}

// getDBSpanType checks if the dbType is a known db type and returns the corresponding span.Type (from agent)
func getDBSpanType(dbType string) string {
	spanType, ok := DBTypes[dbType]
	if ok {
		return spanType
	}
	// span type not found, return generic db type
	return SpanTypeDB
}

// GetVersion returns the version based on OTel resource attributes
func GetVersion(resourceAttrs pcommon.Map) string {
	if version := GetOTelAttrVal(resourceAttrs, true, string(semconv1_12.ServiceVersionKey)); version != "" {
		return version
	}
	return ""
}

// GetStatusCode returns the HTTP status code based on OTel signal attributes.
func GetStatusCode(signalAttrs pcommon.Map) uint32 {
	if code, ok := signalAttrs.Get(string(semconv1_17.HTTPStatusCodeKey)); ok {
		if code.Type() != pcommon.ValueTypeInt {
			return 0
		}
		return uint32(code.Int())
	}
	if code, ok := signalAttrs.Get(string(semconv1_27.HTTPResponseStatusCodeKey)); ok {
		if code.Type() != pcommon.ValueTypeInt {
			return 0
		}
		return uint32(code.Int())
	}
	return 0
}

// GetContainerID returns the container ID based on OTel resource attributes.
func GetContainerID(resourceAttrs pcommon.Map) string {
	if cid := GetOTelAttrVal(resourceAttrs, true, string(semconv1_27.ContainerIDKey)); cid != "" {
		return cid
	}
	return ""
}

// GetSpecifiedKeysFromOTelAttributes returns a subset of OTel signal and resource attributes, with signal-level taking precedence.
// e.g. Useful for extracting peer tags
func GetSpecifiedKeysFromOTelAttributes(signalAttrs pcommon.Map, resourceAttrs pcommon.Map, keysToReturn map[string]struct{}) []string {
	if keysToReturn == nil {
		return []string{}
	}
	peerTagsMap := make(map[string]string, len(keysToReturn))

	cb := func(k string, v pcommon.Value) bool {
		val := v.AsString()
		if _, ok := keysToReturn[k]; ok {
			peerTagsMap[k] = val
		}
		return true
	}

	// Signal overwrites res
	resourceAttrs.Range(cb)
	signalAttrs.Range(cb)

	peerTags := make([]string, 0, len(peerTagsMap))
	for k, v := range peerTagsMap {
		t := normalizeutil.NormalizeTag(k + ":" + v)
		peerTags = append(peerTags, t)
	}
	return peerTags
}

// GetHost returns the DD hostname based on OTel resource attributes.
func GetHost(resourceAttrs pcommon.Map, fallbackHost string) string {
	src, srcok := SourceFromAttrs(resourceAttrs, nil)
	if !srcok {
		if v := GetOTelAttrVal(resourceAttrs, false, "_dd.hostname"); v != "" {
			src = source.Source{Kind: source.HostnameKind, Identifier: v}
			srcok = true
		}
	}
	if srcok {
		switch src.Kind {
		case source.HostnameKind:
			return src.Identifier
		default:
			// We are not on a hostname (serverless), hence the hostname is empty
			return ""
		}
	}
	return fallbackHost
}
