// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostimpl implements a component to generate the 'host' metadata payload (also known as "v5").
package hostimpl

import (
	"context"
	"embed"
	"encoding/json"
	"expvar"
	"io"
	"path"

	htmlTemplate "html/template"
	textTemplate "text/template"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

func (h *host) Name() string {
	return "Hostanme"
}

func (h *host) Index() int {
	return 1
}

func (h *host) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	h.populateStatus(stats)

	return stats
}

func (h *host) populateStatus(stats map[string]interface{}) {
	hostnameStatsJSON := []byte(expvar.Get("hostname").String())
	hostnameStats := make(map[string]interface{})
	json.Unmarshal(hostnameStatsJSON, &hostnameStats) //nolint:errcheck
	stats["hostnameStats"] = hostnameStats

	ctx := context.Background()

	payload := h.getPayload(ctx)
	metaStats := make(map[string]interface{})
	metaBytes, _ := json.Marshal(payload.Meta)

	json.Unmarshal(metaBytes, &metaStats) //nolint:errcheck

	stats["meta"] = metaStats

	hostTags := make([]string, 0, len(payload.HostTags.System)+len(payload.HostTags.GoogleCloudPlatform))
	hostTags = append(hostTags, payload.HostTags.System...)
	hostTags = append(hostTags, payload.HostTags.GoogleCloudPlatform...)
	stats["hostTags"] = hostTags
}

func (h *host) JSON(stats map[string]interface{}) error {
	h.populateStatus(stats)

	return nil
}

func (h *host) Text(buffer io.Writer) error {
	return renderText(buffer, h.getStatusInfo())
}

func (h *host) HTML(buffer io.Writer) error {
	return renderHTML(buffer, h.getStatusInfo())
}

func renderHTML(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "hostHTML.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("hostHTML").Funcs(status.HTMLFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}

func renderText(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "host.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("host").Funcs(status.TextFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}
