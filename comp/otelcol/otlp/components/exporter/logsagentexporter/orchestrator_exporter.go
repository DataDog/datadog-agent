// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/twmb/murmur3"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	orchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// manifestCacheTTL is the time-to-live for manifest cache entries (3 minutes, same as orchestrator collector)
	manifestCacheTTL = 3 * time.Minute
	// manifestCachePurge is the interval for purging expired cache entries
	manifestCachePurge = 30 * time.Second
	// maxManifestsPerPayload is the maximum number of manifests to send in a single payload
	maxManifestsPerPayload = 100
	// maxPayloadSizeBytes is the maximum serialized size of manifest content per payload (10 MB, same as orchestrator default)
	maxPayloadSizeBytes = 10 * 1000 * 1000
)

var (
	// manifestCache provides an in-memory cache to avoid sending the same manifest multiple times
	// within a short period. Uses UID + resourceVersion as the cache key.
	manifestCache     *gocache.Cache
	manifestCacheOnce sync.Once
	k8sTypeMap        map[string]int
)

func init() {
	// Map Kubernetes resource types to orchestrator manifest types
	k8sTypeMap = make(map[string]int)
	for _, t := range orchestratormodel.NodeTypes() {
		k8sTypeMap[t.String()] = int(t)
	}
}

// getManifestCache returns the singleton manifest cache instance
func getManifestCache() *gocache.Cache {
	manifestCacheOnce.Do(func() {
		manifestCache = gocache.New(manifestCacheTTL, manifestCachePurge)
	})
	return manifestCache
}

// shouldSkipManifest checks if the manifest was already sent recently.
// Returns true if the manifest should be skipped (cache hit with same resourceVersion).
// This follows the same pattern as pkg/orchestrator.SkipKubernetesResource.
// Watch log events always bypass the cache to ensure real-time updates are sent.
func shouldSkipManifest(manifest *agentmodel.Manifest, isWatchEvent bool) bool {
	if manifest == nil || manifest.Uid == "" {
		return false
	}

	// Watch events should always bypass the cache to ensure real-time updates
	if isWatchEvent {
		return false
	}

	cache := getManifestCache()
	cacheKey := manifest.Uid

	// Check if we have this resource in cache
	value, hit := cache.Get(cacheKey)

	if !hit {
		// Cache miss - this is a new resource, add it to cache
		cache.Set(cacheKey, manifest.ResourceVersion, manifestCacheTTL)
		return false
	}

	// Cache hit - check if the resourceVersion changed
	cachedVersion, ok := value.(string)
	if !ok || cachedVersion != manifest.ResourceVersion {
		// ResourceVersion changed - update cache and don't skip
		cache.Set(cacheKey, manifest.ResourceVersion, manifestCacheTTL)
		return false
	}

	// Cache hit with same resourceVersion - skip this manifest
	return true
}

// chunkManifestsBySizeAndWeight chunks manifests based on both count and serialized size
// to avoid intake endpoint rejections. This follows the same logic as the orchestrator collector.
func chunkManifestsBySizeAndWeight(manifests []*agentmodel.Manifest, maxChunkSize, maxChunkWeight int) [][]*agentmodel.Manifest {
	if len(manifests) == 0 {
		return make([][]*agentmodel.Manifest, 0)
	}

	// Convert to interface{} for the chunking utility
	interfaceManifests := make([]interface{}, 0, len(manifests))
	for _, m := range manifests {
		interfaceManifests = append(interfaceManifests, m)
	}

	chunker := &util.ChunkAllocator[[]interface{}, interface{}]{
		AppendToChunk: func(chunk *[]interface{}, payloads []interface{}) {
			*chunk = append(*chunk, payloads...)
		},
	}

	list := &util.PayloadList[interface{}]{
		Items: interfaceManifests,
		WeightAt: func(i int) int {
			// Use the serialized manifest content size as the weight
			return len(manifests[i].Content)
		},
	}

	util.ChunkPayloadsBySizeAndWeight[[]interface{}, interface{}](list, chunker, maxChunkSize, maxChunkWeight)

	// Convert back to typed chunks
	chunks := *chunker.GetChunks()
	result := make([][]*agentmodel.Manifest, len(chunks))
	for i, chunk := range chunks {
		result[i] = make([]*agentmodel.Manifest, len(chunk))
		for j, item := range chunk {
			result[i][j] = item.(*agentmodel.Manifest)
		}
	}
	return result
}

