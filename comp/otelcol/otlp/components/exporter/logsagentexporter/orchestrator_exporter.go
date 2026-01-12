// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	logsmapping "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/logs"
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
)

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
				manifest, isWatchEvent, err = logsmapping.ToManifest(logRecord)
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
		clusterManifest := logsmapping.CreateClusterManifest(clusterID, nodes, e.set.Logger)

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

		payload := logsmapping.ToManifestPayload(chunk, hostname, clusterName, clusterID)

		if err := sendManifestPayload(ctx, e.orchestratorConfig.Endpoint, e.orchestratorConfig.Key, payload, hostname, clusterID, e.set.Logger); err != nil {
			e.set.Logger.Error("Failed to send collector manifest chunk",
				zap.Int("chunk_index", i),
				zap.Int("chunk_size", len(chunk)),
				zap.Error(err))
		}
	}

	return nil
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
