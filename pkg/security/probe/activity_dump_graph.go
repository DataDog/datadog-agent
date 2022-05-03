// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/tinylib/msgp/msgp"
)

var (
	processColor         = "#8fbbff"
	processRuntimeColor  = "#edf3ff"
	processSnapshotColor = "white"

	fileColor         = "#77bf77"
	fileRuntimeColor  = "#e9f3e7"
	fileSnapshotColor = "white"
)

type node struct {
	ID        string
	Label     string
	Size      int
	Color     string
	FillColor string
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

func (ad *ActivityDump) generateGraph() error {
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
		{{ .ID }} [label="{{ .Label }}", fontsize={{ .Size }}, shape=record, fontname = "arial", color="{{ .Color }}", fillcolor="{{ .FillColor }}", style="filled"]{{ end }}

		{{ range .Edges }}
		{{ .Link }} [arrowhead=none, color="{{ .Color }}"]
		{{ end }}
}`

	title := fmt.Sprintf("Activity tree: %s", ad.GetSelectorStr())
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
	var args string
	if p.Process.ArgsEntry != nil {
		args = strings.ReplaceAll(strings.Join(p.Process.ArgsEntry.Values, " "), "\"", "\\\"")
		args = strings.ReplaceAll(args, "\n", " ")
		args = strings.ReplaceAll(args, ">", "\\>")
		args = strings.ReplaceAll(args, "|", "\\|")
	}
	pan := node{
		ID:    p.GetID(),
		Label: fmt.Sprintf("%s %s", p.Process.FileEvent.PathnameStr, args),
		Size:  60,
		Color: processColor,
	}
	switch p.GenerationType {
	case Runtime:
		pan.FillColor = processRuntimeColor
	case Snapshot:
		pan.FillColor = processSnapshotColor
	}
	data.Nodes[p.GetID()] = pan

	for _, f := range p.Files {
		data.Edges = append(data.Edges, edge{
			Link:  p.GetID() + " -> " + p.GetID() + f.GetID(),
			Color: fileColor,
		})
		ad.prepareFileNode(f, data, "", p.GetID())
	}
	for _, child := range p.Children {
		data.Edges = append(data.Edges, edge{
			Link:  p.GetID() + " -> " + child.GetID(),
			Color: processColor,
		})
		ad.prepareProcessActivityNode(child, data)
	}
}

func (ad *ActivityDump) prepareFileNode(f *FileActivityNode, data *graph, prefix string, processID string) {
	mergedID := processID + f.GetID()
	fn := node{
		ID:    mergedID,
		Label: f.getNodeLabel(),
		Size:  30,
		Color: fileColor,
	}
	switch f.GenerationType {
	case Runtime:
		fn.FillColor = fileRuntimeColor
	case Snapshot:
		fn.FillColor = fileSnapshotColor
	}
	data.Nodes[mergedID] = fn

	for _, child := range f.Children {
		data.Edges = append(data.Edges, edge{
			Link:  mergedID + " -> " + processID + child.GetID(),
			Color: fileColor,
		})
		ad.prepareFileNode(child, data, prefix+f.Name, processID)
	}
}

// GenerateGraph creates a graph from the input activity dump
func GenerateGraph(inputFile string) (string, error) {
	// open and parse activity dump file
	f, err := os.Open(inputFile)
	if err != nil {
		return "", fmt.Errorf("couldn't open activity dump file: %w", err)
	}

	var dump ActivityDump
	msgpReader := msgp.NewReader(f)
	err = dump.DecodeMsg(msgpReader)
	if err != nil {
		return "", fmt.Errorf("couldn't parse activity dump file: %w", err)
	}

	// create profile output file
	dump.graphFile, err = os.CreateTemp("/tmp", "graph-")
	if err != nil {
		return "", fmt.Errorf("couldn't create profile file: %w", err)
	}

	if err = os.Chmod(dump.graphFile.Name(), 0400); err != nil {
		return "", fmt.Errorf("couldn't change the mode of the profile file: %w", err)
	}

	if err = dump.generateGraph(); err != nil {
		return "", fmt.Errorf("couldn't generate graph from activity dump %s: %w", inputFile, err)
	}

	return dump.graphFile.Name(), nil
}
