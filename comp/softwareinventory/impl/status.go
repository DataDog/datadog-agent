// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package softwareinventoryimpl

import (
	"embed"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
)

//go:embed status_templates
var templatesFS embed.FS

// Name returns the display name for the software inventory status section.
// This name appears in the agent status output to identify the software inventory
// metadata section.
func (is *softwareInventory) Name() string {
	return "Software Inventory Metadata"
}

// Index returns the display order for the software inventory status section.
// Lower numbers appear earlier in the status output. The value 4 places this
// section after core components but before other metadata sections.
func (is *softwareInventory) Index() int {
	return 4
}

// JSON populates the status map with software inventory data in JSON format.
// This method is called when generating JSON status output and adds the
// software inventory information to the provided stats map.
func (is *softwareInventory) JSON(_ bool, stats map[string]interface{}) error {
	is.populateStatus(stats)

	return nil
}

// Text renders the text output for the software inventory status section.
// This method uses the embedded template to generate human-readable text
// output showing the software inventory information.
func (is *softwareInventory) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "inventory.tmpl", buffer, is.getStatusInfo())
}

// HTML renders the html output for the software inventory status section.
// This method uses the embedded template to generate HTML output showing
// the software inventory information in a web-friendly format.
func (is *softwareInventory) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "inventoryHTML.tmpl", buffer, is.getStatusInfo())
}

// formatYYYYMMDD converts a timestamp string to YYYY/MM/DD format for display.
// This function is used to format installation dates in a human-readable format
// for the status output. It expects the input to be in RFC3339 format.
func formatYYYYMMDD(ts string) (string, error) {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "", err
	}
	return t.Format("2006/01/02"), nil
}

// populateStatus populates the status map with software inventory data.
// This method processes the cached inventory data and formats it for display
// in the status output. It handles date formatting, computes statistics by
// software type, and organizes the data by software ID for easy lookup.
// Entries are sorted by Source (software type) and then by DisplayName for
// easier navigation in the GUI and status output.
// Note: Stats are computed from deduplicated entries to ensure consistency
// between the total count and the breakdown by type.
func (is *softwareInventory) populateStatus(status map[string]interface{}) {
	is.cachedInventoryMu.RLock()
	cachedInventory := is.cachedInventory
	is.cachedInventoryMu.RUnlock()

	// First pass: deduplicate entries by ID and format dates
	data := map[string]interface{}{}
	for _, inventory := range cachedInventory {
		inventory.InstallDate, _ = formatYYYYMMDD(inventory.InstallDate)
		data[inventory.GetID()] = inventory
	}

	// Convert to slice and sort by Source (category), then DisplayName
	sortedEntries := make([]software.Entry, 0, len(data))
	for _, v := range data {
		sortedEntries = append(sortedEntries, v.(software.Entry))
	}
	sort.Slice(sortedEntries, func(i, j int) bool {
		// First sort by Source (software type)
		if sortedEntries[i].Source != sortedEntries[j].Source {
			return sortedEntries[i].Source < sortedEntries[j].Source
		}
		// Then sort by DisplayName within each category
		return sortedEntries[i].DisplayName < sortedEntries[j].DisplayName
	})

	// Second pass: compute stats from deduplicated entries
	// This ensures stats sum matches the total count
	stats := map[string]int{}
	brokenCount := 0
	for _, inventory := range sortedEntries {
		stats[inventory.Source]++
		if strings.Contains(inventory.Status, "broken") {
			brokenCount++
		}
	}

	status["software_inventory_metadata"] = sortedEntries
	status["software_inventory_stats"] = stats
	status["software_inventory_total"] = len(sortedEntries)
	// Only include broken count if there are broken entries
	if brokenCount > 0 {
		status["software_inventory_broken"] = brokenCount
	}
}

// getStatusInfo returns the status information map for the software inventory.
// This method prepares all the data needed for status rendering, including
// the processed software inventory information.
func (is *softwareInventory) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	is.populateStatus(stats)

	return stats
}
