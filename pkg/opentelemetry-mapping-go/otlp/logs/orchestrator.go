// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/twmb/murmur3"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	orchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
)

var (
	k8sTypeMap map[string]int
)

func init() {
	// Map Kubernetes resource types to orchestrator manifest types
	k8sTypeMap = make(map[string]int)
	for _, t := range orchestratormodel.NodeTypes() {
		k8sTypeMap[t.String()] = int(t)
	}
}

// ToManifest converts a log record from k8sobjectsreceiver to an orchestrator manifest.
// The receiver supports two modes:
//   - Pull mode: k8s object is directly in the log body as JSON
//   - Watch mode: log body contains an "object" field with the k8s resource, and a "type" field for event type
//
// Returns the manifest and a boolean indicating if it's from a watch event.
func ToManifest(logRecord plog.LogRecord) (*agentmodel.Manifest, bool, error) {
	// Try to parse the body to detect the mode
	var bodyMap map[string]interface{}
	if err := json.Unmarshal([]byte(logRecord.Body().AsString()), &bodyMap); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal log body: %w", err)
	}

	// Check if this is a watch log (body contains "object" field)
	if objectField, hasObject := bodyMap["object"]; hasObject {
		// Watch log: body has structure like {"object": {...}, "type": "ADDED"}
		manifest, err := watchLogToManifest(objectField, bodyMap)
		return manifest, true, err
	}

	// Pull log: body directly contains the k8s object
	manifest, err := pullLogToManifestFromMap(bodyMap)
	return manifest, false, err
}

// watchLogToManifest handles logs from k8sobjectsreceiver in watch mode.
// Structure of watch mode logs - Body is a JSON string containing:
//
//	{
//	  "object": {...k8s resource...},
//	  "type": "ADDED" | "MODIFIED" | "DELETED"
//	}
func watchLogToManifest(objectField interface{}, bodyMap map[string]interface{}) (*agentmodel.Manifest, error) {
	// Convert the object field to a k8s resource map
	k8sResource, ok := objectField.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("object field in body is not a map, got type: %T", objectField)
	}

	// Extract event type from body
	eventType := ""
	if typeField, hasType := bodyMap["type"]; hasType {
		if typeStr, ok := typeField.(string); ok {
			eventType = typeStr
		}
	}

	// Reuse common logic to build manifest
	// Event types from k8s watch: ADDED, MODIFIED, DELETED
	isTerminated := eventType == "DELETED"

	return BuildManifestFromK8sResource(k8sResource, isTerminated)
}

// pullLogToManifestFromMap handles logs from k8sobjectsreceiver in pull mode.
// Structure of pull mode logs:
//   - Body: JSON string containing the k8s resource directly (e.g., {"apiVersion":"v1","kind":"Pod",...})
//   - Attributes: May contain additional metadata from the receiver
func pullLogToManifestFromMap(k8sResource map[string]interface{}) (*agentmodel.Manifest, error) {
	// Body is already parsed, just pass it to common logic
	// Not a delete event in pull mode, not from watch
	return BuildManifestFromK8sResource(k8sResource, false)
}

// BuildManifestFromK8sResource is the shared logic to convert a k8s resource map to a manifest.
// This function is used by both watchLogToManifest and pullLogToManifest to ensure consistent
// manifest creation regardless of the source mode.
//
// Parameters:
//   - k8sResource: The Kubernetes resource as a map (already unmarshaled from JSON)
//   - resource: OTLP resource containing additional metadata
//   - logRecord: OTLP log record for extracting tags and attributes
//   - isTerminated: true if this represents a deleted resource (watch mode only)
func BuildManifestFromK8sResource(k8sResource map[string]interface{}, isTerminated bool) (*agentmodel.Manifest, error) {
	// Check if the resource kind should be skipped for security reasons
	kind, _ := k8sResource["kind"].(string)
	group, _ := k8sResource["apiVersion"].(string)
	if shouldSkipResourceKind(kind, group) {
		return nil, fmt.Errorf("skipping unsupported resource kind: %s (contains sensitive data)", kind)
	}

	// Extract metadata
	metadata, ok := k8sResource["metadata"].(map[string]interface{})
	if !ok || metadata == nil {
		return nil, errors.New("k8s resource missing metadata")
	}

	uid, _ := metadata["uid"].(string)
	if uid == "" {
		return nil, errors.New("k8s resource missing uid in metadata")
	}

	resourceVersion, _ := metadata["resourceVersion"].(string)
	apiVersion, _ := k8sResource["apiVersion"].(string)

	// Convert the Kubernetes resource to JSON bytes for the manifest content
	content, err := json.Marshal(k8sResource)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal k8s resource content: %w", err)
	}

	// Determine manifest type based on the Kubernetes resource kind
	manifestType := getManifestType(kind)

	// Build tags from resource and log record attributes
	tags := buildCommonTags()

	// Create the manifest
	manifest := &agentmodel.Manifest{
		Type:            int32(manifestType),
		ResourceVersion: resourceVersion,
		Uid:             uid,
		Content:         content,
		ContentType:     "application/json",
		Version:         "v1",
		Tags:            tags,
		IsTerminated:    isTerminated,
		ApiVersion:      apiVersion,
		Kind:            kind,
	}
	return manifest, nil
}

