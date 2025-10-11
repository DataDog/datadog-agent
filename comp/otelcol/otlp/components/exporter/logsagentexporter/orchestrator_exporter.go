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
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/stormcat24/protodep/pkg/logger"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	orchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
)

func (e *Exporter) consumeK8sObjects(ctx context.Context, ld plog.Logs) (err error) {
	var manifests []*agentmodel.Manifest
	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		resourceLogs := ld.ResourceLogs().At(i)
		resource := resourceLogs.Resource()

		for j := 0; j < resourceLogs.ScopeLogs().Len(); j++ {
			scopeLogs := resourceLogs.ScopeLogs().At(j)

			for k := 0; k < scopeLogs.LogRecords().Len(); k++ {
				logRecord := scopeLogs.LogRecords().At(k)

				// Convert Kubernetes resource manifest to orchestrator payload format
				fmt.Println("manifest: ", logRecord.Body().AsString(), "att:", logRecord.Attributes().AsRaw())
				manifest, err := toManifest(ctx, logRecord, resource)
				if err != nil {
					logger.Error("Failed to convert to manifest", zap.Error(err))
					continue
				}

				// ToDo: add 3m TTL cache to avoid sending the same manifest multiple times in a short period
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

	// Todo: check if k8sResource is supported, for example if kind is configmap, secret, etc, we skip it

	// Convert the Kubernetes resource to JSON bytes for the manifest content
	_, err := json.Marshal(k8sResource)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal k8s resource content: %w", err)
	}

	// Extract resource type and version from the Kubernetes resource
	apiVersion, _ := k8sResource["apiVersion"].(string)
	kind, _ := k8sResource["kind"].(string)
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
		Content:         []byte(logRecord.Body().AsString()),
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
		ClusterName: clusterName,
		ClusterId:   clusterID,
		HostName:    hostName,
		Manifests:   manifests,
		Tags:        buildCommonTags(),
		Source:      "datadog-exporter",
	}
}

func sendManifestPayload(ctx context.Context, config OrchestratorConfig, payload *agentmodel.CollectorManifest, hostName, clusterID string) error {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
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
		req.Header.Set("DD-EVP-ORIGIN-VERSION", "1.0.0") // ToDo: set the actual version

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

// ToDo: consider using some common function we already have in the agent codebase if possible
func getEndpoints(config OrchestratorConfig) map[string]string {
	suffix := "api/v2/orchmanif"

	endpoints := make(map[string]string)
	for ep, apiKey := range config.Endpoints {
		if ep != "" {
			if !strings.HasSuffix(ep, suffix) {
				if strings.HasSuffix(ep, "/") {
					ep = ep + suffix
				} else {
					ep = fmt.Sprintf("%s/%s", ep, suffix)
				}

			}
			endpoints[ep] = apiKey
		}
	}

	if len(endpoints) == 0 {
		endpoints[fmt.Sprintf("https://orchestrator.%s/%s", config.Site, suffix)] = config.Key
	}
	return endpoints
}
