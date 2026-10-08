// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	orchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/util"
)

const (
	manifestCacheTTL       = 3 * time.Minute
	manifestCachePurge     = 30 * time.Second
	maxManifestsPerPayload = 100
	MaxPayloadSizeBytes    = 10 * 1000 * 1000
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

// ToManifest converts resource/log records from k8sobjectsreceiver to an orchestrator manifest.
// The receiver supports two modes:
//   - Pull mode: k8s object is directly in the log body as JSON
//   - Watch mode: log body contains an "object" field with the k8s resource, and a "type" field for event type
//
// Log record attributes and optional resource attributes are preserved as manifest extra attributes.
// Returns the manifest and a boolean indicating if it's from a watch event.
func ToManifest(logRecord plog.LogRecord, resources ...pcommon.Resource) (*agentmodel.Manifest, bool, error) {
	// Try to parse the body to detect the mode
	var bodyMap map[string]interface{}
	if err := json.Unmarshal([]byte(logRecord.Body().AsString()), &bodyMap); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal log body: %w", err)
	}

	var manifest *agentmodel.Manifest
	var err error
	isWatch := false

	// Check if this is a watch log (body contains "object" field)
	if objectField, hasObject := bodyMap["object"]; hasObject {
		// Watch log: body has structure like {"object": {...}, "type": "ADDED"}
		manifest, err = watchLogToManifest(objectField, bodyMap)
		isWatch = true
	} else {
		// Pull log: body directly contains the k8s object
		manifest, err = pullLogToManifestFromMap(bodyMap)
	}
	if err != nil {
		return nil, isWatch, err
	}

	manifest.ExtraAttributes = manifestExtraAttributes(logRecord, resources...)

	return manifest, isWatch, nil
}

func manifestExtraAttributes(logRecord plog.LogRecord, resources ...pcommon.Resource) map[string]string {
	extraAttributesCapacity := logRecord.Attributes().Len()
	for _, resource := range resources {
		extraAttributesCapacity += resource.Attributes().Len()
	}
	if extraAttributesCapacity == 0 {
		return nil
	}

	extraAttributes := make(map[string]string, extraAttributesCapacity)
	for _, resource := range resources {
		addOTLPAttributes(extraAttributes, resource.Attributes())
	}
	addOTLPAttributes(extraAttributes, logRecord.Attributes())
	if len(extraAttributes) == 0 {
		return nil
	}
	return extraAttributes
}

func addOTLPAttributes(extraAttributes map[string]string, attributes pcommon.Map) {
	attributes.Range(func(k string, v pcommon.Value) bool {
		if k != "" {
			extraAttributes[k] = v.AsString()
		}
		return true
	})
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
	var nodeName string
	switch kind {
	case "Pod":
		if spec, ok := k8sResource["spec"].(map[string]interface{}); ok {
			if n, ok := spec["nodeName"].(string); ok {
				nodeName = n
			}
		}
	case "Node":
		if n, ok := metadata["name"].(string); ok {
			nodeName = n
		}
	default:
		nodeName = ""
	}

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
		NodeName:        nodeName,
	}
	return manifest, nil
}

// ToManifestPayload creates a CollectorManifest payload from a list of manifests.
func ToManifestPayload(manifests []*agentmodel.Manifest, hostName, clusterName, clusterID string, originCollector agentmodel.OriginCollector) *agentmodel.CollectorManifest {
	return &agentmodel.CollectorManifest{
		ClusterName:     clusterName,
		ClusterId:       clusterID,
		HostName:        hostName,
		Manifests:       manifests,
		Tags:            buildCommonTags(),
		OriginCollector: originCollector,
	}
}

// NewManifestCache creates a new manifest deduplication cache with the standard TTL and purge interval.
// Callers are responsible for managing the cache lifetime (e.g., as a singleton).
func NewManifestCache() *gocache.Cache {
	return gocache.New(manifestCacheTTL, manifestCachePurge)
}

