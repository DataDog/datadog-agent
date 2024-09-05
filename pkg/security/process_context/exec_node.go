// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package processcontext holds process context
package processcontext

import (
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type ExecNode struct {
	Process *ProcessNode

	Exec *model.Process

	ImageTags    []string
	MatchedRules []*model.MatchedRule

	Files      map[string]*FileNode
	DNSNames   map[string]*DNSNode
	IMDSEvents map[model.IMDSEvent]*IMDSNode
	Sockets    []*SocketNode
	Syscalls   []int32
}

// NewExecNode returns a new ExecNode instance
func NewExecNode(entry *model.Process) *ExecNode {
	return &ExecNode{
		CurrentExec: entry,
	}
}

// debug prints out recursively content of each node
func (e *ExecNode) Debug(w io.Writer, prefix string) {
	fmt.Fprintf(w, prefix+"- process: %s\n", e.Exec.FileEvent.PathnameStr)
	prefix = prefix + "  "
	fmt.Fprintf(w, prefix+"args: %v\n", e.Exec.Args)
	fmt.Fprintf(w, prefix+"envs: %v\n", e.Exec.Envs)
	fmt.Fprintf(w, prefix+"files: %v\n")
	prefix = prefix + "  . "
	for file := range e.Files {
		file.Debug(w, prefix)
	}
	// TODO: rest of events
}

// scrub args/envs
func (pl *ExecNode) Scrub() error {

}