// ToManifestPayload creates a CollectorManifest payload from a list of manifests.
func ToManifestPayload(manifests []*agentmodel.Manifest, hostName, clusterName, clusterID string) *agentmodel.CollectorManifest {
	return &agentmodel.CollectorManifest{
		ClusterName:     clusterName,
		ClusterId:       clusterID,
		HostName:        hostName,
		Manifests:       manifests,
		Tags:            buildCommonTags(),
		OriginCollector: agentmodel.OriginCollector_datadogExporter,
	}
}

// CreateClusterManifest creates a Cluster manifest to be sent after all nodes have been collected.
// This is used to trigger cluster-level processing in the backend after node collection is complete.
func CreateClusterManifest(clusterID string, nodes []*agentmodel.Manifest, logger *zap.Logger) *agentmodel.Manifest {
	// Initialize aggregated data structures
	kubeletVersions := make(map[string]int32)
	var cpuAllocatable uint64
	var cpuCapacity uint64
	var memoryAllocatable uint64
	var memoryCapacity uint64
	var podAllocatable uint32
	var podCapacity uint32
	extendedResourcesCapacity := make(map[string]int64)
	extendedResourcesAllocatable := make(map[string]int64)

	// Extended resources blacklist (standard resources that shouldn't be counted as extended)
	extendedResourcesBlacklist := map[string]struct{}{
		"cpu":    {},
		"memory": {},
		"pods":   {},
	}

	nodeCount := int32(len(nodes))
	nodesInfo := make([]*agentmodel.ClusterNodeInfo, 0, len(nodes))

	// Parse and aggregate information from each node
	for _, nodeManifest := range nodes {
		// Unmarshal the node JSON content
		var nodeMap map[string]interface{}
		if err := json.Unmarshal(nodeManifest.Content, &nodeMap); err != nil {
			logger.Warn("Failed to unmarshal node content", zap.Error(err), zap.String("uid", nodeManifest.Uid))
			continue
		}

		// Extract status information
		status, ok := nodeMap["status"].(map[string]interface{})
		if !ok {
			logger.Warn("Node missing status field", zap.String("uid", nodeManifest.Uid))
			continue
		}

		// Extract node info
		nodeInfo, ok := status["nodeInfo"].(map[string]interface{})
		if ok {
			if kubeletVersion, ok := nodeInfo["kubeletVersion"].(string); ok && kubeletVersion != "" {
				kubeletVersions[kubeletVersion]++
			}
		}

		// Extract allocatable resources
		if allocatable, ok := status["allocatable"].(map[string]interface{}); ok {
			// CPU allocatable (in millicores)
			if cpuStr, ok := allocatable["cpu"].(string); ok {
				if cpu := parseQuantity(cpuStr, true); cpu > 0 {
					cpuAllocatable += cpu
				}
			}
			// Memory allocatable (in bytes)
			if memStr, ok := allocatable["memory"].(string); ok {
				if mem := parseQuantity(memStr, false); mem > 0 {
					memoryAllocatable += mem
				}
			}
			// Pods allocatable
			if podsStr, ok := allocatable["pods"].(string); ok {
				if pods := parseQuantity(podsStr, false); pods > 0 {
					podAllocatable += uint32(pods)
				}
			}
			// Extended resources allocatable
			for name, value := range allocatable {
				if _, isBlacklisted := extendedResourcesBlacklist[name]; !isBlacklisted {
					if valStr, ok := value.(string); ok {
						if qty := parseQuantity(valStr, false); qty > 0 {
							extendedResourcesAllocatable[name] += int64(qty)
						}
					}
				}
			}
		}

		// Extract capacity resources
		if capacity, ok := status["capacity"].(map[string]interface{}); ok {
			// CPU capacity (in millicores)
			if cpuStr, ok := capacity["cpu"].(string); ok {
				if cpu := parseQuantity(cpuStr, true); cpu > 0 {
					cpuCapacity += cpu
				}
			}
			// Memory capacity (in bytes)
			if memStr, ok := capacity["memory"].(string); ok {
				if mem := parseQuantity(memStr, false); mem > 0 {
					memoryCapacity += mem
				}
			}
			// Pods capacity
			if podsStr, ok := capacity["pods"].(string); ok {
				if pods := parseQuantity(podsStr, false); pods > 0 {
					podCapacity += uint32(pods)
				}
			}
			// Extended resources capacity
			for name, value := range capacity {
				if _, isBlacklisted := extendedResourcesBlacklist[name]; !isBlacklisted {
					if valStr, ok := value.(string); ok {
						if qty := parseQuantity(valStr, false); qty > 0 {
							extendedResourcesCapacity[name] += int64(qty)
						}
					}
				}
			}
		}

		// Extract node info for ClusterNodeInfo
		nodeInfoModel := extractClusterNodeInfo(nodeMap)
		if nodeInfoModel != nil {
			nodesInfo = append(nodesInfo, nodeInfoModel)
		}
	}

	clusterModel := &agentmodel.Cluster{
		CpuAllocatable:               cpuAllocatable,
		CpuCapacity:                  cpuCapacity,
		KubeletVersions:              kubeletVersions,
		MemoryAllocatable:            memoryAllocatable,
		MemoryCapacity:               memoryCapacity,
		NodeCount:                    nodeCount,
		PodAllocatable:               podAllocatable,
		PodCapacity:                  podCapacity,
		ExtendedResourcesCapacity:    extendedResourcesCapacity,
		ExtendedResourcesAllocatable: extendedResourcesAllocatable,
		NodesInfo:                    nodesInfo,
	}

	content, err := json.Marshal(clusterModel)
	if err != nil {
		logger.Error("Failed to marshal cluster manifest", zap.Error(err))
		return nil
	}

	version := murmur3.Sum64(content)

	return &agentmodel.Manifest{
		Type:            int32(getManifestType("Cluster")),
		ResourceVersion: strconv.FormatUint(version, 10),
		Uid:             clusterID,
		Content:         content,
		ContentType:     "application/json",
		Version:         "v1",
		Tags:            buildCommonTags(),
		IsTerminated:    false,
		ApiVersion:      "virtual.datadoghq.com/v1",
		Kind:            "Cluster",
	}
}

