// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
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

type node struct {
	ID        GraphID
	Label     string
	Size      int
	Color     string
	FillColor string
	Shape     string
	IsTable   bool
}

type edge struct {
	From  GraphID
	To    GraphID
	Color string
}

type graph struct {
	Title string
	Nodes map[GraphID]node
	Edges []edge
}

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

	title := fmt.Sprintf("%s: %s", ad.DumpMetadata.Name, ad.getSelectorStr())
	data := ad.prepareGraphData(title)
	t := template.Must(template.New("tmpl").Parse(GraphTemplate))
	raw := bytes.NewBuffer(nil)
	if err := t.Execute(raw, data); err != nil {
		return nil, fmt.Errorf("couldn't encode %s in %s: %w", ad.getSelectorStr(), config.DOT, err)
	}
	return raw, nil
}

func (ad *ActivityDump) prepareGraphData(title string) graph {
	data := graph{
		Title: title,
		Nodes: make(map[GraphID]node),
	}

	for _, p := range ad.ProcessActivityTree {
		ad.prepareProcessActivityNode(p, &data)
	}

	return data
}

func (ad *ActivityDump) prepareProcessActivityNode(p *ProcessActivityNode, data *graph) GraphID {
	var args string
	if ad.adm != nil && ad.adm.resolvers != nil {
		if argv, _ := ad.adm.resolvers.ProcessResolver.GetProcessScrubbedArgv(&p.Process); len(argv) > 0 {
			args = strings.ReplaceAll(strings.Join(argv, " "), "\"", "\\\"")
			args = strings.ReplaceAll(args, "\n", " ")
			args = strings.ReplaceAll(args, ">", "\\>")
			args = strings.ReplaceAll(args, "|", "\\|")
		}
	}
	panGraphID := NewGraphID(NewNodeIDFromPtr(p))
	pan := node{
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
		data.Edges = append(data.Edges, edge{
			From:  panGraphID,
			To:    socketNodeID,
			Color: networkColor,
		})
	}

	for _, n := range p.DNSNames {
		dnsNodeID, ok := ad.prepareDNSNode(n, data, panGraphID)
		if ok {
			data.Edges = append(data.Edges, edge{
				From:  panGraphID,
				To:    dnsNodeID,
				Color: networkColor,
			})
		}
	}

	for _, f := range p.Files {
		fileID := ad.prepareFileNode(f, data, "", panGraphID)
		data.Edges = append(data.Edges, edge{
			From:  panGraphID,
			To:    fileID,
			Color: fileColor,
		})
	}

	if len(p.Syscalls) > 0 {
		syscallsNodeID := ad.prepareSyscallsNode(p, data)
		data.Edges = append(data.Edges, edge{
			From:  NewGraphID(NewNodeIDFromPtr(p)),
			To:    syscallsNodeID,
			Color: processColor,
		})
	}

	for _, child := range p.Children {
		childID := ad.prepareProcessActivityNode(child, data)
		data.Edges = append(data.Edges, edge{
			From:  panGraphID,
			To:    childID,
			Color: processColor,
		})
	}

	return panGraphID
}

func (ad *ActivityDump) prepareDNSNode(n *DNSNode, data *graph, processID GraphID) (GraphID, bool) {
	if len(n.Requests) == 0 {
		// save guard, this should never happen
		return GraphID{}, false
	}
	name := n.Requests[0].Name + " (" + (model.QType(n.Requests[0].Type).String())
	for _, req := range n.Requests[1:] {
		name += ", " + model.QType(req.Type).String()
	}
	name += ")"

	dnsNode := node{
		ID:        processID.derive(NewNodeIDFromPtr(n)),
		Label:     name,
		Size:      30,
		Color:     networkColor,
		FillColor: networkRuntimeColor,
		Shape:     networkShape,
	}
	data.Nodes[dnsNode.ID] = dnsNode
	return dnsNode.ID, true
}

func (ad *ActivityDump) prepareSocketNode(n *SocketNode, data *graph, processID GraphID) GraphID {
	targetID := processID.derive(NewNodeIDFromPtr(n))

	// prepare main socket node
	data.Nodes[targetID] = node{
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
		socketNode := node{
			ID:        processID.derive(NewNodeIDFromPtr(n), NodeID{inner: uint64(i + 1)}),
			Label:     name,
			Size:      30,
			Color:     networkColor,
			FillColor: networkRuntimeColor,
			Shape:     networkShape,
		}
		data.Edges = append(data.Edges, edge{
			From:  targetID,
			To:    socketNode.ID,
			Color: networkColor,
		})
		data.Nodes[socketNode.ID] = socketNode
	}

	return targetID
}

func (ad *ActivityDump) prepareFileNode(f *FileActivityNode, data *graph, prefix string, processID GraphID) GraphID {
	mergedID := processID.derive(NewNodeIDFromPtr(f))
	fn := node{
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
		data.Edges = append(data.Edges, edge{
			From:  mergedID,
			To:    childID,
			Color: fileColor,
		})
	}
	return mergedID
}

func (ad *ActivityDump) prepareSyscallsNode(p *ProcessActivityNode, data *graph) GraphID {
	label := fmt.Sprintf("<<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"1\"> <TR><TD><b>arch: %s</b></TD></TR>", ad.Arch)
	for _, s := range p.Syscalls {
		label += "<TR><TD>" + model.Syscall(s).String() + "</TD></TR>"
	}
	label += "</TABLE>>"

	syscallsNode := node{
		ID:        NewGraphIDWithDescription("syscalls", NewNodeIDFromPtr(p)),
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

// GraphID represents an ID used in a graph, combination of NodeIDs
type GraphID struct {
	raw string
}

// NewGraphID returns a new GraphID based on the provided NodeIDs
func NewGraphID(id NodeID) GraphID {
	return NewGraphIDWithDescription("", id)
}

// NewGraphIDWithDescription returns a new GraphID based on a description and on the provided NodeIDs
func NewGraphIDWithDescription(description string, id NodeID) GraphID {
	if description == "" {
		description = "node"
	}
	return GraphID{
		raw: fmt.Sprintf("%s_%d", description, id.inner),
	}
}

func (id *GraphID) derive(ids ...NodeID) GraphID {
	var builder strings.Builder
	builder.WriteString(id.raw)
	for _, sub := range ids {
		builder.WriteString(fmt.Sprintf("_%d", sub.inner))
	}
	return GraphID{
		raw: builder.String(),
	}
}

func (id GraphID) String() string {
	return id.raw
}

// NodeID represents the ID of a Node
type NodeID struct {
	inner uint64
}

// NewRandomNodeID returns a new random NodeID
func NewRandomNodeID() NodeID {
	return NodeID{
		inner: eval.RandNonZeroUint64(),
	}
}

// NewNodeIDFromPtr returns a new NodeID based on a pointer value
func NewNodeIDFromPtr[T any](v *T) NodeID {
	ptr := uintptr(unsafe.Pointer(v))
	return NodeID{
		inner: uint64(ptr),
	}
}

// IsUnset checks if the NodeID is unset
func (id NodeID) IsUnset() bool {
	return id.inner == 0
}
