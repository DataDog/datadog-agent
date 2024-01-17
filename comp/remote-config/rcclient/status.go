// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rcclient

import (
	"embed"
	"encoding/json"
	"expvar"
	"io"
	"path"

	htmlTemplate "html/template"
	textTemplate "text/template"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// PopulateStatus populates the status stats
func PopulateStatus(stats map[string]interface{}) {
	status := make(map[string]interface{})

	if config.IsRemoteConfigEnabled(config.Datadog) && expvar.Get("remoteConfigStatus") != nil {
		remoteConfigStatusJSON := expvar.Get("remoteConfigStatus").String()
		json.Unmarshal([]byte(remoteConfigStatusJSON), &status) //nolint:errcheck
	} else {
		if !config.Datadog.GetBool("remote_configuration.enabled") {
			status["disabledReason"] = "it is explicitly disabled in the agent configuration. (`remote_configuration.enabled: false`)"
		} else if config.Datadog.GetBool("fips.enabled") {
			status["disabledReason"] = "it is not supported when FIPS is enabled. (`fips.enabled: true`)"
		} else if config.Datadog.GetString("site") == "ddog-gov.com" {
			status["disabledReason"] = "it is not supported on GovCloud. (`site: \"ddog-gov.com\"`)"
		}
	}

	stats["remoteConfiguration"] = status
}

//go:embed status_templates
var templatesFS embed.FS

func (rc rcClient) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	PopulateStatus(stats)

	return stats
}

func (rc rcClient) Name() string {
	return "Remote Configuration"
}

func (rc rcClient) Section() string {
	return "Remote Configuration"
}

func (rc rcClient) JSON(_ bool, stats map[string]interface{}) error {
	PopulateStatus(stats)

	return nil
}

func (rc rcClient) Text(_ bool, buffer io.Writer) error {
	return renderText(buffer, rc.getStatusInfo())
}

func (rc rcClient) HTML(_ bool, buffer io.Writer) error {
	return renderHTML(buffer, rc.getStatusInfo())
}

func renderHTML(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "remoteconfigurationHTML.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("remoteconfigurationHTML").Funcs(status.HTMLFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}

func renderText(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "remoteconfiguration.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("remoteconfiguration").Funcs(status.TextFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}
