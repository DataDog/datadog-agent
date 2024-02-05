// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dump holds dump related files
package dump

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// ActivityDumpGraphTemplate is the template used to generate graphs
var ActivityDumpGraphTemplate = `digraph {
		label = "{{ .Title }}"
		labelloc =  "t"
		fontsize = 75
		fontcolor = "black"
		fontname = "arial"
		ratio = expand
		ranksep = 2

		graph [pad=2]
		node [margin=0.3, padding=1, penwidth=3]
		edge [penwidth=2]

		{{ range .Nodes }}
		{{ .ID }} [label={{ if not .IsTable }}"{{ end }}{{ .Label }}{{ if not .IsTable }}"{{ end }}, fontsize={{ .Size }}, shape={{ .Shape }}, fontname = "arial", color="{{ .Color }}", fillcolor="{{ .FillColor }}", style="filled"]
		{{ end }}

		{{ range .Edges }}
		{{ .From }} -> {{ .To }} [arrowhead=none, color="{{ .Color }}"]
		{{ end }}
}`

// ToGraph convert the dump to a graph
func (ad *ActivityDump) ToGraph() utils.Graph {
	ad.Lock()
	defer ad.Unlock()

	title := fmt.Sprintf("%s: %s", ad.Metadata.Name, ad.getSelectorStr())
	var resolver *process.EBPFResolver
	if ad.adm != nil {
		resolver = ad.adm.resolvers.ProcessResolver
	}
	return ad.ActivityTree.PrepareGraphData(title, resolver)
}

// EncodeDOT encodes an activity dump in the DOT format
func (ad *ActivityDump) EncodeDOT() (*bytes.Buffer, error) {
	graph := ad.ToGraph()
	raw, err := graph.EncodeDOT(ActivityDumpGraphTemplate)
	if err != nil {
		return nil, fmt.Errorf("couldn't encode %s in %s: %w", ad.getSelectorStr(), config.Dot, err)
	}
	return raw, nil
}
