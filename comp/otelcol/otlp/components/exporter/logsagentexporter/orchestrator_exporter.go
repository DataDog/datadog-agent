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
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	orchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// manifestCacheTTL is the time-to-live for manifest cache entries (3 minutes, same as orchestrator collector)
	manifestCacheTTL = 3 * time.Minute
	// manifestCachePurge is the interval for purging expired cache entries
	manifestCachePurge = 30 * time.Second
	// maxManifestsPerPayload is the maximum number of manifests to send in a single payload
	maxManifestsPerPayload = 100
)

var (
	// manifestCache provides an in-memory cache to avoid sending the same manifest multiple times
	// within a short period. Uses UID + resourceVersion as the cache key.
	manifestCache     *gocache.Cache
	manifestCacheOnce sync.Once
	k8sTypeMap        map[string]int
	clusterIDCache    string
	clusterIDOnce     sync.Once
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
	return false
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

func (e *Exporter) consumeK8sObjects(ctx context.Context, ld plog.Logs) (err error) {
	var manifests []*agentmodel.Manifest

	clusterID := getClusterID(ctx, e.set.Logger)
	if clusterID == "" {
		e.set.Logger.Error("Failed to get cluster ID, skipping manifest payload")
		return nil
	}

	var totalNodes int

	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		resourceLogs := ld.ResourceLogs().At(i)
		resource := resourceLogs.Resource()

		for j := 0; j < resourceLogs.ScopeLogs().Len(); j++ {
			scopeLogs := resourceLogs.ScopeLogs().At(j)

			for k := 0; k < scopeLogs.LogRecords().Len(); k++ {
				logRecord := scopeLogs.LogRecords().At(k)

				// Convert Kubernetes resource manifest to orchestrator payload format
				manifest, isWatchEvent, err := toManifest(ctx, logRecord, resource)
				if err != nil {
					e.set.Logger.Error("Failed to convert to manifest: "+err.Error(), zap.Error(err))
					continue
				}

				if manifest.Kind == "Node" {
					totalNodes++
				}

				// Check cache to avoid sending the same manifest multiple times within 3 minutes
				// Watch events bypass the cache to ensure real-time updates
				if shouldSkipManifest(manifest, isWatchEvent) {
					e.set.Logger.Info("Skipping manifest (cache hit)",
						zap.String("uid", manifest.Uid),
						zap.String("kind", manifest.Kind),
						zap.String("resourceVersion", manifest.ResourceVersion))
					continue
				}

				e.set.Logger.Info("Sending manifest",
					zap.String("uid", manifest.Uid),
					zap.String("kind", manifest.Kind),
					zap.String("resourceVersion", manifest.ResourceVersion))

				manifests = append(manifests, manifest)
			}
		}
	}

	// Send a Cluster manifest once all nodes have been collected
	if totalNodes > 0 {
		e.set.Logger.Info("Creating Cluster manifest after collecting nodes", zap.Int("total_nodes", totalNodes))
		clusterManifest := createClusterManifest(clusterID, totalNodes, e.set.Logger)

		// Check cache for the cluster manifest too (not a watch event)
		if !shouldSkipManifest(clusterManifest, false) {
			manifests = append(manifests, clusterManifest)
			e.set.Logger.Info("Added Cluster manifest to payload",
				zap.String("uid", clusterManifest.Uid),
				zap.Int("total_nodes", totalNodes))
		}
	}

	hostname, err := e.orchestratorConfig.Hostname.Get(ctx)
	if err != nil || hostname == "" {
		e.set.Logger.Error("Failed to get hostname from config", zap.Error(err))
	}

	// Chunk manifests into batches of maxManifestsPerPayload and send each chunk separately
	// to ensure the backend can handle the load
	totalManifests := len(manifests)
	e.set.Logger.Info("Sending manifests in chunks",
		zap.Int("total_manifests", totalManifests),
		zap.Int("chunk_size", maxManifestsPerPayload))

	for i := 0; i < totalManifests; i += maxManifestsPerPayload {
		end := i + maxManifestsPerPayload
		if end > totalManifests {
			end = totalManifests
		}

		chunk := manifests[i:end]
		e.set.Logger.Info("Sending manifest chunk",
			zap.Int("chunk_start", i),
			zap.Int("chunk_end", end),
			zap.Int("chunk_size", len(chunk)))

		payload := toManifestPayload(ctx, chunk, hostname, e.orchestratorConfig.ClusterName, clusterID)

		if err := sendManifestPayload(ctx, e.orchestratorConfig, payload, hostname, clusterID, e.set.Logger); err != nil {
			e.set.Logger.Error("Failed to send collector manifest chunk",
				zap.Int("chunk_start", i),
				zap.Int("chunk_end", end),
				zap.Error(err))
		}
	}

	return nil
}

