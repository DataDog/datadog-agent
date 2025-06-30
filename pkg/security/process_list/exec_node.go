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
	"math/rand"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ExecNode defines an exec
type ExecNode struct {
	sync.Mutex
	model.Process

	// Key represents the key used to retrieve the exec from the cache
	// if the owner is able to define a key we use it, otherwise we'll put
	// a random generated uint64 cookie
	Key interface{}

	ProcessLink *ProcessNode

	MatchedRules []*model.MatchedRule

	// TODO: redo
	// Files      map[string]*FileNode
	// DNSNames   map[string]*DNSNode
	// IMDSEvents map[model.IMDSEvent]*IMDSNode
	// Sockets    []*SocketNode
	// Syscalls   []int32
}

// NewEmptyExecNode returns a new empty ExecNode instance
func NewEmptyExecNode() *ExecNode {
	// TODO: init maps
	return &ExecNode{}
}

// NewExecNodeFromEvent returns a new exec node from a given event, and if any, use
// the provided key to assign it (otherwise it will choose a random one)
func NewExecNodeFromEvent(event *model.Event, key interface{}) *ExecNode {
	if key == nil {
		key = rand.Uint64()
	}
	exec := NewEmptyExecNode()
	exec.Process = event.ProcessContext.Process
	exec.Key = key
	return exec
}

// Insert will inserts the given event to the exec node, returns true if an entry were inserted
// nolint: all
func (e *ExecNode) Insert(event *model.Event, imageTag string) (newEntryAdded bool, err error) {
	e.Lock()
	defer e.Unlock()

	// TODO
	return false, nil
}

// Debug prints out recursively content of each node
func (e *ExecNode) Debug(w io.Writer, prefix string) {
	e.Lock()
	defer e.Unlock()

	fmt.Fprintf(w, prefix+"- process: %s\n", e.FileEvent.PathnameStr)
	prefix = prefix + "  "
	fmt.Fprintf(w, prefix+"PID: %v\n", e.Pid)
	fmt.Fprintf(w, prefix+"PPID: %v\n", e.PPid)
	// fmt.Fprintf(w, prefix+"args: %v\n", e.Args)
	// fmt.Fprintf(w, prefix+"envs: %v\n", e.Envs)
	// fmt.Fprintf(w, prefix+"files: %v\n")
	// prefix = prefix + "  . "
	// for file := range e.Files {
	// 	file.Debug(w, prefix)
	// }
	// TODO: rest of events
}

// Scrub scrubs args and envs
// nolint: all
func (e *ExecNode) Scrub() error {
	// TODO
	return nil
}
