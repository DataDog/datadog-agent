// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package haagentimpl

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Name renders the name
func (i *haagentimpl) Name() string {
	return "HA Agent Metadata"
}

// Index renders the index
func (i *haagentimpl) Index() int {
	return 3
}

// JSON populates the status map
func (i *haagentimpl) JSON(_ bool, stats map[string]interface{}) error {
	i.populateStatus(stats)

	return nil
}

// Text renders the text output
func (i *haagentimpl) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "inventory.tmpl", buffer, i.getStatusInfo())
}

// HTML renders the html output
func (i *haagentimpl) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "inventoryHTML.tmpl", buffer, i.getStatusInfo())
}

func (i *haagentimpl) populateStatus(stats map[string]interface{}) {
	stats["enabled"] = i.haAgent.Enabled()
	stats["ha_agent_metadata"] = i.Get()
}

func (i *haagentimpl) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	i.populateStatus(stats)

	return stats
}
