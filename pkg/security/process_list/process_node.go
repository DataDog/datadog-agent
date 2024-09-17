// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package processlist holds process context
package processlist

import (
	"fmt"
	"io"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"golang.org/x/exp/slices"
)

// ProcessNode holds the activity of a process
type ProcessNode struct {
	sync.Mutex

	// mainly used by dump/profiles
	ImageTags []string

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

	Children []*ProcessNode

	// Used to store custom fields, depending on the owner, basically:
	// == Fields used by process resolver:
	// refCount?
	// onRelase CB?
	// (would be great if we finaly can get rid of it!)
	UserData interface{}
}

func NewProcessExecNodeFromEvent(event *model.Event) *ProcessNode {
	exec := NewExecNodeFromEvent(event)
	process := &ProcessNode{
		CurrentExec:   exec,
		PossibleExecs: []*ExecNode{exec},
	}
	exec.ProcessLink = process
	return process
}

// GetParent returns nil for the ActivityTree
func (pn *ProcessNode) GetCurrentParent() ProcessNodeIface {
	pn.Lock()
	defer pn.Unlock()

	return pn.CurrentParent
}

// GetParent returns nil for the ActivityTree
func (pn *ProcessNode) GetPossibleParents() []ProcessNodeIface {
	pn.Lock()
	defer pn.Unlock()

	return pn.PossibleParents
}

// GetChildren returns the list of children from the ProcessNode
func (pn *ProcessNode) GetChildren() *[]*ProcessNode {
	pn.Lock()
	defer pn.Unlock()

	if len(pn.Children) == 0 {
		return nil
	}
	return &pn.Children
}

// GetCurrentSiblings returns the list of siblings of the current node
func (pn *ProcessNode) GetCurrentSiblings() *[]*ProcessNode {
	pn.Lock()
	defer pn.Unlock()

	if pn.CurrentParent != nil {
		return pn.CurrentParent.GetChildren()
	}
	return nil
}

// AppendChild appends a new root node in the ActivityTree
func (pn *ProcessNode) AppendChild(child *ProcessNode, currentParent bool) {
	pn.Lock()
	defer pn.Unlock()

	pn.Children = append(pn.Children, child)
	child.PossibleParents = append(child.PossibleParents, pn)
	if currentParent || child.CurrentParent == nil {
		child.CurrentParent = pn
	}
}

// AppendChild appends a new root node in the ActivityTree
func (pn *ProcessNode) AppendExec(exec *ExecNode, currentExec bool) {
	pn.Lock()
	defer pn.Unlock()

	pn.PossibleExecs = append(pn.PossibleExecs, exec)
	exec.ProcessLink = pn
	if currentExec || pn.CurrentExec == nil {
		pn.CurrentExec = exec
	}
}

// UnlinkChild unlinks a child from the children list
func (pn *ProcessNode) UnlinkChild(owner ProcessListOwner, child *ProcessNode) bool {
	pn.Lock()
	defer pn.Unlock()

	removed := false
	pn.Children = slices.DeleteFunc(pn.Children, func(node *ProcessNode) bool {
		if owner.ProcessMatches(child, node) {
			removed = true
			return true
		}
		return false
	})
	return removed
}

// debug prints out recursively content of each node
func (pn *ProcessNode) Debug(w io.Writer, prefix string) {
	pn.Lock()
	defer pn.Unlock()

	pn.CurrentExec.Debug(w, prefix)
	prefix = prefix + "  "
	fmt.Fprintf(w, prefix+"%d children:\n", len(pn.Children))
	for _, child := range pn.Children {
		child.Debug(w, prefix)
	}
}
