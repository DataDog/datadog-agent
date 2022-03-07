// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"fmt"
	"strings"
	"text/template"

	"golang.org/x/crypto/blake2b"
)

var (
	processColor = "#8fbbff"
	fileColor    = "#77bf77"
)

type node struct {
	ID    string
	Label string
	Size  int
	Color string
}

type edge struct {
	Link  string
	Color string
}

type graph struct {
	Title string
	Nodes map[string]node
	Edges []edge
}

func (ad *ActivityDump) generateGraph(title string) error {
	if ad.graphFile == nil {
		return nil
	}

	tmpl := `digraph {
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
		{{ .ID }} [label="{{ .Label }}", fontsize={{ .Size }}, shape=record, fontname = "arial", color="{{ .Color }}"]{{ end }}

		{{ range .Edges }}
		{{ .Link }} [arrowhead=none, color="{{ .Color }}"]
		{{ end }}
}`

	data := ad.prepareGraphData(title)
	t := template.Must(template.New("tmpl").Parse(tmpl))
	return t.Execute(ad.graphFile, data)
}

func (ad *ActivityDump) prepareGraphData(title string) graph {
	data := graph{
		Title: title,
		Nodes: make(map[string]node),
	}

	for _, p := range ad.ProcessActivityTree {
		ad.prepareProcessActivityNode(p, &data)
	}

	return data
}

func (ad *ActivityDump) prepareProcessActivityNode(p *ProcessActivityNode, data *graph) {
	processID := fmt.Sprintf("%s_%s_%d", p.Process.PathnameStr, p.Process.ExecTime, p.Process.Tid)
	var args []string
	args, _ = ad.resolvers.ProcessResolver.GetProcessArgv(&p.Process)
	data.Nodes[processID] = node{
		ID:    generateNodeID(processID),
		Label: fmt.Sprintf("%s %s", p.Process.PathnameStr, strings.Join(args, " ")),
		Size:  60,
		Color: processColor,
	}
	for _, f := range p.Files {
		fileID := fmt.Sprintf("%s_%s_%s", processID, f.Name, f.Name)
		data.Edges = append(data.Edges, edge{
			Link:  generateNodeID(processID) + " -> " + generateNodeID(fileID),
			Color: fileColor,
		})
		ad.prepareFileNode(f, data, "", processID)
	}
	for _, child := range p.Children {
		childID := fmt.Sprintf("%s_%s_%d", child.Process.PathnameStr, child.Process.ExecTime, child.Process.Tid)
		data.Edges = append(data.Edges, edge{
			Link:  generateNodeID(processID) + " -> " + generateNodeID(childID),
			Color: processColor,
		})
		ad.prepareProcessActivityNode(child, data)
	}
}

func (ad *ActivityDump) prepareFileNode(f *FileActivityNode, data *graph, prefix string, processID string) {
	fileID := fmt.Sprintf("%s_%s_%s", processID, f.Name, prefix+f.Name)
	data.Nodes[fileID] = node{
		ID:    generateNodeID(fileID),
		Label: f.Name,
		Size:  30,
		Color: fileColor,
	}
	for _, child := range f.Children {
		childID := fmt.Sprintf("%s_%s_%s", processID, child.Name, prefix+f.Name+child.Name)
		data.Edges = append(data.Edges, edge{
			Link:  generateNodeID(fileID) + " -> " + generateNodeID(childID),
			Color: fileColor,
		})
		ad.prepareFileNode(child, data, prefix+f.Name, processID)
	}
}

func generateNodeID(input string) string {
	var id string
	for _, b := range blake2b.Sum256([]byte(input)) {
		id += fmt.Sprintf("%v", b)
	}
	return id
}
