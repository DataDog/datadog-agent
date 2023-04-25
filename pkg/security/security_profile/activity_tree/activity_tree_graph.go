// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package activity_tree

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	processColor                = "#8fbbff"
	processSecurityProfileColor = "#c2daff"
	processRuntimeColor         = "#edf3ff"
	processSnapshotColor        = "white"
	processShape                = "record"

	fileColor                = "#77bf77"
	fileSecurityProfileColor = "#c6e1c1"
	fileRuntimeColor         = "#e9f3e7"
	fileSnapshotColor        = "white"
	fileShape                = "record"

	networkColor                = "#ff9800"
	networkSecurityProfileColor = "#faddb1"
	networkRuntimeColor         = "#ffebcd"
	networkShape                = "record"
)

// PrepareGraphData returns a graph from the activity tree
func (at *ActivityTree) PrepareGraphData(title string, resolver *process.Resolver) utils.Graph {
	data := utils.Graph{
		Title: title,
		Nodes: make(map[utils.GraphID]utils.Node),
	}

	for _, p := range at.ProcessNodes {
		at.prepareProcessNode(p, &data, resolver)
	}

	return data
}

func (at *ActivityTree) prepareProcessNode(p *ProcessNode, data *utils.Graph, resolver *process.Resolver) utils.GraphID {
	var args string
	if resolver != nil {
		if argv, _ := resolver.GetProcessScrubbedArgv(&p.Process); len(argv) > 0 {
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
	case SecurityProfile:
		pan.FillColor = processSecurityProfileColor
	case Runtime, Unknown:
		pan.FillColor = processRuntimeColor
	case Snapshot:
		pan.FillColor = processSnapshotColor
	}
	data.Nodes[panGraphID] = pan

	for _, n := range p.Sockets {
		socketNodeID := at.prepareSocketNode(n, data, panGraphID)
		data.Edges = append(data.Edges, utils.Edge{
			From:  panGraphID,
			To:    socketNodeID,
			Color: networkColor,
		})
	}

	for _, n := range p.DNSNames {
		dnsNodeID, ok := at.prepareDNSNode(n, data, panGraphID)
		if ok {
			data.Edges = append(data.Edges, utils.Edge{
				From:  panGraphID,
				To:    dnsNodeID,
				Color: networkColor,
			})
		}
	}

	for _, f := range p.Files {
		fileID := at.prepareFileNode(f, data, "", panGraphID)
		data.Edges = append(data.Edges, utils.Edge{
			From:  panGraphID,
			To:    fileID,
			Color: fileColor,
		})
	}

	if len(p.Syscalls) > 0 {
		syscallsNodeID := at.prepareSyscallsNode(p, data)
		data.Edges = append(data.Edges, utils.Edge{
			From:  utils.NewGraphID(utils.NewNodeIDFromPtr(p)),
			To:    syscallsNodeID,
			Color: processColor,
		})
	}

	for _, child := range p.Children {
		childID := at.prepareProcessNode(child, data, resolver)
		data.Edges = append(data.Edges, utils.Edge{
			From:  panGraphID,
			To:    childID,
			Color: processColor,
		})
	}

	return panGraphID
}

func (at *ActivityTree) prepareDNSNode(n *DNSNode, data *utils.Graph, processID utils.GraphID) (utils.GraphID, bool) {
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
		ID:    processID.Derive(utils.NewNodeIDFromPtr(n)),
		Label: name,
		Size:  30,
		Color: networkColor,
		Shape: networkShape,
	}
	switch n.GenerationType {
	case Runtime, Snapshot, Unknown:
		dnsNode.FillColor = networkRuntimeColor
	case SecurityProfile:
		dnsNode.FillColor = networkSecurityProfileColor
	}
	data.Nodes[dnsNode.ID] = dnsNode
	return dnsNode.ID, true
}

func (at *ActivityTree) prepareSocketNode(n *SocketNode, data *utils.Graph, processID utils.GraphID) utils.GraphID {
	targetID := processID.Derive(utils.NewNodeIDFromPtr(n))

	// prepare main socket node
	socketNode := utils.Node{
		ID:    targetID,
		Label: n.Family,
		Size:  30,
		Color: networkColor,
		Shape: networkShape,
	}

	switch n.GenerationType {
	case Runtime, Snapshot, Unknown:
		socketNode.FillColor = networkRuntimeColor
	case SecurityProfile:
		socketNode.FillColor = networkSecurityProfileColor
	}
	data.Nodes[targetID] = socketNode

	// prepare bind nodes
	for i, node := range n.Bind {
		bindNode := utils.Node{
			ID:    processID.Derive(utils.NewNodeIDFromPtr(n), utils.NewNodeID(uint64(i+1))),
			Label: fmt.Sprintf("[%s]:%d", node.IP, node.Port),
			Size:  30,
			Color: networkColor,
			Shape: networkShape,
		}

		switch node.GenerationType {
		case Runtime, Snapshot, Unknown:
			bindNode.FillColor = networkRuntimeColor
		case SecurityProfile:
			bindNode.FillColor = networkSecurityProfileColor
		}
		data.Edges = append(data.Edges, utils.Edge{
			From:  targetID,
			To:    bindNode.ID,
			Color: networkColor,
		})
		data.Nodes[bindNode.ID] = bindNode
	}

	return targetID
}

func (at *ActivityTree) prepareFileNode(f *FileNode, data *utils.Graph, prefix string, processID utils.GraphID) utils.GraphID {
	mergedID := processID.Derive(utils.NewNodeIDFromPtr(f))
	fn := utils.Node{
		ID:    mergedID,
		Label: f.getNodeLabel(),
		Size:  30,
		Color: fileColor,
		Shape: fileShape,
	}
	switch f.GenerationType {
	case SecurityProfile:
		fn.FillColor = fileSecurityProfileColor
	case Runtime, Unknown:
		fn.FillColor = fileRuntimeColor
	case Snapshot:
		fn.FillColor = fileSnapshotColor
	}
	data.Nodes[mergedID] = fn

	for _, child := range f.Children {
		childID := at.prepareFileNode(child, data, prefix+f.Name, processID)
		data.Edges = append(data.Edges, utils.Edge{
			From:  mergedID,
			To:    childID,
			Color: fileColor,
		})
	}
	return mergedID
}

func (at *ActivityTree) prepareSyscallsNode(p *ProcessNode, data *utils.Graph) utils.GraphID {
	label := fmt.Sprintf("<<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"1\">")
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