// getClusterID retrieves the cluster ID by fetching the kube-system namespace UID.
// This matches the behavior of the cluster agent which uses the kube-system namespace UID
// as the cluster ID to ensure uniqueness across clusters.
// The cluster ID is cached after the first successful retrieval to avoid repeated API calls.
func getClusterID(ctx context.Context, logger *zap.Logger) string {
	clusterIDOnce.Do(func() {
		// Create in-cluster Kubernetes client
		config, err := rest.InClusterConfig()
		if err != nil {
			logger.Error("Failed to get in-cluster Kubernetes config", zap.Error(err))
			return
		}

		// Create clientset
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			logger.Error("Failed to create Kubernetes client", zap.Error(err))
			return
		}

		// Fetch kube-system namespace
		namespace, err := clientset.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
		if err != nil {
			logger.Error("Failed to get kube-system namespace", zap.Error(err))
			return
		}

		// Extract and cache the UID
		clusterID := string(namespace.UID)
		if clusterID == "" {
			logger.Error("kube-system namespace UID is empty")
			return
		}

		clusterIDCache = clusterID
		logger.Info("Successfully retrieved cluster ID from kube-system namespace", zap.String("cluster_id", clusterID))
	})

	return clusterIDCache
}

// toManifest converts a log record from k8sobjectsreceiver to an orchestrator manifest.
// The receiver supports two modes:
//   - Pull mode: k8s object is directly in the log body as JSON
//   - Watch mode: log body contains an "object" field with the k8s resource, and a "type" field for event type
//
// Returns the manifest and a boolean indicating if it's from a watch event.
func toManifest(ctx context.Context, logRecord plog.LogRecord, resource pcommon.Resource) (*agentmodel.Manifest, bool, error) {
	// Try to parse the body to detect the mode
	var bodyMap map[string]interface{}
	if err := json.Unmarshal([]byte(logRecord.Body().AsString()), &bodyMap); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal log body: %w", err)
	}

	// Check if this is a watch log (body contains "object" field)
	if objectField, hasObject := bodyMap["object"]; hasObject {
		// Watch log: body has structure like {"object": {...}, "type": "ADDED"}
		manifest, err := watchLogToManifest(ctx, objectField, bodyMap, logRecord, resource)
		return manifest, true, err
	}

	// Pull log: body directly contains the k8s object
	manifest, err := pullLogToManifestFromMap(ctx, bodyMap, logRecord, resource)
	return manifest, false, err
}

// watchLogToManifest handles logs from k8sobjectsreceiver in watch mode.
// Structure of watch mode logs - Body is a JSON string containing:
//
//	{
//	  "object": {...k8s resource...},
//	  "type": "ADDED" | "MODIFIED" | "DELETED"
//	}
func watchLogToManifest(ctx context.Context, objectField interface{}, bodyMap map[string]interface{}, logRecord plog.LogRecord, resource pcommon.Resource) (*agentmodel.Manifest, error) {
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

	return buildManifestFromK8sResource(ctx, k8sResource, resource, logRecord, isTerminated, true)
}