func getManifestType(kind string) int {
	if _, ok := k8sTypeMap[kind]; ok {
		return k8sTypeMap[kind]
	}

	return int(orchestratormodel.K8sUnsetType)
}

func buildCommonTags() []string {
	return []string{
		"otel_receiver:k8sobjectsreceiver",
	}
}

// shouldSkipResourceKind returns true if the Kubernetes resource kind should be skipped
// for security or data sensitivity reasons. This matches the behavior of the orchestrator
// collector which skips secrets and configmaps as they can contain sensitive data.
func shouldSkipResourceKind(kind string, group string) bool {
	if group == "v1" && (strings.ToLower(kind) == "secret" || strings.ToLower(kind) == "configmap") {
		return true
	}

	return false
}

// parseQuantity parses Kubernetes resource quantity strings (e.g., "4", "1000m", "2Gi")
// and returns the value in the appropriate unit:
// - For CPU (asMillis=true): returns milliCPU (e.g., "1" -> 1000, "500m" -> 500)
// - For other resources (asMillis=false): returns the raw value in base unit
func parseQuantity(quantityStr string, asMillis bool) uint64 {
	if quantityStr == "" {
		return 0
	}

	// Handle CPU millicores (e.g., "500m")
	if strings.HasSuffix(quantityStr, "m") {
		valueStr := strings.TrimSuffix(quantityStr, "m")
		if value, err := strconv.ParseUint(valueStr, 10, 64); err == nil {
			return value
		}
		return 0
	}

	// Handle binary suffixes (Ki, Mi, Gi, Ti, Pi, Ei)
	binarySuffixes := map[string]uint64{
		"Ki": 1024,
		"Mi": 1024 * 1024,
		"Gi": 1024 * 1024 * 1024,
		"Ti": 1024 * 1024 * 1024 * 1024,
		"Pi": 1024 * 1024 * 1024 * 1024 * 1024,
		"Ei": 1024 * 1024 * 1024 * 1024 * 1024 * 1024,
	}

	for suffix, multiplier := range binarySuffixes {
		if strings.HasSuffix(quantityStr, suffix) {
			valueStr := strings.TrimSuffix(quantityStr, suffix)
			if value, err := strconv.ParseUint(valueStr, 10, 64); err == nil {
				return value * multiplier
			}
			return 0
		}
	}

	// Handle decimal suffixes (k, M, G, T, P, E)
	decimalSuffixes := map[string]uint64{
		"k": 1000,
		"M": 1000 * 1000,
		"G": 1000 * 1000 * 1000,
		"T": 1000 * 1000 * 1000 * 1000,
		"P": 1000 * 1000 * 1000 * 1000 * 1000,
		"E": 1000 * 1000 * 1000 * 1000 * 1000 * 1000,
	}

	for suffix, multiplier := range decimalSuffixes {
		if strings.HasSuffix(quantityStr, suffix) {
			valueStr := strings.TrimSuffix(quantityStr, suffix)
			if value, err := strconv.ParseUint(valueStr, 10, 64); err == nil {
				return value * multiplier
			}
			return 0
		}
	}

	// Plain number without suffix
	if value, err := strconv.ParseUint(quantityStr, 10, 64); err == nil {
		// If this is a CPU value and we need millis, multiply by 1000
		if asMillis {
			return value * 1000
		}
		return value
	}

	return 0
}

