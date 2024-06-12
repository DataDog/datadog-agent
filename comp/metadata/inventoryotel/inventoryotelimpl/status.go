// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryotelimpl

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Name renders the name
func (io *inventoryotel) Name() string {
	return "OTel metadata"
}

// Index renders the index
func (io *inventoryotel) Index() int {
	return 3
}

// JSON populates the status map
func (io *inventoryotel) JSON(_ bool, stats map[string]interface{}) error {
	io.populateStatus(stats)

	return nil
}

// Text renders the text output
func (io *inventoryotel) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "inventory.tmpl", buffer, io.getStatusInfo())
}

// HTML renders the html output
func (io *inventoryotel) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "inventoryHTML.tmpl", buffer, io.getStatusInfo())
}

func (io *inventoryotel) populateStatus(stats map[string]interface{}) {
	stats["otel_metadata"] = io.Get()
}

func (io *inventoryotel) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	io.populateStatus(stats)

	return stats
}