// shouldSkipManifest reports whether the manifest should be suppressed because an identical
// (same UID + resourceVersion) manifest was already sent within the cache TTL.
// clusterID scopes the cache key so UIDs from different clusters never collide.
// Watch events always bypass the cache so real-time updates are never dropped.
// If cache is nil, deduplication is skipped.
func shouldSkipManifest(manifest *agentmodel.Manifest, clusterID string, isWatchEvent bool, cache *gocache.Cache) bool {
	if cache == nil || manifest == nil || manifest.Uid == "" {
		return false
	}

	// Watch events should always bypass the cache to ensure real-time updates
	if isWatchEvent {
		return false
	}

	cacheKey := clusterID + "/" + manifest.Uid

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
			return manifests[i].Size()
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

// K8sTranslationResult holds the output of TranslateK8sObjects.
type K8sTranslationResult struct {
	// Chunks holds manifests split into payload-sized groups ready to send.
	Chunks [][]*agentmodel.Manifest
	// ClusterName extracted from k8s.cluster.name resource attribute.
	ClusterName string
	// ClusterID extracted from k8s.cluster.uid resource attribute.
	ClusterID string
}

// TranslateK8sObjects converts k8sobjectsreceiver logs into chunked orchestrator manifest payloads, grouped by cluster identity.
// It handles deduplication via cache (pass nil to disable) and chunking.
// Set maxChunkSize to value > 0 to override individual chunk weight. Otherwise, default value will be used.
// Individual record errors are logged and skipped rather than aborting the batch.
func TranslateK8sObjects(ld plog.Logs, cache *gocache.Cache, logger *zap.Logger, maxChunkSize int) []*K8sTranslationResult {
	if maxChunkSize <= 0 {
		maxChunkSize = MaxPayloadSizeBytes
	}

	groups, names, order := groupResourceLogsByCluster(ld, logger)
	if len(order) == 0 {
		return nil
	}

	results := make([]*K8sTranslationResult, 0, len(order))
	for _, id := range order {
		if result := translateClusterLogs(id, names[id], groups[id], cache, logger, maxChunkSize); result != nil {
			results = append(results, result)
		}
	}
	return results
}

// groupResourceLogsByCluster partitions ld into per-cluster buckets keyed by cluster UID.
// The first cluster name seen for a given UID is used; subsequent ResourceLogs with the same UID
// but a different name produce a Warn log.
// ResourceLogs missing k8s.cluster.uid or k8s.cluster.name are skipped with an error log.
func groupResourceLogsByCluster(ld plog.Logs, logger *zap.Logger) (groups map[string][]plog.ResourceLogs, names map[string]string, order []string) {
	groups = make(map[string][]plog.ResourceLogs)
	names = make(map[string]string)

	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		rl := ld.ResourceLogs().At(i)
		attrs := rl.Resource().Attributes()

		cid, ok := attrs.Get("k8s.cluster.uid")
		if !ok {
			logger.Error("Failed to get k8s cluster ID, skipping resource log")
			continue
		}
		cname, ok := attrs.Get("k8s.cluster.name")
		if !ok {
			logger.Error("Failed to get k8s cluster name, skipping resource log")
			continue
		}

		id := cid.AsString()
		if id == "" {
			logger.Error("k8s.cluster.uid is empty, skipping resource log")
			continue
		}
		name := cname.AsString()
		if name == "" {
			logger.Error("k8s.cluster.name is empty, skipping resource log")
			continue
		}
		if _, seen := groups[id]; !seen {
			order = append(order, id)
			names[id] = name
		} else if names[id] != name {
			logger.Warn("Conflicting cluster name for same cluster UID; using first name seen",
				zap.String("k8s.cluster.uid", id),
				zap.String("first_name", names[id]),
				zap.String("conflicting_name", name))
		}
		groups[id] = append(groups[id], rl)
	}
	return groups, names, order
}

// translateClusterLogs converts one cluster's ResourceLogs into a K8sTranslationResult.
func translateClusterLogs(clusterID, clusterName string, rls []plog.ResourceLogs, cache *gocache.Cache, logger *zap.Logger, maxChunkSize int) *K8sTranslationResult {
	var manifests []*agentmodel.Manifest

	for _, rl := range rls {
		resource := rl.Resource()
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			for k := 0; k < sl.LogRecords().Len(); k++ {
				lr := sl.LogRecords().At(k)
				manifest, isWatch, err := ToManifest(lr, resource)
				if err != nil {
					logger.Error("Failed to convert to manifest", zap.Error(err))
					continue
				}
				if shouldSkipManifest(manifest, clusterID, isWatch, cache) {
					logger.Debug("Skipping manifest (cache hit)",
						zap.String("uid", manifest.Uid),
						zap.String("kind", manifest.Kind),
						zap.String("resourceVersion", manifest.ResourceVersion))
					continue
				}
				logger.Debug("Sending manifest",
					zap.String("uid", manifest.Uid),
					zap.String("kind", manifest.Kind),
					zap.String("resourceVersion", manifest.ResourceVersion))
				manifests = append(manifests, manifest)
			}
		}
	}

	chunks := chunkManifestsBySizeAndWeight(manifests, maxManifestsPerPayload, maxChunkSize)
	logger.Debug("Sending manifests in chunks",
		zap.String("k8s.cluster.uid", clusterID),
		zap.String("k8s.cluster.name", clusterName),
		zap.Int("total_manifests", len(manifests)),
		zap.Int("chunk_count", len(chunks)),
		zap.Int("max_manifests_per_chunk", maxManifestsPerPayload),
		zap.Int("max_payload_size_bytes", maxChunkSize))

	if len(chunks) == 0 {
		return nil
	}
	return &K8sTranslationResult{
		Chunks:      chunks,
		ClusterName: clusterName,
		ClusterID:   clusterID,
	}
}

func getManifestType(kind string) int {
	if _, ok := k8sTypeMap[kind]; ok {
		return k8sTypeMap[kind]
	}
	// Anything not in the built-in K8s type map is a CRD-defined kind; route it as K8sCR
	if kind != "" {
		return int(orchestratormodel.K8sCR)
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
