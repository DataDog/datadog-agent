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
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
	"github.com/gogo/protobuf/proto"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv1_12 "go.opentelemetry.io/otel/semconv/v1.12.0"
	semconv1_17 "go.opentelemetry.io/otel/semconv/v1.17.0"
	semconv1_27 "go.opentelemetry.io/otel/semconv/v1.27.0"
	semconv1_6_1 "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	normalizeutil "github.com/DataDog/datadog-agent/pkg/util/normalize"
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

	// KeyDatadogService is the key for the service name in the Datadog namespace
	KeyDatadogService = "datadog.service"
	// KeyDatadogResource is the key for the resource name in the Datadog namespace
	KeyDatadogResource = "datadog.resource"
	// KeyDatadogType is the key for the span type in the Datadog namespace
	KeyDatadogType = "datadog.type"
	// KeyDatadogVersion is the key for the version in the Datadog namespace
	KeyDatadogVersion = "datadog.version"
	// KeyDatadogHTTPStatusCode is the key for the HTTP status code in the Datadog namespace
	KeyDatadogHTTPStatusCode = "datadog.http_status_code"
	// KeyDatadogHost is the key for the host in the Datadog namespace
	KeyDatadogHost = "datadog.host"
	// KeyDatadogEnvironment is the key for the environment in the Datadog namespace
	KeyDatadogEnvironment = "datadog.env"
	// KeyDatadogContainerID is the key for the container ID in the Datadog namespace
	KeyDatadogContainerID = "datadog.container_id"
	// DefaultOTLPServiceName is the default service name for OTel spans when no service name is found in the resource attributes.
	DefaultOTLPServiceName = "otlpresourcenoservicename"
	// DefaultOTLPEnvironmentName is the default environment name for OTel spans when no environment name is found in the resource attributes.
	DefaultOTLPEnvironmentName = "default"
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

// getTimeUnitScaleToNanos returns the scaling factor to convert the given unit to nanoseconds
func getTimeUnitScaleToNanos(unit string) float64 {
	switch unit {
	case "ns":
		return float64(time.Nanosecond)
	case "us", "μs":
		return float64(time.Microsecond)
	case "ms":
		return float64(time.Millisecond)
	case "s":
		return float64(time.Second)
	case "min":
		return float64(time.Minute)
	case "h":
		return float64(time.Hour)
	default:
		// If unit is unknown, assume seconds (common for duration metrics)
		return float64(time.Second)
	}
}

// getBounds returns the lower and upper bounds for a histogram bucket
func getBounds(explicitBounds pcommon.Float64Slice, idx int) (lowerBound float64, upperBound float64) {
	// See https://github.com/open-telemetry/opentelemetry-proto/blob/v0.10.0/opentelemetry/proto/metrics/v1/metrics.proto#L427-L439
	lowerBound = math.Inf(-1)
	upperBound = math.Inf(1)
	if idx > 0 {
		lowerBound = explicitBounds.At(idx - 1)
	}
	if idx < explicitBounds.Len() {
		upperBound = explicitBounds.At(idx)
	}
	return
}