// extractClusterNodeInfo extracts node information from a Kubernetes node resource
// and returns a ClusterNodeInfo model for inclusion in the cluster model.
func extractClusterNodeInfo(nodeMap map[string]interface{}) *agentmodel.ClusterNodeInfo {
	nodeInfo := &agentmodel.ClusterNodeInfo{
		ResourceAllocatable: make(map[string]string),
		ResourceCapacity:    make(map[string]string),
	}

	// Extract metadata
	metadata, ok := nodeMap["metadata"].(map[string]interface{})
	if !ok {
		return nil
	}

	if name, ok := metadata["name"].(string); ok {
		nodeInfo.Name = name
	}

	// Extract labels for instance type and region
	if labels, ok := metadata["labels"].(map[string]interface{}); ok {
		// Instance type
		if instanceType, ok := labels["node.kubernetes.io/instance-type"].(string); ok {
			nodeInfo.InstanceType = instanceType
		} else if instanceType, ok := labels["beta.kubernetes.io/instance-type"].(string); ok {
			nodeInfo.InstanceType = instanceType
		}

		// Region
		if region, ok := labels["topology.kubernetes.io/region"].(string); ok {
			nodeInfo.Region = region
		} else if region, ok := labels["failure-domain.beta.kubernetes.io/region"].(string); ok {
			nodeInfo.Region = region
		}
	}

	// Extract status
	status, ok := nodeMap["status"].(map[string]interface{})
	if !ok {
		return nodeInfo
	}

	// Extract node info (kubelet version, OS, architecture, etc.)
	if statusNodeInfo, ok := status["nodeInfo"].(map[string]interface{}); ok {
		if architecture, ok := statusNodeInfo["architecture"].(string); ok {
			nodeInfo.Architecture = architecture
		}
		if containerRuntimeVersion, ok := statusNodeInfo["containerRuntimeVersion"].(string); ok {
			nodeInfo.ContainerRuntimeVersion = containerRuntimeVersion
		}
		if kernelVersion, ok := statusNodeInfo["kernelVersion"].(string); ok {
			nodeInfo.KernelVersion = kernelVersion
		}
		if kubeletVersion, ok := statusNodeInfo["kubeletVersion"].(string); ok {
			nodeInfo.KubeletVersion = kubeletVersion
		}
		if operatingSystem, ok := statusNodeInfo["operatingSystem"].(string); ok {
			nodeInfo.OperatingSystem = operatingSystem
		}
		if osImage, ok := statusNodeInfo["osImage"].(string); ok {
			nodeInfo.OperatingSystemImage = osImage
		}
	}

	// Extract allocatable resources
	if allocatable, ok := status["allocatable"].(map[string]interface{}); ok {
		for resourceName, resourceValue := range allocatable {
			if valueStr, ok := resourceValue.(string); ok {
				nodeInfo.ResourceAllocatable[resourceName] = valueStr
			}
		}
	}

	// Extract capacity resources
	if capacity, ok := status["capacity"].(map[string]interface{}); ok {
		for resourceName, resourceValue := range capacity {
			if valueStr, ok := resourceValue.(string); ok {
				nodeInfo.ResourceCapacity[resourceName] = valueStr
			}
		}
	}

	return nodeInfo
}
