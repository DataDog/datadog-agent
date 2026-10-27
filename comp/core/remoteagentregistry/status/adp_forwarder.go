// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package status

import (
	"io"

	"github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/expvarcompat"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

type adpForwarderProvider struct{ source expvarcompat.Source }

// GetADPForwarderProvider returns the per-agent ADP Forwarder status section.
func GetADPForwarderProvider(source expvarcompat.Source) status.Provider {
	return adpForwarderProvider{source: source}
}

func (p adpForwarderProvider) Name() string    { return "ADP Forwarder" }
func (p adpForwarderProvider) Section() string { return "ADP Forwarder" }

func (p adpForwarderProvider) statusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	_ = p.JSON(false, stats)
	return stats
}

func (p adpForwarderProvider) JSON(_ bool, stats map[string]interface{}) error {
	if p.source == nil {
		stats["adpForwarderStats"] = map[string]map[string]any{}
	} else {
		stats["adpForwarderStats"] = adpForwarderStatsFromSnapshot(p.source())
	}
	return nil
}

func (p adpForwarderProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "adp_forwarder.tmpl", buffer, p.statusInfo())
}

func (p adpForwarderProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "adp_forwarder_html.tmpl", buffer, p.statusInfo())
}

func adpForwarderStatsFromSnapshot(snapshot map[string]any) map[string]map[string]any {
	result := make(map[string]map[string]any)
	for name, raw := range snapshot {
		roots, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if forwarder, ok := roots["forwarder"].(map[string]any); ok && len(forwarder) > 0 {
			result[name] = forwarder
		}
	}
	return result
}
