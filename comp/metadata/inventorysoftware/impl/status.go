// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package impl

import (
	"embed"
	"io"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Name renders the name
func (is *inventorySoftware) Name() string {
	return "Software Inventory Metadata"
}

// Index renders the index
func (is *inventorySoftware) Index() int {
	return 4
}

// JSON populates the status map
func (is *inventorySoftware) JSON(_ bool, stats map[string]interface{}) error {
	is.populateStatus(stats)

	return nil
}

// Text renders the text output
func (is *inventorySoftware) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "inventory.tmpl", buffer, is.getStatusInfo())
}

// HTML renders the html output
func (is *inventorySoftware) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "inventoryHTML.tmpl", buffer, is.getStatusInfo())
}

// For display in status we format the date as YYYYMMDD
func formatYYYYMMDD(ts string) (string, error) {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "", err
	}
	return t.Format("2006/01/02"), nil
}

func (is *inventorySoftware) populateStatus(status map[string]interface{}) {
	data := map[string]interface{}{}
	if is.cachedInventory == nil {
		_ = is.refreshCachedValues()
	}
	for _, inventory := range is.cachedInventory {
		inventory.InstallDate, _ = formatYYYYMMDD(inventory.InstallDate)
		data[inventory.GetID()] = inventory
	}
	status["software_inventory_metadata"] = data
}

func (is *inventorySoftware) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	is.populateStatus(stats)

	return stats
}
