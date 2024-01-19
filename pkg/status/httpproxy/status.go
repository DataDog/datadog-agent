// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package httpproxy implements the status provider interface
package httpproxy

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct{}

// GetProvider if no_proxy_nonexact_match is disabled returns status.Provider otherwise returns NoopProvider
func GetProvider() status.Provider {
	if !config.Datadog.GetBool("no_proxy_nonexact_match") {
		return Provider{}
	}

	return status.NoopProvider{}
}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	p.populateStatus(stats)

	return stats
}

func (p Provider) populateStatus(stats map[string]interface{}) {
	stats["TransportWarnings"] = httputils.GetNumberOfWarnings() > 0
	stats["NoProxyIgnoredWarningMap"] = httputils.GetProxyIgnoredWarnings()
	stats["NoProxyUsedInFuture"] = httputils.GetProxyUsedInFutureWarnings()
	stats["NoProxyChanged"] = httputils.GetProxyIgnoredWarnings()
}

// Name returns the name
func (p Provider) Name() string {
	return "Transport Proxy Warnings"
}

// Section return the section
func (p Provider) Section() string {
	return "Transport Proxy Warnings"
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	p.populateStatus(stats)

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "http.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "httpHTML.tmpl", buffer, p.getStatusInfo())
}
