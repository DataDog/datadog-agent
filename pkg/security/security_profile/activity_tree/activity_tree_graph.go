// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	bigText     = 10
	mediumText  = 7
	smallText   = 5
	tableHeader = "<<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"1\">"

	processColor             = "#8fbbff"
	processProfileDriftColor = "#c2daff"
	processRuntimeColor      = "#edf3ff"
	processSnapshotColor     = "white"
	processShape             = "record"
	//nolint:deadcode,unused
	processClusterColor = "#c7ddff"

	processCategoryColor = "#c7c7c7"
	//nolint:deadcode,unused
	processCategoryProfileDriftColor = "#e0e0e0"
	//nolint:deadcode,unused
	processCategoryRuntimeColor  = "#f5f5f5"
	processCategorySnapshotColor = "white"
	processCategoryShape         = "record"
	processCategoryClusterColor  = "#e3e3e3"

	fileColor             = "#77bf77"
	fileProfileDriftColor = "#c6e1c1"
	fileRuntimeColor      = "#e9f3e7"
	fileSnapshotColor     = "white"
	fileShape             = "record"
	fileClusterColor      = "#c2f2c2"

	networkColor             = "#ff9800"
	networkProfileDriftColor = "#faddb1"
	networkRuntimeColor      = "#ffebcd"
	networkShape             = "record"
	networkClusterColor      = "#fff5e6"
)

func (at *ActivityTree) getGraphTitle(name string, selector string) string {
	title := tableHeader
	title += "<TR><TD>Name</TD><TD><FONT POINT-SIZE=\"" + strconv.Itoa(bigText) + "\">" + name + "</FONT></TD></TR>"
	for i, t := range strings.Split(selector, ",") {
		if i%3 == 0 {
			if i != 0 {
				title += "</TD></TR>"
			}
			title += "<TR>"
			if i == 0 {
				title += "<TD>Selector</TD>"
			} else {
				title += "<TD></TD>"
			}
			title += "<TD>"
		} else {
			title += ", "
		}
		title += t
	}
	title += "</TD></TR>"
	title += "</TABLE>>"
	return title
}

