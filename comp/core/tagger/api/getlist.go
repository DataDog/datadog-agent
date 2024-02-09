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
	"sort"
	"strings"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/pkg/api/util"
)

// GetTaggerList display in a human readable format the Tagger entities into the io.Write w.
func GetTaggerList(w io.Writer, url string) error {
	c := util.GetClient(false) // FIX: get certificates right then make this true

	// get the tagger-list from server
	r, err := util.DoGet(c, url, util.LeaveConnectionOpen)
	if err != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintf(w, "The agent ran into an error while getting tags list: %s\n", string(r))
		} else {
			fmt.Fprintf(w, "Failed to query the agent (running?): %s\n", err)
		}
	}

	tr := TaggerListResponse{}
	err = json.Unmarshal(r, &tr)
	if err != nil {
		return err
	}

	printTaggerEntities(color.Output, &tr)
	return nil
}

// printTaggerEntities use to print Tagger entities into an io.Writer
func printTaggerEntities(w io.Writer, tr *TaggerListResponse) {
	for entity, tagItem := range tr.Entities {
		fmt.Fprintf(w, "\n=== Entity %s ===\n", color.GreenString(entity))

		sources := make([]string, 0, len(tagItem.Tags))
		for source := range tagItem.Tags {
			sources = append(sources, source)
		}

		// sort sources for deterministic output
		sort.Slice(sources, func(i, j int) bool {
			return sources[i] < sources[j]
		})

		for _, source := range sources {
			fmt.Fprintf(w, "== Source %s =\n=", source)

			fmt.Fprint(w, "Tags: [")

			// sort tags for easy comparison
			tags := tagItem.Tags[source]
			sort.Slice(tags, func(i, j int) bool {
				return tags[i] < tags[j]
			})

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