func (e *Exporter) consumeK8sObjects(ctx context.Context, ld plog.Logs) (err error) {
	var manifests []*agentmodel.Manifest

	var nodes []*agentmodel.Manifest

	var isWatchEvent bool

	var clusterID string
	var clusterName string

	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		resourceLogs := ld.ResourceLogs().At(i)
		resource := resourceLogs.Resource()

		for j := 0; j < resourceLogs.ScopeLogs().Len(); j++ {
			scopeLogs := resourceLogs.ScopeLogs().At(j)

			for k := 0; k < scopeLogs.LogRecords().Len(); k++ {
				logRecord := scopeLogs.LogRecords().At(k)

				k8sClusterID, ok := resource.Attributes().Get("k8s.cluster.uid")
				if ok {
					clusterID = k8sClusterID.AsString()
				} else {
					e.set.Logger.Error("Failed to get cluster ID, skipping manifest payload", zap.Error(err))
					continue
				}

				k8sClusterName, ok := resource.Attributes().Get("k8s.cluster.name")
				if ok {
					clusterName = k8sClusterName.AsString()
				} else {
					e.set.Logger.Error("Failed to get cluster name, skipping manifest payload", zap.Error(err))
					continue
				}

				// Convert Kubernetes resource manifest to orchestrator payload format
				var manifest *agentmodel.Manifest
				manifest, isWatchEvent, err = toManifest(logRecord, resource)
				if err != nil {
					e.set.Logger.Error("Failed to convert to manifest: "+err.Error(), zap.Error(err))
					continue
				}

				if manifest.Kind == "Node" {
					nodes = append(nodes, manifest)
				}

				// Check cache to avoid sending the same manifest multiple times within 3 minutes
				// Watch events bypass the cache to ensure real-time updates
				if shouldSkipManifest(manifest, isWatchEvent) {
					e.set.Logger.Debug("Skipping manifest (cache hit)",
						zap.String("uid", manifest.Uid),
						zap.String("kind", manifest.Kind),
						zap.String("resourceVersion", manifest.ResourceVersion))
					continue
				}

				e.set.Logger.Debug("Sending manifest",
					zap.String("uid", manifest.Uid),
					zap.String("kind", manifest.Kind),
					zap.String("resourceVersion", manifest.ResourceVersion))

				manifests = append(manifests, manifest)
			}
		}
	}

	// Send a Cluster manifest once all nodes have been collected
	if len(nodes) > 0 && !isWatchEvent {
		e.set.Logger.Debug("Creating Cluster manifest after collecting nodes", zap.Int("total_nodes", len(nodes)))
		clusterManifest := createClusterManifest(clusterID, nodes, e.set.Logger)

		// Check cache for the cluster manifest too (not a watch event)
		if !shouldSkipManifest(clusterManifest, false) {
			manifests = append(manifests, clusterManifest)
			e.set.Logger.Debug("Added Cluster manifest to payload",
				zap.String("uid", clusterManifest.Uid),
				zap.Int("total_nodes", len(nodes)))
		}
	}

	hostname, err := e.orchestratorConfig.Hostname.Get(ctx)
	if err != nil || hostname == "" {
		e.set.Logger.Error("Failed to get hostname from config", zap.Error(err))
	}

	// Chunk manifests by both count and serialized size to avoid intake endpoint rejections.
	// This follows the same logic as the orchestrator collector, ensuring payloads respect
	// both the maximum number of manifests (100) and the maximum payload size (10 MB).
	totalManifests := len(manifests)
	chunks := chunkManifestsBySizeAndWeight(manifests, maxManifestsPerPayload, maxPayloadSizeBytes)

	e.set.Logger.Debug("Sending manifests in chunks",
		zap.Int("total_manifests", totalManifests),
		zap.Int("chunk_count", len(chunks)),
		zap.Int("max_manifests_per_chunk", maxManifestsPerPayload),
		zap.Int("max_payload_size_bytes", maxPayloadSizeBytes))

	for i, chunk := range chunks {
		e.set.Logger.Debug("Sending manifest chunk",
			zap.Int("chunk_index", i),
			zap.Int("chunk_size", len(chunk)))

		payload := toManifestPayload(chunk, hostname, clusterName, clusterID, e.set.Logger)

		if err := sendManifestPayload(ctx, e.orchestratorConfig.Endpoint, e.orchestratorConfig.Key, payload, hostname, clusterID, e.set.Logger); err != nil {
			e.set.Logger.Error("Failed to send collector manifest chunk",
				zap.Int("chunk_index", i),
				zap.Int("chunk_size", len(chunk)),
				zap.Error(err))
		}
	}

	return nil
}

