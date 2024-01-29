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

	"github.com/DataDog/datadog-agent/comp/core/status"

	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
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
	hostinfo := utils.GetInformation()
	hostinfoMap := make(map[string]interface{})
	hostinfoBytes, _ := json.Marshal(hostinfo)
	json.Unmarshal(hostinfoBytes, &hostinfoMap) //nolint:errcheck
	stats["hostinfo"] = hostinfoMap
}

func (h *host) JSON(_ bool, stats map[string]interface{}) error {
	h.populateStatus(stats)

	return nil
}

func (h *host) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "host.tmpl", buffer, h.getStatusInfo())
}

func (h *host) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "hostHTML.tmpl", buffer, h.getStatusInfo())
}