// createDDSketchFromHistogram creates a DDSketch from regular histogram data point
func createDDSketchFromHistogram(dp pmetric.HistogramDataPoint, unit string) ([]byte, error) {
	relativeAccuracy := 0.01 // 1% relative accuracy
	maxNumBins := 2048
	newSketch, err := ddsketch.LogCollapsingLowestDenseDDSketch(relativeAccuracy, maxNumBins)
	if err != nil {
		return nil, err
	}

	bucketCounts := dp.BucketCounts()
	explicitBounds := dp.ExplicitBounds()

	// Get scaling factor to convert unit to nanoseconds
	scaleToNanos := getTimeUnitScaleToNanos(unit)

	// Find first and last bucket indices with count > 0
	lowestBucketIndex := -1
	highestBucketIndex := -1
	for j := 0; j < bucketCounts.Len(); j++ {
		count := bucketCounts.At(j)
		if count > 0 {
			if lowestBucketIndex == -1 {
				lowestBucketIndex = j
			}
			highestBucketIndex = j
		}
	}

	hasMin := dp.HasMin()
	hasMax := dp.HasMax()
	minNanoseconds := dp.Min() * scaleToNanos
	maxNanoseconds := dp.Max() * scaleToNanos

	for j := 0; j < bucketCounts.Len(); j++ {
		lowerBound, upperBound := getBounds(explicitBounds, j)

		if math.IsInf(upperBound, 1) {
			upperBound = lowerBound
		} else if math.IsInf(lowerBound, -1) {
			lowerBound = upperBound
		}

		count := bucketCounts.At(j)

		if count > 0 {
			insertionPoint := 0.0
			adjustedCount := float64(count)
			midpoint := (lowerBound + upperBound) / 2 * scaleToNanos
			// Determine insertion point based on bucket position
			if j == lowestBucketIndex && j == highestBucketIndex {
				// Special case: min and max are in the same bucket
				if hasMin && hasMax {
					insertionPoint = (minNanoseconds + maxNanoseconds) / 2
				}
			} else if j == lowestBucketIndex {
				// Bottom bucket: insert at min value
				if hasMin {
					insertionPoint = minNanoseconds
				}
			} else if j == highestBucketIndex {
				// Top bucket: insert at max value
				if hasMax {
					insertionPoint = maxNanoseconds
				}
			}

			if insertionPoint == 0.0 {
				insertionPoint = midpoint
			}

			newSketch.AddWithCount(insertionPoint, adjustedCount)
		}
	}

	// Marshal sketch to protobuf bytes
	return proto.Marshal(newSketch.ToProto())
}

// createDDSketchFromExponentialHistogram creates a DDSketch from exponential histogram data point
func createDDSketchFromExponentialHistogram(dp pmetric.ExponentialHistogramDataPoint, unit string) ([]byte, error) {
	// Get scaling factor to convert unit to nanoseconds
	scaleToNanos := getTimeUnitScaleToNanos(unit)

	// Create the DDSketch stores with unit scaling
	positiveStore := toStoreFromExponentialBucketsWithUnitScale(dp.Positive(), dp.Scale(), scaleToNanos)
	negativeStore := toStoreFromExponentialBucketsWithUnitScale(dp.Negative(), dp.Scale(), scaleToNanos)

	// Create the DDSketch mapping for nanoseconds (no offset needed since we scaled the stores)
	gamma := math.Pow(2, math.Pow(2, float64(-dp.Scale())))
	mapping, err := mapping.NewLogarithmicMappingWithGamma(gamma, 0)
	if err != nil {
		return nil, fmt.Errorf("couldn't create LogarithmicMapping for DDSketch: %w", err)
	}

	// Create DDSketch with the above mapping and stores
	sketch := ddsketch.NewDDSketch(mapping, positiveStore, negativeStore)
	// Zero count represents values at exactly zero, so we add at zero (no scaling needed for zero)
	err = sketch.AddWithCount(0, float64(dp.ZeroCount()))
	if err != nil {
		return nil, fmt.Errorf("failed to add ZeroCount to DDSketch: %w", err)
	}

	// Marshal sketch to protobuf bytes
	return proto.Marshal(sketch.ToProto())
}

// toStoreFromExponentialBucketsWithUnitScale converts exponential histogram buckets to store with unit scaling
func toStoreFromExponentialBucketsWithUnitScale(b pmetric.ExponentialHistogramDataPointBuckets, scale int32, scaleToNanos float64) store.Store {
	offset := b.Offset()
	bucketCounts := b.BucketCounts()

	// Calculate the base for the exponential histogram
	base := math.Pow(2, math.Pow(2, float64(-scale)))

	store := store.NewDenseStore()
	for j := 0; j < bucketCounts.Len(); j++ {
		bucketIndex := j + int(offset)
		count := bucketCounts.At(j)

		if count > 0 {
			// Calculate the actual bucket boundary value
			bucketValue := math.Pow(base, float64(bucketIndex))

			// Scale the bucket value to nanoseconds
			scaledValue := bucketValue * scaleToNanos

			// Convert back to the index in the nanosecond space
			// Using the same gamma since we're keeping the same precision
			scaledIndex := int(math.Log(scaledValue) / math.Log(base))

			store.AddWithCount(scaledIndex, float64(count))
		}
	}
	return store
}