// toManifest converts a log record from k8sobjectsreceiver to an orchestrator manifest.
// The receiver supports two modes:
//   - Pull mode: k8s object is directly in the log body as JSON
//   - Watch mode: log body contains an "object" field with the k8s resource, and a "type" field for event type
//
// Returns the manifest and a boolean indicating if it's from a watch event.
func toManifest(logRecord plog.LogRecord, resource pcommon.Resource) (*agentmodel.Manifest, bool, error) {
	// Try to parse the body to detect the mode
	var bodyMap map[string]interface{}
	if err := json.Unmarshal([]byte(logRecord.Body().AsString()), &bodyMap); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal log body: %w", err)
	}

	// Check if this is a watch log (body contains "object" field)
	if objectField, hasObject := bodyMap["object"]; hasObject {
		// Watch log: body has structure like {"object": {...}, "type": "ADDED"}
		manifest, err := watchLogToManifest(objectField, bodyMap, logRecord, resource)
		return manifest, true, err
	}

	// Pull log: body directly contains the k8s object
	manifest, err := pullLogToManifestFromMap(bodyMap, logRecord, resource)
	return manifest, false, err
}

// watchLogToManifest handles logs from k8sobjectsreceiver in watch mode.
// Structure of watch mode logs - Body is a JSON string containing:
//
//	{
//	  "object": {...k8s resource...},
//	  "type": "ADDED" | "MODIFIED" | "DELETED"
//	}
func watchLogToManifest(objectField interface{}, bodyMap map[string]interface{}, logRecord plog.LogRecord, resource pcommon.Resource) (*agentmodel.Manifest, error) {
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

	return buildManifestFromK8sResource(k8sResource, resource, logRecord, isTerminated)
}

// pullLogToManifestFromMap handles logs from k8sobjectsreceiver in pull mode.
// Structure of pull mode logs:
//   - Body: JSON string containing the k8s resource directly (e.g., {"apiVersion":"v1","kind":"Pod",...})
//   - Attributes: May contain additional metadata from the receiver
func pullLogToManifestFromMap(k8sResource map[string]interface{}, logRecord plog.LogRecord, resource pcommon.Resource) (*agentmodel.Manifest, error) {
	// Body is already parsed, just pass it to common logic
	// Not a delete event in pull mode, not from watch
	return buildManifestFromK8sResource(k8sResource, resource, logRecord, false)
}

