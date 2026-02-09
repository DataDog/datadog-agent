// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package workloadmeta

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
)

// WorkloadDumpResponse is used to dump the store content.
type WorkloadDumpResponse struct {
	Entities map[string]WorkloadEntity `json:"entities"`
}

// WorkloadEntity contains entity data.
type WorkloadEntity struct {
	Infos map[string]string `json:"infos"`
}

// Write writes the stores content in a given writer.
// Useful for agent's CLI and Flare.
func (wdr WorkloadDumpResponse) Write(writer io.Writer) {
	if writer != color.Output {
		color.NoColor = true
	}

	for kind, entities := range wdr.Entities {
		for entity, info := range entities.Infos {
			fmt.Fprintf(writer, "\n=== Entity %s %s ===\n", color.GreenString(kind), color.GreenString(entity))
			fmt.Fprint(writer, info)
			fmt.Fprintln(writer, "===")
		}
	}
}

// WorkloadDumpStructuredResponse is used to dump the store content with structured data.
type WorkloadDumpStructuredResponse struct {
	Entities map[string][]Entity `json:"entities"`
}

// BuildWorkloadResponse builds a JSON response for workload-list with filtering.
//
// Backend does all processing to avoid client-side unmarshaling issues:
//  1. Get structured/text entities from workloadmeta store
//  2. Apply search filtering if provided
//  3. Return in requested format:
//     - jsonFormat=true: Return structured JSON (for -j/-p flags)
//     - jsonFormat=false: Return text format with preserved source info
//
// This approach leverages the backend's access to concrete entity types, avoiding the
// unmarshaling problem where clients can't reconstruct interface types from JSON.
func BuildWorkloadResponse(wmeta Component, verbose bool, search string, jsonFormat bool) ([]byte, error) {
	if jsonFormat {
		structuredResp := wmeta.DumpStructured(verbose)
		if search != "" {
			structuredResp = FilterStructuredResponse(structuredResp, search)
		}
		return json.Marshal(structuredResp)
	}

	// Text format - use DumpFiltered which preserves source info format
	return json.Marshal(wmeta.DumpFiltered(verbose, search))
}

// FilterStructuredResponse filters entities by kind or entity ID
func FilterStructuredResponse(response WorkloadDumpStructuredResponse, search string) WorkloadDumpStructuredResponse {
	filtered := WorkloadDumpStructuredResponse{
		Entities: make(map[string][]Entity),
	}

	for kind, entities := range response.Entities {
		if strings.Contains(kind, search) {
			// Kind matches - include all entities
			filtered.Entities[kind] = entities
			continue
		}

		// Filter by entity ID
		var matchingEntities []Entity
		for _, entity := range entities {
			if strings.Contains(entity.GetID().ID, search) {
				matchingEntities = append(matchingEntities, entity)
			}
		}

		if len(matchingEntities) > 0 {
			filtered.Entities[kind] = matchingEntities
		}
	}

	return filtered
}
