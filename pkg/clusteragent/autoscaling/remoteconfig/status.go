// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider reports the autoscaling configuration endpoint in the Cluster Agent
// status output, and therefore in the flare. It renders nothing and contributes
// no JSON key when no endpoint is configured, so the entry does not appear at
// all in a default deployment.
type Provider struct {
	config model.Reader
	status *EndpointStatus
}

// NewProvider returns the autoscaling configuration endpoint status provider.
func NewProvider(cfg model.Reader, endpointStatus *EndpointStatus) Provider {
	return Provider{config: cfg, status: endpointStatus}
}

// Name returns the name of this entry within its status section. Entries are
// rendered in name order, so this one comes before "Workload Autoscaling":
// where the recommendations are read from is context for what arrived.
func (Provider) Name() string {
	return "Configuration Endpoint"
}

// Section returns the name of the status section this entry belongs to, shared
// with the workload and cluster autoscaling entries.
func (Provider) Section() string {
	return "Autoscaling"
}

// JSON populates the status map.
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	if info := p.statusInfo(); info != nil {
		stats["autoscalingEndpoint"] = info
	}
	return nil
}

// Text renders the text output.
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return renderText(buffer, p.statusInfo())
}

// HTML renders the html output.
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return renderHTML(buffer, p.statusInfo())
}

func renderText(buffer io.Writer, info map[string]interface{}) error {
	if info == nil {
		return nil
	}
	return status.RenderText(templatesFS, "autoscalingendpoint.tmpl", buffer, info)
}

func renderHTML(buffer io.Writer, info map[string]interface{}) error {
	if info == nil {
		return nil
	}
	return status.RenderHTML(templatesFS, "autoscalingendpointHTML.tmpl", buffer, info)
}

// statusInfo describes where autoscaling reads its configuration from and the
// state of the client serving it. It returns nil when no endpoint is configured.
func (p Provider) statusInfo() map[string]interface{} {
	if p.config == nil || !IsConfigured(p.config) {
		return nil
	}

	// From configuration, so this renders even when the client was never built,
	// which is the interesting failure case.
	info := map[string]interface{}{
		"Endpoint": displayURL(p.config.GetString(URLSetting)),
		"Started":  false,
	}

	snapshot := p.status.snapshot()
	if !snapshot.started {
		return info
	}

	info["Started"] = true
	// Both fields are false until the first answer, so report them only once
	// there has been one. An endpoint that answers 4XX never produces one: the
	// service reports that as a proxy error, which poll drops before logging
	// above debug. This line is the only diagnostic the operator gets, so the
	// template names both settings that can cause it.
	info["Reported"] = snapshot.reported
	info["URLSetting"] = URLSetting
	info["APIKeySetting"] = APIKeySetting
	if snapshot.reported {
		info["OrgEnabled"] = snapshot.orgEnabled
		info["APIKeyAuthorized"] = snapshot.apiKeyAuthorized
	}
	info["LastError"] = snapshot.lastError

	return info
}
