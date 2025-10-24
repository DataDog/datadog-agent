// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	gocache "github.com/patrickmn/go-cache"

	"github.com/google/uuid"
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
	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		resourceLogs := ld.ResourceLogs().At(i)
		resource := resourceLogs.Resource()

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

				// Check cache to avoid sending the same manifest multiple times within 3 minutes
				if shouldSkipManifest(manifest) {
					fmt.Println("Skipping manifest (cache hit)", manifest.Uid, manifest.Kind, manifest.ResourceVersion)
					logger.Info("Skipping manifest (cache hit)",
						zap.String("uid", manifest.Uid),
						zap.String("kind", manifest.Kind),
						zap.String("resourceVersion", manifest.ResourceVersion))
					continue
				}

				manifests = append(manifests, manifest)
			}
		}
	}

	hostname, err := e.orchestratorConfig.Hostname.Get(ctx)
	if err != nil || hostname == "" {
		logger.Error("Failed to get hostname from config", zap.Error(err))
	}

	clusterID := buildClusterID(e.orchestratorConfig.ClusterName)

	payload := toManifestPayload(ctx, manifests, hostname, e.orchestratorConfig.ClusterName, clusterID)

	if err := sendManifestPayload(ctx, e.orchestratorConfig, payload, hostname, clusterID); err != nil {
		logger.Error("Failed to send collector manifest", zap.Error(err))
	}
	return nil
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

// ToDo: in cluster agent, we use kube-system namespace UID as cluster ID, here we only have cluster name
// If two clusters have the same name, it will generate the same cluster ID, which is not ideal
// Find a better way to generate cluster ID
func buildClusterID(clusterName string) string {
	hash := md5.New()
	hash.Write([]byte(clusterName))
	hashString := hex.EncodeToString(hash.Sum(nil))
	uuid, err := uuid.FromBytes([]byte(hashString[0:16]))
	if err != nil {
		logger.Error("Failed to generate UUID", zap.Error(err))
		return ""
	}
	return uuid.String()
}

// ToDo: add more types if needed
func getManifestType(kind string) int {
	switch kind {
	case "Pod":
		return int(orchestratormodel.K8sPod)
	case "Deployment":
		return int(orchestratormodel.K8sDeployment)
	case "Service":
		return int(orchestratormodel.K8sService)
	case "Node":
		return int(orchestratormodel.K8sNode)
	case "Namespace":
		return int(orchestratormodel.K8sNamespace)
	case "ReplicaSet":
		return int(orchestratormodel.K8sReplicaSet)
	case "DaemonSet":
		return int(orchestratormodel.K8sDaemonSet)
	case "StatefulSet":
		return int(orchestratormodel.K8sStatefulSet)
	case "Job":
		return int(orchestratormodel.K8sJob)
	case "CronJob":
		return int(orchestratormodel.K8sCronJob)
	case "PersistentVolume":
		return int(orchestratormodel.K8sPersistentVolume)
	case "PersistentVolumeClaim":
		return int(orchestratormodel.K8sPersistentVolumeClaim)
	case "Ingress":
		return int(orchestratormodel.K8sIngress)
	case "NetworkPolicy":
		return int(orchestratormodel.K8sNetworkPolicy)
	case "StorageClass":
		return int(orchestratormodel.K8sStorageClass)
	case "LimitRange":
		return int(orchestratormodel.K8sLimitRange)
	case "PodDisruptionBudget":
		return int(orchestratormodel.K8sPodDisruptionBudget)
	default:
		// For unknown types, use the generic manifest type
		return int(orchestratormodel.K8sUnsetType)
	}
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