// Agent-style attribute extraction functions

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

// GetOTelHostname returns the DD hostname based on OTel span and resource attributes, with span taking precedence.
func GetOTelHostname(signalattrs pcommon.Map, resattrs pcommon.Map, fallbackHost string, hostFromAttributesHandler HostFromAttributesHandler, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool) string {
	if useDatadogNamespaceIfPresent {
		host := GetOTelAttrFromEitherMap(signalattrs, resattrs, true, KeyDatadogHost)
		if host != "" {
			return host
		}
	}
	if ignoreMissingDatadogFields {
		return ""
	}
	// Try to get source from resource attributes using translator logic
	src, srcok := SourceFromAttrs(resattrs, hostFromAttributesHandler)
	if !srcok {
		if v := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, "_dd.hostname"); v != "" {
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
	} else {
		// fallback hostname from Agent conf.Hostname
		return fallbackHost
	}
}

// GetOTelEnv returns the environment based on OTel span and resource attributes, with span taking precedence.
func GetOTelEnv(signalattr pcommon.Map, resattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool) (env string) {
	if useDatadogNamespaceIfPresent {
		env = GetOTelAttrFromEitherMap(signalattr, resattrs, true, KeyDatadogEnvironment)
		if env != "" && !ignoreMissingDatadogFields {
			return env
		}
	}
	if !ignoreMissingDatadogFields {
		env = GetOTelAttrFromEitherMap(signalattr, resattrs, true, string(semconv1_27.DeploymentEnvironmentNameKey), string(semconv1_12.DeploymentEnvironmentKey))
	}
	if env == "" {
		return DefaultOTLPEnvironmentName
	}
	return env
}

// GetOTelService returns the DD service name based on OTel span and resource attributes.
func GetOTelService(signalattrs pcommon.Map, resattrs pcommon.Map, normalize bool, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool) string {
	if useDatadogNamespaceIfPresent {
		svc := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, KeyDatadogService)
		if svc != "" {
			return svc
		}
	}
	if ignoreMissingDatadogFields {
		return ""
	}
	svc := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, string(semconv1_6_1.ServiceNameKey))
	if svc == "" {
		svc = DefaultOTLPServiceName
	}
	if normalize {
		newsvc, err := normalizeutil.NormalizeService(svc, "")
		switch err {
		case normalizeutil.ErrTooLong:
			log.Debugf("Fixing malformed trace. Service is too long (reason:service_truncate), truncating span.service to length=%d: %s", normalizeutil.MaxServiceLen, svc) // XXX use a different logger
		case normalizeutil.ErrInvalid:
			log.Debugf("Fixing malformed trace. Service is invalid (reason:service_invalid), replacing invalid span.service=%s with fallback span.service=%s", svc, newsvc) // XXX use a different logger
		}
		svc = newsvc
	}
	return svc
}