// buildManifestFromK8sResource is the shared logic to convert a k8s resource map to a manifest.
// This function is used by both watchLogToManifest and pullLogToManifest to ensure consistent
// manifest creation regardless of the source mode.
//
// Parameters:
//   - k8sResource: The Kubernetes resource as a map (already unmarshaled from JSON)
//   - resource: OTLP resource containing additional metadata
//   - logRecord: OTLP log record for extracting tags and attributes
//   - isTerminated: true if this represents a deleted resource (watch mode only)
//   - isWatchEvent: true if this event comes from watch mode (vs pull mode)
func buildManifestFromK8sResource(k8sResource map[string]interface{}, resource pcommon.Resource, logRecord plog.LogRecord, isTerminated bool) (*agentmodel.Manifest, error) {
	// Check if the resource kind should be skipped for security reasons
	kind, _ := k8sResource["kind"].(string)
	group, _ := k8sResource["apiVersion"].(string)
	if shouldSkipResourceKind(kind, group) {
		return nil, fmt.Errorf("skipping unsupported resource kind: %s (contains sensitive data)", kind)
	}

	// Extract metadata
	metadata, ok := k8sResource["metadata"].(map[string]interface{})
	if !ok || metadata == nil {
		return nil, fmt.Errorf("k8s resource missing metadata")
	}

	uid, _ := metadata["uid"].(string)
	if uid == "" {
		return nil, fmt.Errorf("k8s resource missing uid in metadata")
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
	tags := buildTags(resource, logRecord)

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

func toManifestPayload(manifests []*agentmodel.Manifest, hostName, clusterName, clusterID string, logger *zap.Logger) *agentmodel.CollectorManifest {
	version, err := version.Agent()
	if err != nil {
		logger.Error("Failed to get agent version", zap.Error(err))
		return nil
	}
	return &agentmodel.CollectorManifest{
		ClusterName:     clusterName,
		ClusterId:       clusterID,
		HostName:        hostName,
		Manifests:       manifests,
		Tags:            buildCommonTags(),
		OriginCollector: agentmodel.OriginCollector_datadogExporter,
		AgentVersion: &agentmodel.AgentVersion{
			Major:  version.Major,
			Minor:  version.Minor,
			Patch:  version.Patch,
			Pre:    version.Pre,
			Meta:   version.Meta,
			Commit: version.Commit,
		},
	}
}

// createClusterManifest creates a Cluster manifest to be sent after all nodes have been collected.
// This is used to trigger cluster-level processing in the backend after node collection is complete.
func createClusterManifest(clusterID string, nodes []*agentmodel.Manifest, logger *zap.Logger) *agentmodel.Manifest {
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
		ResourceVersion: fmt.Sprint(version),
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

func sendManifestPayload(ctx context.Context, endpoint, apiKey string, payload *agentmodel.CollectorManifest, hostName, clusterID string, logger *zap.Logger) error {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Get the agent version
	agentVersion := "1.0.0" // Default fallback
	av, err := version.Agent()
	if err == nil {
		agentVersion = av.GetNumberAndPre()
	} else {
		logger.Warn("Failed to get agent version, using default", zap.Error(err))
	}

	encoded, err := agentmodel.EncodeMessage(agentmodel.Message{
		Header: agentmodel.MessageHeader{
			Version:  agentmodel.MessageV3,
			Encoding: agentmodel.MessageEncodingZstdPBxNoCgo,
			Type:     agentmodel.TypeCollectorManifest,
		}, Body: payload})
	if err != nil {
		return fmt.Errorf("failed to encode payload: %w", err)
	}

	// Create the request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("X-Dd-Hostname", hostName)
	req.Header.Set("X-DD-Agent-Timestamp", strconv.Itoa(int(time.Now().Unix())))
	req.Header.Set("X-Dd-Orchestrator-ClusterID", clusterID)
	req.Header.Set("DD-EVP-ORIGIN", "agent")
	req.Header.Set("DD-EVP-ORIGIN-VERSION", agentVersion)

	// Send the request
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("orchestrator endpoint returned non-200 status: %d", resp.StatusCode)
	}

	return nil
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

func buildTags(resource pcommon.Resource, logRecord plog.LogRecord) []string {
	tags := buildCommonTags()

	// Add resource attributes as tags
	resource.Attributes().Range(func(key string, value pcommon.Value) bool {
		tags = append(tags, fmt.Sprintf("%s:%s", strings.ReplaceAll(key, ".", "_"), value.AsString()))
		return true
	})

	// Add log record attributes as tags
	logRecord.Attributes().Range(func(key string, value pcommon.Value) bool {
		tags = append(tags, fmt.Sprintf("%s:%s", strings.ReplaceAll(key, ".", "_"), value.AsString()))
		return true
	})

	return tags
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
