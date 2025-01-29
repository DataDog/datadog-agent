// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryhaagentlimpl

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Name renders the name
func (io *inventoryhaagent) Name() string {
	return "OTel metadata"
}

// Index renders the index
func (io *inventoryhaagent) Index() int {
	return 3
}

// JSON populates the status map
func (io *inventoryhaagent) JSON(_ bool, stats map[string]interface{}) error {
	io.populateStatus(stats)

	return nil
}

// Text renders the text output
func (io *inventoryhaagent) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "inventory.tmpl", buffer, io.getStatusInfo())
}

// HTML renders the html output
func (io *inventoryhaagent) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "inventoryHTML.tmpl", buffer, io.getStatusInfo())
}

func (io *inventoryhaagent) populateStatus(stats map[string]interface{}) {
	stats["haagent_metadata"] = io.Get()
}

func (io *inventoryhaagent) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	io.populateStatus(stats)

	return stats
}