// GetOTelResource returns the DD resource name based on OTel span and resource attributes.
func GetOTelResource(spanKind ptrace.SpanKind, signalattrs pcommon.Map, resattrs pcommon.Map, fallbackResource string, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool) (resource string) {
	if useDatadogNamespaceIfPresent {
		resource = GetOTelAttrFromEitherMap(signalattrs, resattrs, false, KeyDatadogResource)
		if resource != "" {
			return resource
		}
	}
	if ignoreMissingDatadogFields {
		return fallbackResource
	}
	if m := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, "resource.name"); m != "" {
		return m
	}

	// HTTP method + route logic
	if method := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, "http.request.method", string(semconv1_12.HTTPMethodKey)); method != "" {
		if method == "_OTHER" {
			method = "HTTP"
		}
		resource = method
		if spanKind == ptrace.SpanKindServer {
			if route := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, string(semconv1_12.HTTPRouteKey)); route != "" {
				resource = resource + " " + route
			}
		}
		return resource
	}

	// Messaging operation logic
	if operation := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, string(semconv1_12.MessagingOperationKey)); operation != "" {
		resource = operation
		if dest := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, string(semconv1_12.MessagingDestinationKey), string(semconv1_17.MessagingDestinationNameKey)); dest != "" {
			resource = resource + " " + dest
		}
		return resource
	}

	// RPC method logic
	if method := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, string(semconv1_12.RPCMethodKey)); method != "" {
		resource = method
		if svc := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, string(semconv1_12.RPCServiceKey)); svc != "" {
			resource = resource + " " + svc
		}
		return resource
	}

	// GraphQL operation logic
	if opType := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, string(semconv1_17.GraphqlOperationTypeKey)); opType != "" {
		resource = opType
		if name := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, string(semconv1_17.GraphqlOperationNameKey)); name != "" {
			resource = resource + " " + name
		}
		return resource
	}

	// Database operation logic
	if dbSystem := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, string(semconv1_12.DBSystemKey)); dbSystem != "" {
		if statement := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, string(semconv1_12.DBStatementKey)); statement != "" {
			return statement
		}
		if dbQuery := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, string(semconv1_27.DBQueryTextKey)); dbQuery != "" {
			return dbQuery
		}
	}

	return fallbackResource
}

// GetOTelSpanType returns the DD span type based on OTel span kind and attributes.
func GetOTelSpanType(spanKind ptrace.SpanKind, signalattrs pcommon.Map, resattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool) string {
	if useDatadogNamespaceIfPresent {
		typ := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, KeyDatadogType)
		if typ != "" {
			return typ
		}
	}
	if ignoreMissingDatadogFields {
		return "custom"
	}
	typ := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, "span.type")
	if typ != "" {
		return typ
	}

	switch spanKind {
	case ptrace.SpanKindServer:
		return "web"
	case ptrace.SpanKindClient:
		db := GetOTelAttrFromEitherMap(signalattrs, resattrs, true, string(semconv1_6_1.DBSystemKey))
		if db == "" {
			typ = "http"
		} else {
			typ = checkDBType(db)
		}
	default:
		typ = "custom"
	}
	return typ
}

// Database span type constants (from agent)
const (
	spanTypeSQL           = "sql"
	spanTypeCassandra     = "cassandra"
	spanTypeRedis         = "redis"
	spanTypeMemcached     = "memcached"
	spanTypeMongoDB       = "mongodb"
	spanTypeElasticsearch = "elasticsearch"
	spanTypeOpenSearch    = "opensearch"
	spanTypeDB            = "db"
)

