// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"bytes"
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"io"
	"net/http"
	"strconv"
)

const (
	mainEndpointPrefix = "https://instrumentation-telemetry-intake."
	mainEndpointUrlKey = "apm_config.telemetry.dd_url"
)

// ApmRemoteConfigEvent is used to report remote config updates to the Datadog backend
type ApmRemoteConfigEvent struct {
	RequestType string                      `json:"request_type"`
	ApiVersion  string                      `json:"api_version"`
	Payload     ApmRemoteConfigEventPayload `json:"payload,omitempty"`
}

// ApmRemoteConfigEventPayload contains the information on an individual remote config event
type ApmRemoteConfigEventPayload struct {
	EventName string                    `json:"event_name"`
	Tags      ApmRemoteConfigEventTags  `json:"tags"`
	Error     ApmRemoteConfigEventError `json:"error,omitempty"`
}

// ApmRemoteConfigEventTags store the information on an individual remote config event
type ApmRemoteConfigEventTags struct {
	Env                 string `json:"env"`
	RcId                string `json:"rc_id"`
	RcClientId          string `json:"rc_client_id"`
	RcRevision          string `json:"rc_revision"`
	RcVersion           int64  `json:"rc_version"`
	KubernetesClusterId string `json:"k8s_cluster_id"`
	KubernetesCluster   string `json:"k8s_cluster"`
	KubernetesNamespace string `json:"k8s_namespace"`
	KubernetesKind      string `json:"k8s_kind"`
	KubernetesName      string `json:"k8s_name"`
}

// ApmRemoteConfigEventError stores the debugging information about remote config deployment failures
type ApmRemoteConfigEventError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// TelemetryCollector is the interface used to send reports about startup to the instrumentation telemetry intake
type TelemetryCollector interface {
	SendEvent(event *ApmRemoteConfigEvent)
}

type telemetryCollector struct {
	client    *http.Client
	host      string
	userAgent string
}

// NewCollector returns either collector, or a noop implementation if instrumentation telemetry is disabled
func NewCollector() TelemetryCollector {
	return &telemetryCollector{
		client:    http.DefaultClient,
		host:      utils.GetMainEndpoint(config.Datadog, mainEndpointPrefix, mainEndpointUrlKey),
		userAgent: "Datadog Cluster Agent",
	}
}

// NewNoopCollector returns a noop collector
func NewNoopCollector() TelemetryCollector {
	return &noopTelemetryCollector{}
}

func (collector *telemetryCollector) SendEvent(event *ApmRemoteConfigEvent) {
	body, err := json.Marshal(event)
	if err != nil {
		log.Error("Error while trying to marshal a remote config event to JSON: %v", err)
		return
	}
	bodyLen := strconv.Itoa(len(body))

	req, err := http.NewRequest("POST", collector.host+"/api/v2/apmtelemetry", bytes.NewReader(body))
	if err != nil {
		log.Error("Error while trying to create a web request for a remote config event: %v", err)
		return
	}
	if !config.Datadog.IsSet("api_key") {
		return
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("User-Agent", collector.userAgent)
	req.Header.Add("DD-API-KEY", config.Datadog.GetString("api_key"))
	req.Header.Add("Content-Length", bodyLen)

	resp, err := collector.client.Do(req)
	if err != nil {
		log.Error("Failed to transmit remote config event to Datadog: %v", err)
		return
	}
	// Unconditionally read the body and ignore any errors
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

type noopTelemetryCollector struct{}

func (*noopTelemetryCollector) SendEvent(event *ApmRemoteConfigEvent) {}
