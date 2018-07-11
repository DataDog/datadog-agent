// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ListHandler provides the tagger contents as json
func ListHandler(w http.ResponseWriter, r *http.Request) {
	response := tagger.List(tagger.IsFullCardinality())

	jsonTags, err := json.Marshal(response)
	if err != nil {
		log.Errorf("Unable to marshal tagger list response: %s", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}
	w.Write(jsonTags)
}

// PrintList prints a ListResponse as human-readable text
func PrintList(tr *tagger.ListResponse) error {
	for entity, tagItem := range tr.Entities {
		fmt.Fprintln(color.Output, fmt.Sprintf("\n=== Entity %s ===", color.GreenString(entity)))

		fmt.Fprint(color.Output, "Tags: [")
		// sort tags for easy comparison
		sort.Slice(tagItem.Tags, func(i, j int) bool {
			return tagItem.Tags[i] < tagItem.Tags[j]
		})
		for i, tag := range tagItem.Tags {
			tagInfo := strings.Split(tag, ":")
			fmt.Fprintf(color.Output, fmt.Sprintf("%s:%s", color.BlueString(tagInfo[0]), color.CyanString(strings.Join(tagInfo[1:], ":"))))
			if i != len(tagItem.Tags)-1 {
				fmt.Fprintf(color.Output, " ")
			}
		}
		fmt.Fprintln(color.Output, "]")
		fmt.Fprint(color.Output, "Sources: [")
		sort.Slice(tagItem.Sources, func(i, j int) bool {
			return tagItem.Sources[i] < tagItem.Sources[j]
		})
		for i, source := range tagItem.Sources {
			fmt.Fprintf(color.Output, fmt.Sprintf("%s", color.BlueString(source)))
			if i != len(tagItem.Sources)-1 {
				fmt.Fprintf(color.Output, " ")
			}
		}
		fmt.Fprintln(color.Output, "]")
		fmt.Fprintln(color.Output, "===")
	}

	return nil
}
