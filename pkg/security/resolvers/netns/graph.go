// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"fmt"
	"os"
	"text/template"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	namespaceColor        = "#8fbbff"
	lonelyNamespaceColor  = "#edf3ff"
	activeNamespacetColor = "white"
	namespaceShape        = "record"

	deviceColor       = "#77bf77"
	queuedDeviceColor = "#e9f3e7"
	activeDeviceColor = "white"
	deviceShape       = "record"
)

func (nr *Resolver) generateGraph(dump []NetworkNamespaceDump, graphFile *os.File) error {
	if graphFile == nil {
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
		{{ .ID }} [label="{{ .Label }}", fontsize={{ .Size }}, shape=record, fontname = "arial", color="{{ .Color }}", fillcolor="{{ .FillColor }}", style="filled"]
		{{ end }}

		{{ range .Edges }}
		{{ .From }} -> {{ .To }} [arrowhead=none, color="{{ .Color }}"]
		{{ end }}
}`

	data := nr.generateGraphDataFromDump(dump)
	t := template.Must(template.New("tmpl").Parse(tmpl))
	return t.Execute(graphFile, data)
}

func (nr *Resolver) generateGraphDataFromDump(dump []NetworkNamespaceDump) utils.Graph {
	g := utils.Graph{
		Title: fmt.Sprintf("Network Namespace Dump (%s)", time.Now().Format("2006-01-02 15:04:05")),
		Nodes: make(map[utils.GraphID]utils.Node),
	}

	for _, netns := range dump {
		// create namespace node
		netnsNode := utils.Node{
			ID:    utils.NewGraphID(utils.NewRandomNodeID()),
			Label: fmt.Sprintf("%v [fd:%d][handle:%v]", netns.NsID, netns.HandleFD, netns.HandlePath),
			Color: namespaceColor,
			Shape: namespaceShape,
			Size:  60,
		}
		if netns.LonelyTimeout.Equal(time.Time{}) {
			netnsNode.FillColor = activeNamespacetColor
		} else {
			netnsNode.FillColor = lonelyNamespaceColor
		}
		g.Nodes[netnsNode.ID] = netnsNode

		// create active and queued devices nodes
		for _, dev := range netns.Devices {
			devNode := utils.Node{
				ID:        utils.NewGraphID(utils.NewRandomNodeID()),
				Label:     fmt.Sprintf("%s [%d]", dev.IfName, dev.IfIndex),
				FillColor: activeDeviceColor,
				Color:     deviceColor,
				Shape:     deviceShape,
				Size:      50,
			}
			g.Nodes[devNode.ID] = devNode

			devEdge := utils.Edge{
				From:  netnsNode.ID,
				To:    devNode.ID,
				Color: namespaceColor,
			}
			g.Edges = append(g.Edges, devEdge)
		}
		for _, dev := range netns.DevicesInQueue {
			devNode := utils.Node{
				ID:        utils.NewGraphID(utils.NewRandomNodeID()),
				Label:     fmt.Sprintf("%s [%d]", dev.IfName, dev.IfIndex),
				FillColor: queuedDeviceColor,
				Color:     deviceColor,
				Shape:     deviceShape,
				Size:      50,
			}
			g.Nodes[devNode.ID] = devNode

			devEdge := utils.Edge{
				From:  netnsNode.ID,
				To:    devNode.ID,
				Color: namespaceColor,
			}
			g.Edges = append(g.Edges, devEdge)
		}
	}

	return g
}
