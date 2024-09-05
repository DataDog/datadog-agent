// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package processcontext holds process context
package processcontext

import (
	"errors"
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/exp/slices"
)

// ProcessNodeIface is an interface used to identify the parent of a process context
type ProcessNodeIface interface {
	GetCurrentParent() ProcessNodeIface
	GetPossibleParents() []ProcessNodeIface
	GetChildren() *[]ProcessNodeIface
	GetCurrentSiblings() *[]ProcessNodeIface
	AppendChild(child *ProcessNode, currentParent bool)
}

type ProcessListOwner interface {
	// is valid root node
	IsAValidRootNode(event *model.Process) bool

	// matches
	Matches(p1, p2 *ExecNode) bool

	// send custom stats
	SendStats(client statsd.ClientInterface) error

	GetCacheKeyFromExec(exec *ExecNode) interface{}

	// // tells if we want an aggregated view or a runtime mirror
	// ShouldRemoveProcessesOnExit() bool
}

type ProcessStats struct {
	ProcessNodes int64
	ExecNodes    int64
	FileNodes    int64
	DNSNodes     int64
	SocketNodes  int64
	IMDSNodes    int64
}

type ProcessList struct {
	// selector:
	// for dump:             imageName/imageTag
	// for profile:          imageName/*
	// for process resolver: */*
	selector cgroupModel.WorkloadSelector

	// already present for dump/profiles
	// for process resolvers, today it's only fork/execs/exits
	// for dump/profile: could be anything else EXCEPT EXITS (which will remove nodes)
	// /!\ QUESTION: we could want to save other event types to the process resolver too, WDYT?
	validEventTypes []model.EventType // min: exec, plus dns, files, dns etc

	owner ProcessListOwner

	// internals
	resolvers    *resolvers // eBPF, eBPF-less or windows
	stats        ProcessStats
	statsdClient statsd.ClientInterface
	scrubber     *procutil.DataScrubber

	cache map[interface{}]*ExecNode

	rootNodes []ProcessNode
}

func NewProcessList(selector cgroupModel.WorkloadSelector, validEventTypes []model.EventType, owner ProcessListOwner,
	resolvers *resolvers, statsdClient statsd.ClientInterface, scrubber *procutil.DataScrubber) *ProcessList {
	cache := make(map[interface{}]*ExecNode)
	return &ProcessList{
		selector:        seclector,
		validEventTypes: validEventTypes,
		owner:           owner,
		resolvers:       resolvers,
		statsdClient:    statsdClient,
		scrubber:        scrubber,
		cache:           cache,
	}
}

func NewProcessListFromFile(owner ProcessListOwner, resolvers *resolvers) (*ProcessList, error) {}

// marshall and save processes to the given file
func (pl *ProcessList) SaveToFile(filePath, format string) error {
	// TODO
	return nil
}

func (pl *ProcessList) ToJSON(pl *ProcessList) ([]byte, error) {
	// TODO
	return nil, nil
}

func (pl *ProcessList) ToDOT(pl *ProcessList) ([]byte, error) {
	// TODO
	return nil, nil
}

func (pl *ProcessList) MatchesSelector(event *model.Event) bool {
	// TODO
	return true
}

// debug prints out recursively content of each node
func (pl *ProcessList) Debug(w io.Writer) {
	fmt.Fprintf(w, "== PROCESS LIST\n")
	fmt.Fprintf(w, "selector: %v\n", pl.selector)
	fmt.Fprintf(w, "valid event types: %v\n", pl.validEventTypes)
	fmt.Fprintf(w, "process list:\n")
	for root := range pl.rootNodes {
		roo.Debug(w, prefix+"  ")
	}
}

// at least, look at event.Process and can look at event.ContainerContext depending on the selector
func (pl *ProcessList) Insert(event *model.Event, insertMissingProcesses bool, imageTag string) (newEntryAdded bool, err error) {
	if !slices.Contains(pl.validEventTypes, event.GetEventType()) {
		return false, errors.New("event type unvalid")
	}

	// Process list take only care of execs
	exec, new, error := pl.FindOrInsertExec(event, insertMissingProcesses, imageTag)
	if error != nil {
		return new, error
	}

	if new {
		key := pl.owner.GetCacheKeyFromExec(exec)
		pl.cache[key] = exec
	}

	if event.GetEventType() == model.ExecEventType {
		return new, nil
	}

	// if we want to insert other event types, give them to the process:
	return exec.Insert(event, imageTag)
}

func (pl *ProcessList) GetCacheExec(key interface{}) *ExecNode {
	exec, ok := pl.cache[key]
	if ok {
		return exec
	}
	return nil
}

func (pl *ProcessList) Contains(event *model.Event, insertMissingProcesses bool, imageTag string) (newEntryAdded bool, err error) {
	// ~same as Insert()
}

func (pl *ProcessList) Delete(pid uint32, nsid, imageTag string) (entryDeleted bool, err error) {
}

func (pl *ProcessList) GetCurrentParent() ProcessNodeIface {
	return nil
}

func (pl *ProcessList) GetPossibleParents() []ProcessNodeIface {
	return []ProcessNode{}
}

func (pl *ProcessList) GetChildren() *[]ProcessNodeIface {
	return &pl.rootNodes
}

func (pl *ProcessList) GetCurrentSiblings() *[]ProcessNodeIface {
	return nil
}

// AppendChild appends a new root node in the ProcessList
func (pl *ProcessList) AppendChild(node *ProcessNode, currentParrent bool) {
	pl.rootNodes = append(pl.rootNodes, node)
	node.PossibleParents = append(node.PossibleParents, pl)
	if currentParrent {
		node.CurrentParent = pl
	}
}

// func (pl *ProcessList) InsertProcess(pid, ppid uint32, nsid, ts uint64) (newEntryAdded bool, err error) {
// }
// func (pl *ProcessList) InsertExec(pid, ppid uint32, nsid, ts uint64, file, tty, containerID string, argv []string,
// 	argsTruncated bool, envs []string, envsTruncated bool) (newEntryAdded bool, err error) {
// }