// pullLogToManifestFromMap handles logs from k8sobjectsreceiver in pull mode.
// Structure of pull mode logs:
//   - Body: JSON string containing the k8s resource directly (e.g., {"apiVersion":"v1","kind":"Pod",...})
//   - Attributes: May contain additional metadata from the receiver
func pullLogToManifestFromMap(ctx context.Context, k8sResource map[string]interface{}, logRecord plog.LogRecord, resource pcommon.Resource) (*agentmodel.Manifest, error) {
	// Body is already parsed, just pass it to common logic
	// Not a delete event in pull mode, not from watch
	return buildManifestFromK8sResource(ctx, k8sResource, resource, logRecord, false, false)
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
func buildManifestFromK8sResource(ctx context.Context, k8sResource map[string]interface{}, resource pcommon.Resource, logRecord plog.LogRecord, isTerminated bool, isWatchEvent bool) (*agentmodel.Manifest, error) {
	// Check if the resource kind should be skipped for security reasons
	kind, _ := k8sResource["kind"].(string)
	if shouldSkipResourceKind(kind) {
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

	// Add event type tag for deleted resources
	if isTerminated {
		tags = append(tags, "event_type:delete")
	}

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

func toManifestPayload(ctx context.Context, manifests []*agentmodel.Manifest, hostName, clusterName, clusterID string) *agentmodel.CollectorManifest {
	return &agentmodel.CollectorManifest{
		ClusterName:     clusterName,
		ClusterId:       clusterID,
		HostName:        hostName,
		Manifests:       manifests,
		Tags:            buildCommonTags(),
		OriginCollector: agentmodel.OriginCollector_datadogExporter,
	}
}

// createClusterManifest creates a Cluster manifest to be sent after all nodes have been collected.
// This is used to trigger cluster-level processing in the backend after node collection is complete.
func createClusterManifest(clusterID string, nodeCount int, logger *zap.Logger) *agentmodel.Manifest {
	// Create a minimal cluster resource
	cluster := map[string]interface{}{
		"apiVersion": "virtual.datadoghq.com/v1",
		"kind":       "Cluster",
		"metadata": map[string]interface{}{
			"uid":             clusterID,
			"resourceVersion": fmt.Sprintf("cluster-%d", time.Now().Unix()),
			"name":            "cluster",
			"annotations": map[string]interface{}{
				"node-count": strconv.Itoa(nodeCount),
				"synthetic":  "true",
			},
		},
	}

	content, err := json.Marshal(cluster)
	if err != nil {
		logger.Error("Failed to marshal cluster manifest", zap.Error(err))
		return nil
	}

	return &agentmodel.Manifest{
		Type:            int32(getManifestType("Cluster")),
		ResourceVersion: fmt.Sprintf("fake-%d", time.Now().Unix()),
		Uid:             clusterID,
		Content:         content,
		ContentType:     "application/json",
		Version:         "v1",
		Tags:            append(buildCommonTags(), "synthetic:true", fmt.Sprintf("node_count:%d", nodeCount)),
		IsTerminated:    false,
		ApiVersion:      "virtual.datadoghq.com/v1",
		Kind:            "Cluster",
	}
}

func sendManifestPayload(ctx context.Context, config OrchestratorConfig, payload *agentmodel.CollectorManifest, hostName, clusterID string, logger *zap.Logger) error {
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

	endpoints := getEndpoints(config, logger)

	encoded, err := agentmodel.EncodeMessage(agentmodel.Message{
		Header: agentmodel.MessageHeader{
			Version:  agentmodel.MessageV3,
			Encoding: agentmodel.MessageEncodingZstdPBxNoCgo,
			Type:     agentmodel.TypeCollectorManifest,
		}, Body: payload})
	if err != nil {
		return fmt.Errorf("failed to encode payload: %w", err)
	}

	for endpoint, apiKey := range endpoints {
		// Create the request
		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(encoded))
		if err != nil {
			logger.Error("Failed to create request", zap.Error(err))
			continue
		}

		// Set headers
		req.Header.Set("Content-Type", "application/x-protobuf")
		req.Header.Set("DD-API-KEY", string(apiKey))
		req.Header.Set("X-Dd-Hostname", hostName)
		req.Header.Set("X-DD-Agent-Timestamp", strconv.Itoa(int(time.Now().Unix())))
		req.Header.Set("X-Dd-Orchestrator-ClusterID", clusterID)
		req.Header.Set("DD-EVP-ORIGIN", "agent")
		req.Header.Set("DD-EVP-ORIGIN-VERSION", agentVersion)

		// Send the request
		resp, err := httpClient.Do(req)
		if err != nil {
			logger.Error("Failed to send request", zap.String("endpoint", endpoint), zap.Error(err))
			continue
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			logger.Error("Orchestrator endpoint returned non-200 status", zap.String("endpoint", endpoint), zap.Int("status", resp.StatusCode))
			continue
		}
		err = resp.Body.Close()
		if err != nil {
			logger.Error("Failed to close response body", zap.String("endpoint", endpoint), zap.Error(err))
		}
	}
	return nil
}

func getManifestType(kind string) int {
	if _, ok := k8sTypeMap[kind]; ok {
		return k8sTypeMap[kind]
	}

	return int(orchestratormodel.K8sUnsetType)
}

// ToDo: add more common tags
func buildCommonTags() []string {
	return []string{
		"otel_receiver:k8sobjectsreceiver",
	}
}

// ToDo: add more meaningful tags
func buildTags(resource pcommon.Resource, logRecord plog.LogRecord) []string {
	tags := buildCommonTags()

	// Add resource attributes as tags
	resource.Attributes().Range(func(key string, value pcommon.Value) bool {
		tags = append(tags, fmt.Sprintf("otel_%s:%s", strings.ReplaceAll(key, ".", "_"), value.AsString()))
		return true
	})

	// Add log record attributes as tags
	logRecord.Attributes().Range(func(key string, value pcommon.Value) bool {
		tags = append(tags, fmt.Sprintf("otel_%s:%s", strings.ReplaceAll(key, ".", "_"), value.AsString()))
		return true
	})

	return tags
}

// shouldSkipResourceKind returns true if the Kubernetes resource kind should be skipped
// for security or data sensitivity reasons. This matches the behavior of the orchestrator
// collector which skips secrets and configmaps as they can contain sensitive data.
func shouldSkipResourceKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "secret", "configmap":
		return true
	default:
		return false
	}
}

// getEndpoints builds the orchestrator manifest endpoint URLs from the provided configuration.
// This follows the same pattern as the forwarder: Domain + Route (see transaction.HTTPTransaction.GetTarget)
func getEndpoints(config OrchestratorConfig, logger *zap.Logger) map[string]string {
	// Use the same endpoint route as defined in the forwarder
	manifestRoute := endpoints.OrchestratorManifestEndpoint.Route

	result := make(map[string]string)

	// Process configured endpoints
	for ep, apiKey := range config.Endpoints {
		if ep == "" {
			continue
		}

		// Parse URL and extract base domain (scheme://host)
		u, err := url.Parse(ep)
		if err != nil {
			logger.Warn("Failed to parse endpoint URL, skipping", zap.String("endpoint", ep), zap.Error(err))
			continue
		}

		// Build full URL: domain + route (same as forwarder transaction pattern)
		domain := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		fullURL := domain + manifestRoute
		result[fullURL] = apiKey
	}

	// Use default endpoint if no valid endpoints were configured
	if len(result) == 0 {
		domain := fmt.Sprintf("https://orchestrator.%s", config.Site)
		fullURL := domain + manifestRoute
		result[fullURL] = config.Key
	}

	return result
}