// PrepareGraphData returns a graph from the activity tree
func (at *ActivityTree) PrepareGraphData(name string, selector string, resolver *process.EBPFResolver) utils.Graph {
	data := utils.Graph{
		Title: at.getGraphTitle(name, selector),
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
		ID:      panGraphID,
		Label:   p.getNodeLabel(args),
		Size:    smallText,
		Color:   processColor,
		Shape:   processShape,
		IsTable: true,
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

	if len(p.Files) > 0 {
		// create new subgraph for the filesystem events
		subgraph := utils.SubGraph{
			Nodes:     make(map[utils.GraphID]*utils.Node),
			Title:     "Filesystem",
			TitleSize: mediumText,
			Color:     fileClusterColor,
			Name:      "cluster_" + panGraphID.Derive(utils.NewRandomNodeID()).String(),
		}

		for _, f := range p.Files {
			fileID := at.prepareFileNode(f, &subgraph, panGraphID)
			data.Edges = append(data.Edges, &utils.Edge{
				From:  panGraphID,
				To:    fileID,
				Color: fileColor,
			})
		}

		// add subgraph
		data.SubGraphs = append(data.SubGraphs, &subgraph)
	}

	for _, n := range p.NetworkDevices {
		// create new subgraph for network device
		subgraph := utils.SubGraph{
			Nodes:     make(map[utils.GraphID]*utils.Node),
			Title:     "Network Flows",
			TitleSize: mediumText,
		}
		deviceNodeID, ok := at.prepareNetworkDeviceNode(n, &subgraph, panGraphID)
		if ok {
			subgraph.Name = "cluster_" + deviceNodeID.String()
			subgraph.Color = networkClusterColor

			data.Edges = append(data.Edges, &utils.Edge{
				From:  panGraphID,
				To:    deviceNodeID,
				Color: networkColor,
			})

			// build network flow nodes
			for _, flowNode := range n.FlowNodes {
				at.prepareNetworkFlowNodes(flowNode, &subgraph, deviceNodeID)
			}

			// add subgraph
			data.SubGraphs = append(data.SubGraphs, &subgraph)
		}
	}

	if len(p.Syscalls) > 0 {
		// create new subgraph for syscalls
		subgraph := utils.SubGraph{
			Nodes:     make(map[utils.GraphID]*utils.Node),
			Title:     "Syscalls",
			TitleSize: mediumText,
			Color:     processCategoryClusterColor,
		}

		syscallsNodeID := at.prepareSyscallsNode(p, &subgraph)
		subgraph.Name = "cluster_" + syscallsNodeID.String()
		data.Edges = append(data.Edges, &utils.Edge{
			From:  utils.NewGraphID(utils.NewNodeIDFromPtr(p)),
			To:    syscallsNodeID,
			Color: processCategoryColor,
		})

		// add subgraph
		data.SubGraphs = append(data.SubGraphs, &subgraph)
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
		Size:  smallText,
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
	label := tableHeader
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
		Size:    smallText,
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

func (at *ActivityTree) prepareNetworkDeviceNode(n *NetworkDeviceNode, data *utils.SubGraph, processID utils.GraphID) (utils.GraphID, bool) {
	label := tableHeader
	label += "<TR><TD>Device name</TD><TD>" + n.Context.IfName + "</TD></TR>"
	label += "<TR><TD>Index</TD><TD>" + strconv.Itoa(int(n.Context.IfIndex)) + "</TD></TR>"
	label += "<TR><TD>Network namespace</TD><TD>" + strconv.Itoa(int(n.Context.NetNS)) + "</TD></TR>"
	label += "</TABLE>>"

	deviceNode := &utils.Node{
		ID:      processID.Derive(utils.NewNodeIDFromPtr(n)),
		Label:   label,
		Size:    smallText,
		Color:   networkColor,
		Shape:   networkShape,
		IsTable: true,
	}

	switch n.GenerationType {
	case Runtime, Snapshot, Unknown:
		deviceNode.FillColor = networkRuntimeColor
	case ProfileDrift:
		deviceNode.FillColor = networkProfileDriftColor
	}
	data.Nodes[deviceNode.ID] = deviceNode
	return deviceNode.ID, true
}

func (at *ActivityTree) prepareNetworkFlowNodes(n *FlowNode, data *utils.SubGraph, deviceID utils.GraphID) bool {
	if len(n.Flows) == 0 {
		return false
	}

	for _, flow := range n.Flows {
		label := tableHeader
		label += "<TR><TD>Source</TD><TD>" + fmt.Sprintf("%s:%d", flow.Source.IPNet.String(), flow.Source.Port) + "</TD></TR>"
		if flow.Source.IsPublicResolved {
			label += "<TR><TD>Is src public ?</TD><TD>" + strconv.FormatBool(flow.Source.IsPublic) + "</TD></TR>"
		}
		label += "<TR><TD>Destination</TD><TD>" + fmt.Sprintf("%s:%d", flow.Destination.IPNet.String(), flow.Destination.Port) + "</TD></TR>"
		if flow.Destination.IsPublicResolved {
			label += "<TR><TD>Is dst public ?</TD><TD>" + strconv.FormatBool(flow.Destination.IsPublic) + "</TD></TR>"
		}
		label += "<TR><TD>L4 protocol</TD><TD>" + model.L4Protocol(flow.L4Protocol).String() + "</TD></TR>"
		label += "<TR><TD>Egress</TD><TD>" + strconv.Itoa(int(flow.Egress.DataSize)) + " bytes / " + strconv.Itoa(int(flow.Egress.PacketCount)) + " pkts</TD></TR>"
		label += "<TR><TD>Ingress</TD><TD>" + strconv.Itoa(int(flow.Ingress.DataSize)) + " bytes / " + strconv.Itoa(int(flow.Ingress.PacketCount)) + " pkts</TD></TR>"
		label += "</TABLE>>"

		flowNode := &utils.Node{
			ID:      deviceID.Derive(utils.NewNodeIDFromPtr(&flow.Source)),
			Label:   label,
			Size:    smallText,
			Color:   networkColor,
			Shape:   networkShape,
			IsTable: true,
		}

		switch n.GenerationType {
		case Runtime, Snapshot, Unknown:
			flowNode.FillColor = networkRuntimeColor
		case ProfileDrift:
			flowNode.FillColor = networkProfileDriftColor
		}
		data.Nodes[flowNode.ID] = flowNode

		data.Edges = append(data.Edges, &utils.Edge{
			From:  deviceID,
			To:    flowNode.ID,
			Color: networkColor,
		})
	}

	return true
}

func (at *ActivityTree) prepareSocketNode(n *SocketNode, data *utils.Graph, processID utils.GraphID) utils.GraphID {
	targetID := processID.Derive(utils.NewNodeIDFromPtr(n))

	// prepare main socket node
	socketNode := &utils.Node{
		ID:    targetID,
		Label: n.Family,
		Size:  smallText,
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
			Size:  smallText,
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

func (at *ActivityTree) prepareFileNode(f *FileNode, data *utils.SubGraph, processID utils.GraphID) utils.GraphID {
	mergedID := processID.Derive(utils.NewNodeIDFromPtr(f))
	fn := &utils.Node{
		ID:      mergedID,
		Label:   f.getNodeLabel(""),
		Size:    smallText,
		Color:   fileColor,
		Shape:   fileShape,
		IsTable: true,
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
	return mergedID
}

func (at *ActivityTree) prepareSyscallsNode(p *ProcessNode, data *utils.SubGraph) utils.GraphID {
	label := tableHeader
	for i, s := range p.Syscalls {
		if i%5 == 0 {
			if i != 0 {
				label += "</TD></TR>"
			}
			label += "<TR><TD>"
		} else {
			label += ", "
		}
		label += model.Syscall(s.Syscall).String()
	}
	label += "</TD></TR>"
	label += "</TABLE>>"

	syscallsNode := &utils.Node{
		ID:        utils.NewGraphIDWithDescription("syscalls", utils.NewNodeIDFromPtr(p)),
		Label:     label,
		Size:      smallText,
		Color:     processCategoryColor,
		FillColor: processCategorySnapshotColor,
		Shape:     processCategoryShape,
		IsTable:   true,
	}
	data.Nodes[syscallsNode.ID] = syscallsNode
	return syscallsNode.ID

}
