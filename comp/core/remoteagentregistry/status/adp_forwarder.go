// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package status

import (
	"encoding/json"
	"expvar"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

type adpForwarderProvider struct{}

// GetADPForwarderProvider returns a separate Core Agent status section for ADP Forwarder telemetry.
func GetADPForwarderProvider() status.Provider {
	return adpForwarderProvider{}
}

func (p adpForwarderProvider) Name() string    { return "ADP Forwarder" }
func (p adpForwarderProvider) Section() string { return "ADP Forwarder" }

func (p adpForwarderProvider) statusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	_ = p.JSON(false, stats)
	return stats
}

func (p adpForwarderProvider) JSON(_ bool, stats map[string]interface{}) error {
	stats["adpForwarderStats"] = getADPForwarderStats()
	return nil
}

func (p adpForwarderProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "adp_forwarder.tmpl", buffer, p.statusInfo())
}

func (p adpForwarderProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "adp_forwarder_html.tmpl", buffer, p.statusInfo())
}

func getADPForwarderStats() map[string]interface{} {
	result := make(map[string]interface{})
	remoteAgents := expvar.Get("remote_agents")
	if remoteAgents == nil {
		return result
	}
	return adpForwarderStatsFromJSON(remoteAgents.String())
}

func adpForwarderStatsFromJSON(remoteAgentsJSON string) map[string]interface{} {
	result := make(map[string]interface{})
	var agents map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(remoteAgentsJSON), &agents); err != nil {
		return result
	}
	for name, agentStats := range agents {
		if forwarderStats, ok := agentStats["forwarder"].(map[string]interface{}); ok && len(forwarderStats) > 0 {
			result[name] = forwarderStats
		}
	}
	return result
}
