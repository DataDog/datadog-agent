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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/stormcat24/protodep/pkg/logger"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"

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
func shouldSkipManifest(manifest *agentmodel.Manifest) bool {
	if manifest == nil || manifest.Uid == "" {
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
	fmt.Println("aurele-debug: consumeK8sObjects ")

	clusterID := getClusterID(ctx)
	if clusterID == "" {
		logger.Error("Failed to get cluster ID, skipping manifest payload")
		return nil
	}

	var totalNodes int = 0

	logger.Info("consumeK8sObjects: "+strconv.Itoa(ld.ResourceLogs().Len()), zap.Int("resourceLogsLen", ld.ResourceLogs().Len()))

	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		resourceLogs := ld.ResourceLogs().At(i)
		resource := resourceLogs.Resource()

		logger.Info("consumeK8sObjects", zap.Any("resource", resource.Attributes().AsRaw()))

		for j := 0; j < resourceLogs.ScopeLogs().Len(); j++ {
			scopeLogs := resourceLogs.ScopeLogs().At(j)

			for k := 0; k < scopeLogs.LogRecords().Len(); k++ {
				logRecord := scopeLogs.LogRecords().At(k)

				// Convert Kubernetes resource manifest to orchestrator payload format
				manifest, err := toManifest(ctx, logRecord, resource)
				if err != nil {
					logger.Error("Failed to convert to manifest", zap.Error(err))
					continue
				}

				if manifest.Kind == "Node" {
					totalNodes++
				}

				// Check cache to avoid sending the same manifest multiple times within 3 minutes
				if shouldSkipManifest(manifest) {
					logger.Info("Skipping manifest (cache hit)",
						zap.String("uid", manifest.Uid),
						zap.String("kind", manifest.Kind),
						zap.String("resourceVersion", manifest.ResourceVersion))
					continue
				} else {
					logger.Info("Sending manifest",
						zap.String("uid", manifest.Uid),
						zap.String("kind", manifest.Kind),
						zap.String("resourceVersion", manifest.ResourceVersion))
				}

				manifests = append(manifests, manifest)
			}
		}
	}

	// Send a Cluster manifest once all nodes have been collected
	if totalNodes > 0 {
		logger.Info("Creating Cluster manifest after collecting nodes", zap.Int("total_nodes", totalNodes))
		clusterManifest := createClusterManifest(clusterID, totalNodes)

		// Check cache for the cluster manifest too
		if !shouldSkipManifest(clusterManifest) {
			manifests = append(manifests, clusterManifest)
			logger.Info("Added Cluster manifest to payload",
				zap.String("uid", clusterManifest.Uid),
				zap.Int("total_nodes", totalNodes))
		}
	}

	hostname, err := e.orchestratorConfig.Hostname.Get(ctx)
	if err != nil || hostname == "" {
		logger.Error("Failed to get hostname from config", zap.Error(err))
	}

	// Chunk manifests into batches of maxManifestsPerPayload and send each chunk separately
	// to ensure the backend can handle the load
	totalManifests := len(manifests)
	logger.Info("Sending manifests in chunks",
		zap.Int("total_manifests", totalManifests),
		zap.Int("chunk_size", maxManifestsPerPayload))

	for i := 0; i < totalManifests; i += maxManifestsPerPayload {
		end := i + maxManifestsPerPayload
		if end > totalManifests {
			end = totalManifests
		}

		chunk := manifests[i:end]
		logger.Info("Sending manifest chunk",
			zap.Int("chunk_start", i),
			zap.Int("chunk_end", end),
			zap.Int("chunk_size", len(chunk)))

		payload := toManifestPayload(ctx, chunk, hostname, e.orchestratorConfig.ClusterName, clusterID)

		if err := sendManifestPayload(ctx, e.orchestratorConfig, payload, hostname, clusterID); err != nil {
			logger.Error("Failed to send collector manifest chunk",
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
func getClusterID(ctx context.Context) string {
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

func toManifest(ctx context.Context, logRecord plog.LogRecord, resource pcommon.Resource) (*agentmodel.Manifest, error) {
	// Extract the Kubernetes resource data from the log record body
	var k8sResource map[string]interface{}
	if err := json.Unmarshal([]byte(logRecord.Body().AsString()), &k8sResource); err != nil {
		return nil, fmt.Errorf("failed to unmarshal k8s resource: %w", err)
	}

	// Check if the resource kind should be skipped for security reasons
	kind, _ := k8sResource["kind"].(string)
	if shouldSkipResourceKind(kind) {
		return nil, fmt.Errorf("skipping unsupported resource kind: %s (contains sensitive data)", kind)
	}

	// Convert the Kubernetes resource to JSON bytes for the manifest content
	content, err := json.Marshal(k8sResource)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal k8s resource content: %w", err)
	}

	// Extract resource type and version from the Kubernetes resource
	apiVersion, _ := k8sResource["apiVersion"].(string)
	metadata, _ := k8sResource["metadata"].(map[string]interface{})
	uid, _ := metadata["uid"].(string)
	resourceVersion, _ := metadata["resourceVersion"].(string)

	// Determine manifest type based on the Kubernetes resource kind
	manifestType := getManifestType(kind)

	// Build tags from resource and log record attributes
	// ToDo: add more meaningful tags
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
		IsTerminated:    false,
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
func createClusterManifest(clusterID string, nodeCount int) *agentmodel.Manifest {
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

	content, _ := json.Marshal(cluster)

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

func sendManifestPayload(ctx context.Context, config OrchestratorConfig, payload *agentmodel.CollectorManifest, hostName, clusterID string) error {
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

	endpoints := getEndpoints(config)

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
		tags = append(tags, fmt.Sprintf("%s:%s", key, value.AsString()))
		return true
	})

	// Add log record attributes as tags
	logRecord.Attributes().Range(func(key string, value pcommon.Value) bool {
		tags = append(tags, fmt.Sprintf("%s:%s", key, value.AsString()))
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
func getEndpoints(config OrchestratorConfig) map[string]string {
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
