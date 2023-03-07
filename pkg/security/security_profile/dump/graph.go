// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dump

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	processColor         = "#8fbbff"
	processRuntimeColor  = "#edf3ff"
	processSnapshotColor = "white"
	processShape         = "record"

	fileColor         = "#77bf77"
	fileRuntimeColor  = "#e9f3e7"
	fileSnapshotColor = "white"
	fileShape         = "record"

	networkColor        = "#ff9800"
	networkRuntimeColor = "#ffebcd"
	networkShape        = "record"
)

// GraphTemplate is the template used to generate graphs
var GraphTemplate = `digraph {
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

// EncodeDOT encodes an activity dump in the DOT format
func (ad *ActivityDump) EncodeDOT() (*bytes.Buffer, error) {
	ad.Lock()
	defer ad.Unlock()

	title := fmt.Sprintf("%s: %s", ad.Metadata.Name, ad.getSelectorStr())
	data := ad.prepareGraphData(title)
	t := template.Must(template.New("tmpl").Parse(GraphTemplate))
	raw := bytes.NewBuffer(nil)
	if err := t.Execute(raw, data); err != nil {
		return nil, fmt.Errorf("couldn't encode %s in %s: %w", ad.getSelectorStr(), config.DOT, err)
	}
	return raw, nil
}

func (ad *ActivityDump) prepareGraphData(title string) utils.Graph {
	data := utils.Graph{
		Title: title,
		Nodes: make(map[utils.GraphID]utils.Node),
	}

	for _, p := range ad.ProcessActivityTree {
		ad.prepareProcessActivityNode(p, &data)
	}

	return data
}

func (ad *ActivityDump) prepareProcessActivityNode(p *ProcessActivityNode, data *utils.Graph) utils.GraphID {
	var args string
	if ad.adm != nil && ad.adm.processResolver != nil {
		if argv, _ := ad.adm.processResolver.GetProcessScrubbedArgv(&p.Process); len(argv) > 0 {
			args = strings.ReplaceAll(strings.Join(argv, " "), "\"", "\\\"")
			args = strings.ReplaceAll(args, "\n", " ")
			args = strings.ReplaceAll(args, ">", "\\>")
			args = strings.ReplaceAll(args, "|", "\\|")
		}
	}
	panGraphID := utils.NewGraphID(utils.NewNodeIDFromPtr(p))
	pan := utils.Node{
		ID:    panGraphID,
		Label: p.getNodeLabel(args),
		Size:  60,
		Color: processColor,
		Shape: processShape,
	}
	switch p.GenerationType {
	case Runtime, Unknown:
		pan.FillColor = processRuntimeColor
	case Snapshot:
		pan.FillColor = processSnapshotColor
	}
	data.Nodes[panGraphID] = pan

	for _, n := range p.Sockets {
		socketNodeID := ad.prepareSocketNode(n, data, panGraphID)
		data.Edges = append(data.Edges, utils.Edge{
			From:  panGraphID,
			To:    socketNodeID,
			Color: networkColor,
		})
	}

	for _, n := range p.DNSNames {
		dnsNodeID, ok := ad.prepareDNSNode(n, data, panGraphID)
		if ok {
			data.Edges = append(data.Edges, utils.Edge{
				From:  panGraphID,
				To:    dnsNodeID,
				Color: networkColor,
			})
		}
	}

	for _, f := range p.Files {
		fileID := ad.prepareFileNode(f, data, "", panGraphID)
		data.Edges = append(data.Edges, utils.Edge{
			From:  panGraphID,
			To:    fileID,
			Color: fileColor,
		})
	}

	if len(p.Syscalls) > 0 {
		syscallsNodeID := ad.prepareSyscallsNode(p, data)
		data.Edges = append(data.Edges, utils.Edge{
			From:  utils.NewGraphID(utils.NewNodeIDFromPtr(p)),
			To:    syscallsNodeID,
			Color: processColor,
		})
	}

	for _, child := range p.Children {
		childID := ad.prepareProcessActivityNode(child, data)
		data.Edges = append(data.Edges, utils.Edge{
			From:  panGraphID,
			To:    childID,
			Color: processColor,
		})
	}

	return panGraphID
}

func (ad *ActivityDump) prepareDNSNode(n *DNSNode, data *utils.Graph, processID utils.GraphID) (utils.GraphID, bool) {
	if len(n.Requests) == 0 {
		// save guard, this should never happen
		return utils.GraphID{}, false
	}
	name := n.Requests[0].Name + " (" + (model.QType(n.Requests[0].Type).String())
	for _, req := range n.Requests[1:] {
		name += ", " + model.QType(req.Type).String()
	}
	name += ")"

	dnsNode := utils.Node{
		ID:        processID.Derive(utils.NewNodeIDFromPtr(n)),
		Label:     name,
		Size:      30,
		Color:     networkColor,
		FillColor: networkRuntimeColor,
		Shape:     networkShape,
	}
	data.Nodes[dnsNode.ID] = dnsNode
	return dnsNode.ID, true
}

func (ad *ActivityDump) prepareSocketNode(n *SocketNode, data *utils.Graph, processID utils.GraphID) utils.GraphID {
	targetID := processID.Derive(utils.NewNodeIDFromPtr(n))

	// prepare main socket node
	data.Nodes[targetID] = utils.Node{
		ID:        targetID,
		Label:     n.Family,
		Size:      30,
		Color:     networkColor,
		FillColor: networkRuntimeColor,
		Shape:     networkShape,
	}

	// prepare bind nodes
	var names []string
	for _, node := range n.Bind {
		names = append(names, fmt.Sprintf("[%s]:%d", node.IP, node.Port))
	}

	for i, name := range names {
		socketNode := utils.Node{
			ID:        processID.Derive(utils.NewNodeIDFromPtr(n), utils.NewNodeID(uint64(i+1))),
			Label:     name,
			Size:      30,
			Color:     networkColor,
			FillColor: networkRuntimeColor,
			Shape:     networkShape,
		}
		data.Edges = append(data.Edges, utils.Edge{
			From:  targetID,
			To:    socketNode.ID,
			Color: networkColor,
		})
		data.Nodes[socketNode.ID] = socketNode
	}

	return targetID
}

func (ad *ActivityDump) prepareFileNode(f *FileActivityNode, data *utils.Graph, prefix string, processID utils.GraphID) utils.GraphID {
	mergedID := processID.Derive(utils.NewNodeIDFromPtr(f))
	fn := utils.Node{
		ID:    mergedID,
		Label: f.getNodeLabel(),
		Size:  30,
		Color: fileColor,
		Shape: fileShape,
	}
	switch f.GenerationType {
	case Runtime, Unknown:
		fn.FillColor = fileRuntimeColor
	case Snapshot:
		fn.FillColor = fileSnapshotColor
	}
	data.Nodes[mergedID] = fn

	for _, child := range f.Children {
		childID := ad.prepareFileNode(child, data, prefix+f.Name, processID)
		data.Edges = append(data.Edges, utils.Edge{
			From:  mergedID,
			To:    childID,
			Color: fileColor,
		})
	}
	return mergedID
}

func (ad *ActivityDump) prepareSyscallsNode(p *ProcessActivityNode, data *utils.Graph) utils.GraphID {
	label := fmt.Sprintf("<<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"1\"> <TR><TD><b>arch: %s</b></TD></TR>", ad.Arch)
	for _, s := range p.Syscalls {
		label += "<TR><TD>" + model.Syscall(s).String() + "</TD></TR>"
	}
	label += "</TABLE>>"

	syscallsNode := utils.Node{
		ID:        utils.NewGraphIDWithDescription("syscalls", utils.NewNodeIDFromPtr(p)),
		Label:     label,
		Size:      30,
		Color:     processColor,
		FillColor: processSnapshotColor,
		Shape:     processShape,
		IsTable:   true,
	}
	data.Nodes[syscallsNode.ID] = syscallsNode
	return syscallsNode.ID

}
