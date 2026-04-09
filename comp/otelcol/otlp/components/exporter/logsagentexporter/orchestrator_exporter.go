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
	"time"

	gocache "github.com/patrickmn/go-cache"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	logsmapping "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/logs"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type orchestratorExporter struct {
	config OrchestratorConfig

	// manifestCache provides an in-memory cache to avoid sending the same manifest multiple times
	// within a short period. Uses UID + resourceVersion as the cache key.
	manifestCache *gocache.Cache
}

func newOrchestratorExporter(config OrchestratorConfig) orchestratorExporter {
	exporter := orchestratorExporter{
		config: config,
	}

	if config.Enabled {
		exporter.manifestCache = logsmapping.NewManifestCache()
	}
	return exporter
}

func (e *Exporter) consumeK8sObjects(ctx context.Context, ld plog.Logs) error {
	result := logsmapping.TranslateK8sObjects(ld, e.orchestratorExporter.manifestCache, e.set.Logger)

	hostname, err := e.orchestratorExporter.config.Hostname.Get(ctx)
	if err != nil || hostname == "" {
		e.set.Logger.Error("Failed to get hostname from config", zap.Error(err))
	}

	for i, chunk := range result.Chunks {
		e.set.Logger.Debug("Sending manifest chunk",
			zap.Int("chunk_index", i),
			zap.Int("chunk_size", len(chunk)))
		payload := logsmapping.ToManifestPayload(chunk, hostname, result.ClusterName, result.ClusterID)
		if err := sendManifestPayload(ctx, e.orchestratorExporter.config.Endpoint, e.orchestratorExporter.config.Key, payload, hostname, result.ClusterID, e.set.Logger); err != nil {
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