// dbTypes maps database systems to their corresponding span types (copied from agent)
var dbTypes = map[string]string{
	// SQL db types
	semconv1_12.DBSystemOtherSQL.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemMSSQL.Value.AsString():       spanTypeSQL,
	semconv1_12.DBSystemMySQL.Value.AsString():       spanTypeSQL,
	semconv1_12.DBSystemOracle.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemDB2.Value.AsString():         spanTypeSQL,
	semconv1_12.DBSystemPostgreSQL.Value.AsString():  spanTypeSQL,
	semconv1_12.DBSystemRedshift.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemCloudscape.Value.AsString():  spanTypeSQL,
	semconv1_12.DBSystemHSQLDB.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemMaxDB.Value.AsString():       spanTypeSQL,
	semconv1_12.DBSystemIngres.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemFirstSQL.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemEDB.Value.AsString():         spanTypeSQL,
	semconv1_12.DBSystemCache.Value.AsString():       spanTypeSQL,
	semconv1_12.DBSystemFirebird.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemDerby.Value.AsString():       spanTypeSQL,
	semconv1_12.DBSystemInformix.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemMariaDB.Value.AsString():     spanTypeSQL,
	semconv1_12.DBSystemSqlite.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemSybase.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemTeradata.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemVertica.Value.AsString():     spanTypeSQL,
	semconv1_12.DBSystemH2.Value.AsString():          spanTypeSQL,
	semconv1_12.DBSystemColdfusion.Value.AsString():  spanTypeSQL,
	semconv1_12.DBSystemCockroachdb.Value.AsString(): spanTypeSQL,
	semconv1_12.DBSystemProgress.Value.AsString():    spanTypeSQL,
	semconv1_12.DBSystemHanaDB.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemAdabas.Value.AsString():      spanTypeSQL,
	semconv1_12.DBSystemFilemaker.Value.AsString():   spanTypeSQL,
	semconv1_12.DBSystemInstantDB.Value.AsString():   spanTypeSQL,
	semconv1_12.DBSystemInterbase.Value.AsString():   spanTypeSQL,
	semconv1_12.DBSystemNetezza.Value.AsString():     spanTypeSQL,
	semconv1_12.DBSystemPervasive.Value.AsString():   spanTypeSQL,
	semconv1_12.DBSystemPointbase.Value.AsString():   spanTypeSQL,
	semconv1_17.DBSystemClickhouse.Value.AsString():  spanTypeSQL, // not in semconv 1.6.1

	// Cassandra db types
	semconv1_12.DBSystemCassandra.Value.AsString(): spanTypeCassandra,

	// Redis db types
	semconv1_12.DBSystemRedis.Value.AsString(): spanTypeRedis,

	// Memcached db types
	semconv1_12.DBSystemMemcached.Value.AsString(): spanTypeMemcached,

	// MongoDB db types
	semconv1_12.DBSystemMongoDB.Value.AsString(): spanTypeMongoDB,

	// Elasticsearch db types
	semconv1_12.DBSystemElasticsearch.Value.AsString(): spanTypeElasticsearch,

	// OpenSearch db types, not in semconv1_12 1.6.1
	semconv1_17.DBSystemOpensearch.Value.AsString(): spanTypeOpenSearch,

	// Generic db types
	semconv1_12.DBSystemHive.Value.AsString():      spanTypeDB,
	semconv1_12.DBSystemHBase.Value.AsString():     spanTypeDB,
	semconv1_12.DBSystemNeo4j.Value.AsString():     spanTypeDB,
	semconv1_12.DBSystemCouchbase.Value.AsString(): spanTypeDB,
	semconv1_12.DBSystemCouchDB.Value.AsString():   spanTypeDB,
	semconv1_12.DBSystemCosmosDB.Value.AsString():  spanTypeDB,
	semconv1_12.DBSystemDynamoDB.Value.AsString():  spanTypeDB,
	semconv1_12.DBSystemGeode.Value.AsString():     spanTypeDB,
}

// checkDBType checks if the dbType is a known db type and returns the corresponding span.Type (from agent)
func checkDBType(dbType string) string {
	spanType, ok := dbTypes[dbType]
	if ok {
		return spanType
	}
	// span type not found, return generic db type
	return spanTypeDB
}

// GetOTelVersion returns the version based on OTel span and resource attributes, with span taking precedence.
func GetOTelVersion(signalattrs pcommon.Map, resattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool) string {
	if useDatadogNamespaceIfPresent {
		version := GetOTelAttrFromEitherMap(signalattrs, resattrs, false, KeyDatadogVersion)
		if version != "" {
			return version
		}
	}
	if ignoreMissingDatadogFields {
		return ""
	}
	return GetOTelAttrFromEitherMap(signalattrs, resattrs, true, string(semconv1_27.ServiceVersionKey))
}

// GetOTelStatusCode returns the HTTP status code based on OTel span and resource attributes, with span taking precedence.
func GetOTelStatusCode(signalattrs pcommon.Map, resattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool) (uint32, error) {
	ret := _getOTelStatusCode(signalattrs, resattrs, ignoreMissingDatadogFields, useDatadogNamespaceIfPresent)
	switch ret.Type() {
	case pcommon.ValueTypeInt:
		return uint32(ret.Int()), nil
	case pcommon.ValueTypeDouble:
		return uint32(ret.Int()), nil
	case pcommon.ValueTypeStr:
		if code, err := strconv.ParseUint(ret.AsString(), 10, 32); err == nil {
			return uint32(code), nil
		} else {
			return 0, fmt.Errorf("invalid status code %s", ret.AsString())
		}
	default:
		return 0, fmt.Errorf("unsupported type %s", ret.Type())
	}
}

