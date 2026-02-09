// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package workloadmeta

import (
	"fmt"
	"io"

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

// WriteText converts structured response to human-readable text format.
// Uses existing String(verbose) methods to format each entity.
func (w WorkloadDumpStructuredResponse) WriteText(writer io.Writer, verbose bool) {
	if writer != color.Output {
		color.NoColor = true
	}

	for kind, entities := range w.Entities {
		if len(entities) == 0 {
			continue
		}

		for _, entity := range entities {
			entityID := entity.GetID()
			fmt.Fprintf(writer, "\n=== Entity %s %s ===\n", color.GreenString(string(kind)), color.GreenString(entityID.ID))

			// Use existing String(verbose) method - it handles field filtering
			fmt.Fprint(writer, entity.String(verbose))

			fmt.Fprintln(writer, "===")
		}
	}
}
