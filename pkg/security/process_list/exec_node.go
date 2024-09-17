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
)

type ExecNode struct {
	sync.Mutex
	model.Process

	ProcessLink *ProcessNode

	MatchedRules []*model.MatchedRule

	// TODO: redo
	// Files      map[string]*FileNode
	// DNSNames   map[string]*DNSNode
	// IMDSEvents map[model.IMDSEvent]*IMDSNode
	// Sockets    []*SocketNode
	// Syscalls   []int32
}

// NewExecNode returns a new ExecNode instance
func NewEmptyExecNode() *ExecNode {
	// TODO: init maps
	return &ExecNode{}
}

func NewExecNodeFromEvent(event *model.Event) *ExecNode {
	exec := NewEmptyExecNode()
	exec.Process = event.ProcessContext.Process
	return exec
}

func (e *ExecNode) Insert(event *model.Event, imageTag string) (newEntryAdded bool, err error) {
	e.Lock()
	defer e.Unlock()

	// TODO
	return false, nil
}

// debug prints out recursively content of each node
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

// scrub args/envs
func (pl *ExecNode) Scrub() error {
	// TODO
	return nil
}
