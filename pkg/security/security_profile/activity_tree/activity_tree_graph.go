// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	processColor             = "#8fbbff"
	processProfileDriftColor = "#c2daff"
	processRuntimeColor      = "#edf3ff"
	processSnapshotColor     = "white"
	processShape             = "record"

	fileColor             = "#77bf77"
	fileProfileDriftColor = "#c6e1c1"
	fileRuntimeColor      = "#e9f3e7"
	fileSnapshotColor     = "white"
	fileShape             = "record"

	networkColor             = "#ff9800"
	networkProfileDriftColor = "#faddb1"
	networkRuntimeColor      = "#ffebcd"
	networkShape             = "record"
)

// PrepareGraphData returns a graph from the activity tree
func (at *ActivityTree) PrepareGraphData(title string, resolver *process.EBPFResolver) utils.Graph {
	data := utils.Graph{
		Title: title,
		Nodes: make(map[utils.GraphID]*utils.Node),
	}

	for _, p := range at.ProcessNodes {
		at.prepareProcessNode(p, &data, resolver)
	}

	return data
}

func (at *ActivityTree) prepareProcessNode(p *ProcessNode, data *utils.Graph, resolver *process.EBPFResolver) utils.GraphID {
	var args string
	var argv []string
	if resolver != nil {
		argv, _ = resolver.GetProcessArgvScrubbed(&p.Process)
	} else {
		argv, _ = process.GetProcessArgv(&p.Process)
	}
	if len(argv) > 0 {
		args = strings.ReplaceAll(strings.Join(argv, " "), "\"", "\\\"")
		args = strings.ReplaceAll(args, "\n", " ")
		args = strings.ReplaceAll(args, ">", "\\>")
		args = strings.ReplaceAll(args, "|", "\\|")
	}
	panGraphID := utils.NewGraphID(utils.NewNodeIDFromPtr(p))
	pan := &utils.Node{
		ID:    panGraphID,
		Label: p.getNodeLabel(args),
		Size:  60,
		Color: processColor,
		Shape: processShape,
	}
	switch p.GenerationType {
	case ProfileDrift:
		pan.FillColor = processProfileDriftColor
	case Runtime, Unknown:
		pan.FillColor = processRuntimeColor
	case Snapshot:
		pan.FillColor = processSnapshotColor
	}
	data.Nodes[panGraphID] = pan

	for _, n := range p.Sockets {
		socketNodeID := at.prepareSocketNode(n, data, panGraphID)
		data.Edges = append(data.Edges, &utils.Edge{
			From:  panGraphID,
			To:    socketNodeID,
			Color: networkColor,
		})
	}

	for _, n := range p.DNSNames {
		dnsNodeID, ok := at.prepareDNSNode(n, data, panGraphID)
		if ok {
			data.Edges = append(data.Edges, &utils.Edge{
				From:  panGraphID,
				To:    dnsNodeID,
				Color: networkColor,
			})
		}
	}

	for _, n := range p.IMDSEvents {
		imdsNodeID, ok := at.prepareIMDSNode(n, data, panGraphID)
		if ok {
			data.Edges = append(data.Edges, &utils.Edge{
				From:  panGraphID,
				To:    imdsNodeID,
				Color: networkColor,
			})
		}
	}

	for _, f := range p.Files {
		fileID := at.prepareFileNode(f, data, "", panGraphID)
		data.Edges = append(data.Edges, &utils.Edge{
			From:  panGraphID,
			To:    fileID,
			Color: fileColor,
		})
	}

	if len(p.Syscalls) > 0 {
		syscallsNodeID := at.prepareSyscallsNode(p, data)
		data.Edges = append(data.Edges, &utils.Edge{
			From:  utils.NewGraphID(utils.NewNodeIDFromPtr(p)),
			To:    syscallsNodeID,
			Color: processColor,
		})
	}

	for _, child := range p.Children {
		childID := at.prepareProcessNode(child, data, resolver)
		data.Edges = append(data.Edges, &utils.Edge{
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

	dnsNode := &utils.Node{
		ID:    processID.Derive(utils.NewNodeIDFromPtr(n)),
		Label: name,
		Size:  30,
		Color: networkColor,
		Shape: networkShape,
	}
	switch n.GenerationType {
	case Runtime, Snapshot, Unknown:
		dnsNode.FillColor = networkRuntimeColor
	case ProfileDrift:
		dnsNode.FillColor = networkProfileDriftColor
	}
	data.Nodes[dnsNode.ID] = dnsNode
	return dnsNode.ID, true
}

func (at *ActivityTree) prepareIMDSNode(n *IMDSNode, data *utils.Graph, processID utils.GraphID) (utils.GraphID, bool) {
	label := "<<TABLE BORDER=\"0\" CELLBORDER=\"2\" CELLSPACING=\"0\" CELLPADDING=\"10\">"
	label += "<TR><TD>IMDS</TD><TD>" + n.Event.Type + "</TD></TR>"
	label += "<TR><TD>Cloud provider</TD><TD>" + n.Event.CloudProvider + "</TD></TR>"
	if len(n.Event.UserAgent) > 0 {
		label += "<TR><TD>URL</TD><TD>" + n.Event.URL + "</TD></TR>"
	}
	if len(n.Event.UserAgent) > 0 {
		label += "<TR><TD>User agent</TD><TD>" + n.Event.UserAgent + "</TD></TR>"
	}
	if len(n.Event.Server) > 0 {
		label += "<TR><TD>Server</TD><TD>" + n.Event.Server + "</TD></TR>"
	}
	if len(n.Event.Host) > 0 {
		label += "<TR><TD>Host</TD><TD>" + n.Event.Host + "</TD></TR>"
	}
	if n.Event.CloudProvider == model.IMDSAWSCloudProvider {
		label += "<TR><TD>IMDSv2</TD><TD>" + fmt.Sprintf("%v", n.Event.AWS.IsIMDSv2) + "</TD></TR>"
		if len(n.Event.AWS.SecurityCredentials.AccessKeyID) > 0 {
			label += "<TR><TD> AccessKeyID </TD><TD>" + n.Event.AWS.SecurityCredentials.AccessKeyID + "</TD></TR>"
		}
	}
	label += "</TABLE>>"

	imdsNode := &utils.Node{
		ID:      processID.Derive(utils.NewNodeIDFromPtr(n)),
		Label:   label,
		Size:    30,
		Color:   networkColor,
		Shape:   networkShape,
		IsTable: true,
	}
	switch n.GenerationType {
	case Runtime, Snapshot, Unknown:
		imdsNode.FillColor = networkRuntimeColor
	case ProfileDrift:
		imdsNode.FillColor = networkProfileDriftColor
	}
	data.Nodes[imdsNode.ID] = imdsNode
	return imdsNode.ID, true
}

func (at *ActivityTree) prepareSocketNode(n *SocketNode, data *utils.Graph, processID utils.GraphID) utils.GraphID {
	targetID := processID.Derive(utils.NewNodeIDFromPtr(n))

	// prepare main socket node
	socketNode := &utils.Node{
		ID:    targetID,
		Label: n.Family,
		Size:  30,
		Color: networkColor,
		Shape: networkShape,
	}

	switch n.GenerationType {
	case Runtime, Snapshot, Unknown:
		socketNode.FillColor = networkRuntimeColor
	case ProfileDrift:
		socketNode.FillColor = networkProfileDriftColor
	}
	data.Nodes[targetID] = socketNode

	// prepare bind nodes
	for i, node := range n.Bind {
		bindNode := &utils.Node{
			ID:    processID.Derive(utils.NewNodeIDFromPtr(n), utils.NewNodeID(uint64(i+1))),
			Label: fmt.Sprintf("[%s]:%d", node.IP, node.Port),
			Size:  30,
			Color: networkColor,
			Shape: networkShape,
		}

		switch node.GenerationType {
		case Runtime, Snapshot, Unknown:
			bindNode.FillColor = networkRuntimeColor
		case ProfileDrift:
			bindNode.FillColor = networkProfileDriftColor
		}
		data.Edges = append(data.Edges, &utils.Edge{
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
	fn := &utils.Node{
		ID:    mergedID,
		Label: f.getNodeLabel(),
		Size:  30,
		Color: fileColor,
		Shape: fileShape,
	}
	switch f.GenerationType {
	case ProfileDrift:
		fn.FillColor = fileProfileDriftColor
	case Runtime, Unknown:
		fn.FillColor = fileRuntimeColor
	case Snapshot:
		fn.FillColor = fileSnapshotColor
	}
	data.Nodes[mergedID] = fn

	for _, child := range f.Children {
		childID := at.prepareFileNode(child, data, prefix+f.Name, processID)
		data.Edges = append(data.Edges, &utils.Edge{
			From:  mergedID,
			To:    childID,
			Color: fileColor,
		})
	}
	return mergedID
}

func (at *ActivityTree) prepareSyscallsNode(p *ProcessNode, data *utils.Graph) utils.GraphID {
	label := "<<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"5\">"
	for _, s := range p.Syscalls {
		label += "<TR><TD>" + model.Syscall(s.Syscall).String() + "</TD></TR>"
	}
	label += "</TABLE>>"

	syscallsNode := &utils.Node{
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