func _getOTelStatusCode(signalattrs pcommon.Map, resattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool) pcommon.Value {
	if useDatadogNamespaceIfPresent {
		if code, ok := signalattrs.Get(KeyDatadogHTTPStatusCode); ok {
			return code
		}
		if code, ok := resattrs.Get(KeyDatadogHTTPStatusCode); ok {
			return code
		}
	}
	if !ignoreMissingDatadogFields {
		if code, ok := signalattrs.Get(string(semconv1_17.HTTPStatusCodeKey)); ok {
			return code
		}
		if code, ok := signalattrs.Get(string(semconv1_27.HTTPResponseStatusCodeKey)); ok {
			return code
		}
		if code, ok := resattrs.Get(string(semconv1_17.HTTPStatusCodeKey)); ok {
			return code
		}
		if code, ok := resattrs.Get(string(semconv1_27.HTTPResponseStatusCodeKey)); ok {
			return code
		}
	}
	return pcommon.NewValueInt(0)
}

// GetOTelContainerID returns the container ID based on OTel span and resource attributes, with span taking precedence.
func GetOTelContainerID(signalattrs pcommon.Map, resourceattrs pcommon.Map, ignoreMissingDatadogFields bool, useDatadogNamespaceIfPresent bool) string {
	if useDatadogNamespaceIfPresent {
		cid := GetOTelAttrFromEitherMap(signalattrs, resourceattrs, true, KeyDatadogContainerID)
		if cid != "" {
			return cid
		}
	}
	if ignoreMissingDatadogFields {
		return ""
	}
	return GetOTelAttrFromEitherMap(signalattrs, resourceattrs, true, string(semconv1_27.ContainerIDKey), string(semconv1_27.K8SPodUIDKey))
}

// XXX make this take signalattrs as well
// GetOTelContainerTags returns a list of DD container tags in an OTel map's attributes.
// Tags are always normalized.
func GetOTelContainerTags(signalattrs pcommon.Map, rattrs pcommon.Map, tagKeys []string) []string {
	var containerTags []string
	containerTagsMap := ContainerTagsFromResourceAttributes(rattrs)
	for _, key := range tagKeys {
		if mappedKey, ok := ContainerMappings[key]; ok {
			// If the key has a mapping in ContainerMappings, use the mapped key
			if val, ok := containerTagsMap[mappedKey]; ok {
				t := normalizeutil.NormalizeTag(mappedKey + ":" + val)
				containerTags = append(containerTags, t)
			}
		} else {
			// Otherwise populate as additional container tags
			if val := GetOTelAttrVal(rattrs, false, key); val != "" {
				t := normalizeutil.NormalizeTag(key + ":" + val)
				containerTags = append(containerTags, t)
			}
		}
	}
	return containerTags
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

// normalizeTag normalizes a tag (simplified version of agent's normalizeutil.NormalizeTag)
func normalizeTag(tag string) string {
	// Basic validation - must contain colon
	if !strings.Contains(tag, ":") {
		return tag
	}

	// Replace invalid characters with underscores
	invalidChars := regexp.MustCompile(`[^\w.:/\-]`)
	tag = invalidChars.ReplaceAllString(tag, "_")

	// Trim and lowercase
	tag = strings.ToLower(strings.TrimSpace(tag))

	return tag
}

// GetPeerTagsFromOTelAttributes returns the peer tags based on OTel span and resource attributes, with span taking precedence.
func GetPeerTagsFromOTelAttributes(signalattrs pcommon.Map, resattrs pcommon.Map, peerTagKeys map[string]struct{}) []string {
	var peerTagsMap map[string]string

	cb := func(k string, v pcommon.Value) bool {
		val := v.AsString()
		if _, ok := peerTagKeys[k]; ok {
			peerTagsMap[k] = val
		}
		return true
	}

	// Signal overwrites res
	resattrs.Range(cb)
	signalattrs.Range(cb)

	peerTags := make([]string, 0, len(peerTagsMap))
	for k, v := range peerTagsMap {
		t := normalizeTag(k + ":" + v)
		peerTags = append(peerTags, t)
	}
	return peerTags
}
