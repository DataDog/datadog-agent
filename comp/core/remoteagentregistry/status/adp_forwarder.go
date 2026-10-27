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
	stats["adpForwarderStats"] = adpForwarderStatsFromViews(expvarcompat.ReadRoot("forwarder", p.source))
	return nil
}

func (p adpForwarderProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "adp_forwarder.tmpl", buffer, p.statusInfo())
}

func (p adpForwarderProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "adp_forwarder_html.tmpl", buffer, p.statusInfo())
}

func adpForwarderStatsFromViews(views expvarcompat.RootViews) map[string]map[string]any {
	result := make(map[string]map[string]any)
	for name, forwarderStats := range views.RemoteByAgent {
		if len(forwarderStats) > 0 {
			result[name] = forwarderStats
		}
	}
	return result
}

type combinedForwarderProvider struct{ source expvarcompat.Source }

// GetCombinedForwarderProvider returns a separate section for mapped Core Agent + ADP totals.
func GetCombinedForwarderProvider(source expvarcompat.Source) status.Provider {
	return combinedForwarderProvider{source: source}
}

func (p combinedForwarderProvider) Name() string    { return "Combined Forwarder" }
func (p combinedForwarderProvider) Section() string { return "Combined Forwarder" }

func (p combinedForwarderProvider) statusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	_ = p.JSON(false, stats)
	return stats
}

func (p combinedForwarderProvider) JSON(_ bool, stats map[string]interface{}) error {
	stats["combinedForwarderStats"] = combinedForwarderStatsFromViews(expvarcompat.ReadRoot("forwarder", p.source))
	return nil
}

func (p combinedForwarderProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "combined_forwarder.tmpl", buffer, p.statusInfo())
}

func (p combinedForwarderProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "combined_forwarder_html.tmpl", buffer, p.statusInfo())
}

func combinedForwarderStatsFromViews(views expvarcompat.RootViews) map[string]any {
	// An unavailable ADP has no combined Forwarder section. Core-only fields
	// such as endpoint breakdowns and on-disk storage stay in the Core section.
	if len(views.RemoteByAgent) == 0 {
		return map[string]any{}
	}
	all, ok := views.Combined["Transactions"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	transactions := make(map[string]any)
	for _, key := range []string{"Success", "Errors", "HTTPErrors", "RetryQueueSize"} {
		if value, ok := all[key]; ok {
			transactions[key] = value
		}
	}
	// These are the error-type paths selected by the Forwarder expvar mappings.
	// Include Core counts even when ADP has not yet emitted a series of that type.
	if combinedTypes, ok := all["ErrorsByType"].(map[string]any); ok {
		mappedTypes := make(map[string]any)
		for _, name := range []string{"DNSErrors", "TLSErrors", "ConnectionErrors", "WroteRequestErrors", "SentRequestErrors"} {
			if value, ok := combinedTypes[name]; ok {
				mappedTypes[name] = value
			}
		}
		if len(mappedTypes) > 0 {
			transactions["ErrorsByType"] = mappedTypes
		}
	}
	if len(transactions) == 0 {
		return map[string]any{}
	}
	return map[string]any{"Transactions": transactions}
}
