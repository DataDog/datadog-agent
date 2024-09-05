// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package processcontext holds process context
package processcontext

import "io"

// ProcessNode holds the activity of a process
type ProcessNode struct {
	// for runtime cache: possible parents represents an agregated view of what we saw at runtime (ex: if a process
	// loose its parent and being attached to the closest sub-reaper, it would have 1 current parrent but
	// 2 possible ones).
	// for AD: same logic as for runtime
	CurrentParent   ProcessNodeIface
	PossibleParents []ProcessNodeIface

	// for runtime cache: possible execs represents the ancestors, in a unsorted way
	// for AD: possible execs represents, after a fork, what exec we already seen (and so,
	//         possible ones)
	CurrentExec   *ExecNode
	PossibleExecs []*ExecNode

	Children []ProcessNodeIface

	// Used to store custom fields, depending on the owner, basically:
	// == Fields used by process resolver:
	// refCount?
	// onRelase CB?
	// (would be great if we finaly can get rid of it!)
	UserData interface{}
}

// debug prints out recursively content of each node
func (pn *ProcessNode) Debug(w io.Writer, prefix string) {
	pn.CurrentExec.Debug(w, prefix)
	for child := range pn.Children {
		child.Debug(w, prefix+"  ")
	}
}

// GetParent returns nil for the ActivityTree
func (pn *ProcessNode) GetCurrentParent() ProcessNodeIface {
	return pn.CurrentParent
}

// GetParent returns nil for the ActivityTree
func (pn *ProcessNode) GetPossibleParents() []ProcessNodeIface {
	return pn.PossibleParents
}

// GetChildren returns the list of children from the ProcessNode
func (pn *ProcessNode) GetChildren() *[]ProcessNodeIface {
	return &pn.Children
}

// GetCurrentSiblings returns the list of siblings of the current node
func (pn *ProcessNode) GetCurrentSiblings() *[]ProcessNodeIface {
	if pn.CurrentParent != nil {
		return pn.CurrentParent.GetChildren()
	}
	return nil
}

// AppendChild appends a new root node in the ActivityTree
func (pn *ProcessNode) AppendChild(node *ProcessNode, currentParent bool) {
	pn.Children = append(pn.Children, node)
	node.PossibleParents = append(node.PossibleParents, pn)
	if currentParent {
		node.CurrentParent = pn
	}
}
