// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api implements the Tagger API.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/fatih/color"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	jsonutil "github.com/DataDog/datadog-agent/pkg/util/json"
)

// GetTaggerList display in a human readable format the Tagger entities into the io.Write w.
func GetTaggerList(c ipc.HTTPClient, w io.Writer, url string, jsonFlag bool, prettyJSON bool, search string) error {

	// get the tagger-list from server
	r, err := c.Get(url, ipchttp.WithLeaveConnectionOpen)
	if err != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintf(w, "The agent ran into an error while getting tags list: %s\n", string(r))
		} else {
			fmt.Fprintf(w, "Failed to query the agent (running?): %s\n", err)
		}
	}

	tr := types.TaggerListResponse{}
	err = json.Unmarshal(r, &tr)
	if err != nil {
		return err
	}

	// Filter entities if search term provided
	if search != "" {
		tr.Entities = filterEntities(tr.Entities, search)
		if len(tr.Entities) == 0 {
			return fmt.Errorf("no entities found matching %q", search)
		}
	}

	if jsonFlag || prettyJSON {
		return jsonutil.PrintJSON(w, &tr, prettyJSON, false, "")
	}

	printTaggerEntities(w, &tr)
	return nil
}

// filterEntities filters entities by searching for the term in entity IDs and source names
func filterEntities(entities map[string]types.TaggerListEntity, search string) map[string]types.TaggerListEntity {
	filtered := make(map[string]types.TaggerListEntity)

	for entityID, tagItem := range entities {
		// Check if search term is in entity ID
		if strings.Contains(entityID, search) {
			filtered[entityID] = tagItem
			continue
		}

		// Check if search term matches any source name
		for source := range tagItem.Tags {
			if strings.Contains(source, search) {
				filtered[entityID] = tagItem
				break
			}
		}
	}

	return filtered
}

// printTaggerEntities use to print Tagger entities into an io.Writer
func printTaggerEntities(w io.Writer, tr *types.TaggerListResponse) {
	for entity, tagItem := range tr.Entities {
		fmt.Fprintf(w, "\n=== Entity %s ===\n", color.GreenString(entity))

		sources := make([]string, 0, len(tagItem.Tags))
		for source := range tagItem.Tags {
			sources = append(sources, source)
		}

		// sort sources for deterministic output
		slices.Sort(sources)

		for _, source := range sources {
			fmt.Fprintf(w, "== Source %s ==\n", source)

			fmt.Fprint(w, "Tags: [")

			// sort tags for easy comparison
			tags := tagItem.Tags[source]
			slices.Sort(tags)

			for i, tag := range tags {
				tagInfo := strings.Split(tag, ":")
				fmt.Fprintf(w, "%s:%s", color.BlueString(tagInfo[0]), color.CyanString(strings.Join(tagInfo[1:], ":")))
				if i != len(tags)-1 {
					fmt.Fprintf(w, " ")
				}
			}

			fmt.Fprintln(w, "]")
		}

		fmt.Fprintln(w, "===")
	}
}
