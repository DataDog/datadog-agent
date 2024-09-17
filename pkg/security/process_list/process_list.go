// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package processlist holds process context
package processlist

import (
	"errors"
	"fmt"
	"io"
	"sync"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"
)

// ProcessNodeIface is an interface used to identify the parent of a process context
type ProcessNodeIface interface {
	GetCurrentParent() ProcessNodeIface
	GetPossibleParents() []ProcessNodeIface
	GetChildren() *[]*ProcessNode
	GetCurrentSiblings() *[]*ProcessNode
	AppendChild(child *ProcessNode, currentParent bool)
	UnlinkChild(owner ProcessListOwner, child *ProcessNode) bool
}

type ProcessListOwner interface {
	// is valid root node
	IsAValidRootNode(event *model.Process) bool

	// matches
	ExecMatches(e1, e2 *ExecNode) bool
	ProcessMatches(p1, p2 *ProcessNode) bool

	// send custom stats
	SendStats(client statsd.ClientInterface) error

	// returns the key related to an exec
	GetExecCacheKey(process *model.Process) interface{}

	// returns the key related to a process
	GetProcessCacheKey(process *model.Process) interface{}

	// returns the keys related to a process parent, given an event
	GetParentProcessCacheKey(event *model.Event) interface{}
}

type ProcessStats struct {
	// Total metric since startup
	TotalProcessNodes int64
	TotalExecNodes    int64
	// TotalFileNodes    int64
	// TotalDNSNodes     int64
	// TotalSocketNodes  int64
	// TotalIMDSNodes    int64

	// Curent number of nodes per type
	CurrentProcessNodes int64
	CurrentExecNodes    int64
	// CurrentFileNodes    int64
	// CurrentDNSNodes     int64
	// CurrentSocketNodes  int64
	// CurrentIMDSNodes    int64
}

func (ps *ProcessStats) IncExec() {
	ps.TotalExecNodes++
	ps.CurrentExecNodes++
}

func (ps *ProcessStats) IncProcess() {
	ps.TotalProcessNodes++
	ps.CurrentProcessNodes++
}

func (ps *ProcessStats) DecExec() {
	ps.CurrentExecNodes--
}

func (ps *ProcessStats) DecProcess() {
	ps.CurrentProcessNodes--
}

type ProcessList struct {
	sync.Mutex

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
	Stats        ProcessStats
	statsdClient statsd.ClientInterface
	scrubber     *procutil.DataScrubber
	// TODO: redo once we have a generic resolvers interface:
	// resolvers    *resolvers // eBPF, eBPF-less or windows

	execCache    map[interface{}]*ExecNode
	processCache map[interface{}]*ProcessNode

	rootNodes []*ProcessNode
}

func NewProcessList(selector cgroupModel.WorkloadSelector, validEventTypes []model.EventType, owner ProcessListOwner,
	/* resolvers *resolvers,  */ statsdClient statsd.ClientInterface, scrubber *procutil.DataScrubber) *ProcessList {
	execCache := make(map[interface{}]*ExecNode)
	processCache := make(map[interface{}]*ProcessNode)
	return &ProcessList{
		selector:        selector,
		validEventTypes: validEventTypes,
		owner:           owner,
		// resolvers:       resolvers,
		statsdClient: statsdClient,
		scrubber:     scrubber,
		execCache:    execCache,
		processCache: processCache,
	}
}

func NewProcessListFromFile(owner ProcessListOwner /* , resolvers *resolvers */) (*ProcessList, error) {
	// TODO
	return nil, nil
}

// isEventValid evaluates if the provided event is valid
func (pl *ProcessList) isEventValid(event *model.Event) (bool, error) {
	if event.ProcessContext == nil {
		return false, errors.New("event without process context")
	}

	// check event type
	if !slices.Contains(pl.validEventTypes, event.GetEventType()) {
		return false, errors.New("event type unvalid")
	}

	// event specific filtering
	switch event.GetEventType() {
	case model.BindEventType:
		// ignore non IPv4 / IPv6 bind events for now
		if event.Bind.AddrFamily != unix.AF_INET && event.Bind.AddrFamily != unix.AF_INET6 {
			return false, errors.New("invalid event: invalid bind family")
		}
	case model.IMDSEventType:
		// ignore IMDS answers without AccessKeyIDS
		if event.IMDS.Type == model.IMDSResponseType && len(event.IMDS.AWS.SecurityCredentials.AccessKeyID) == 0 {
			return false, fmt.Errorf("untraced event: IMDS response without credentials")
		}
		// ignore IMDS requests without URLs
		if event.IMDS.Type == model.IMDSRequestType && len(event.IMDS.URL) == 0 {
			return false, fmt.Errorf("invalid event: IMDS request without any URL")
		}
	}
	return true, nil
}

// at least, look at event.Process and can look at event.ContainerContext depending on the selector
func (pl *ProcessList) Insert(event *model.Event, insertMissingProcesses bool, imageTag string) (newEntryAdded bool, err error) {
	pl.Lock()
	defer pl.Unlock()

	valid, err := pl.isEventValid(event)
	if !valid || err != nil {
		return false, err
	}

	// special case, on exit we remove the associated process and all its childs
	if event.GetEventType() == model.ExitEventType {
		key := pl.owner.GetProcessCacheKey(&event.ProcessContext.Process)
		return pl.deleteProcess(key, imageTag)
	}

	// Process list take only care of execs
	exec, new, error := pl.findOrInsertExec(event, insertMissingProcesses, imageTag)
	if error != nil {
		return new, error
	}

	if event.GetEventType() == model.ExecEventType {
		return new, nil
	}

	// if we want to insert other event types, give them to the exec:
	return exec.Insert(event, imageTag)
}

// func (pl *ProcessList) hasValidLineage(event *model.Event) (bool, error) {
// // TODO
// 	/*
// 		   EITHER:
// 		      1. process with a valid chain of parents until isvalidrootnode
// 			  2. no parent, but a pid with hierarchy up to isvalidrootnode?
// */
// 	return true, nil
// }

// TODO
func (pl *ProcessList) findOrInsertExec(event *model.Event, insertMissingProcesses bool, imageTag string) (exec *ExecNode, newNode bool, err error) {
	// check if we already have the exec cached
	key := pl.owner.GetExecCacheKey(&event.ProcessContext.Process)
	exec, ok := pl.execCache[key]
	if ok {
		return exec, false, nil
	}

	// check if we already have its related process
	key = pl.owner.GetProcessCacheKey(&event.ProcessContext.Process)
	process, ok := pl.processCache[key]
	if ok {
		exec := NewExecNodeFromEvent(event)
		process.AppendExec(exec, true)
		pl.addExecToCache(exec)
		return exec, true, nil
	}

	// then, check if can be added as root node
	if pl.owner.IsAValidRootNode(&event.ProcessContext.Process) {
		process := NewProcessExecNodeFromEvent(event)
		pl.appendChild(process, true)
		pl.addProcessToCache(process)
		return process.CurrentExec, true, nil
	}

	// check if we already have its parent
	parentKey := pl.owner.GetParentProcessCacheKey(event)
	if parentKey != nil {
		parent, ok := pl.processCache[parentKey]
		if ok {
			process := NewProcessExecNodeFromEvent(event)
			parent.AppendChild(process, true)
			pl.addProcessToCache(process)
			return process.CurrentExec, true, nil
		}
	}

	// err, valid := pl.hasValidLineage(event)
	// if !valid || err != nil {
	// 	return nil, false, err
	// }

	return nil, false, nil
}

func (pl *ProcessList) GetCacheExec(key interface{}) *ExecNode {
	pl.Lock()
	defer pl.Unlock()

	if exec, ok := pl.execCache[key]; ok {
		return exec
	}
	return nil
}

func (pl *ProcessList) GetCacheProcess(key interface{}) *ProcessNode {
	pl.Lock()
	defer pl.Unlock()

	if process, ok := pl.processCache[key]; ok {
		return process
	}
	return nil
}

func (pl *ProcessList) GetExecCacheSize() int {
	return len(pl.execCache)
}

func (pl *ProcessList) GetProcessCacheSize() int {
	return len(pl.processCache)
}

func (pl *ProcessList) Contains(event *model.Event, insertMissingProcesses bool, imageTag string) (newEntryAdded bool, err error) {
	pl.Lock()
	defer pl.Unlock()

	// ~same as Insert()
	// TODO
	return false, nil
}

func (pl *ProcessList) deleteProcess(key interface{}, imageTag string) (entryDeleted bool, err error) {
	if key == nil {
		return false, errors.New("no valid key provided")
	}

	// search for process
	process, ok := pl.processCache[key]
	if !ok {
		return false, errors.New("no process found with provided key")
	}

	// remove imageTag from the list
	process.ImageTags = slices.DeleteFunc(process.ImageTags, func(tag string) bool {
		return tag == imageTag
	})

	// recursively remove childs:
	children := process.GetChildren()
	if children != nil {
		for _, child := range *children {
			childkey := pl.owner.GetProcessCacheKey(&child.CurrentExec.Process)
			pl.deleteProcess(childkey, imageTag)
		}
	}

	// if there is no more versions for this node, unlink it from its parent(s)
	entryDeleted = false
	if len(process.ImageTags) == 0 {
		parents := process.GetPossibleParents()
		for _, parent := range parents {
			switch parent.(type) {
			case *ProcessList:
				// ProcessList is already lock, call directly the lock-free func
				pl.unlinkChild(process)
			default:
				parent.UnlinkChild(pl.owner, process)
			}
		}
		entryDeleted = true
		pl.removeProcessFromCache(process)
	}
	return entryDeleted, nil
}

func (pl *ProcessList) DeleteProcess(key interface{}, imageTag string) (entryDeleted bool, err error) {
	pl.Lock()
	defer pl.Unlock()

	return pl.deleteProcess(key, imageTag)
}

func (pl *ProcessList) GetCurrentParent() ProcessNodeIface {
	return nil
}

func (pl *ProcessList) GetPossibleParents() []ProcessNodeIface {
	return nil
}

func (pl *ProcessList) GetChildren() *[]*ProcessNode {
	pl.Lock()
	defer pl.Unlock()

	if len(pl.rootNodes) == 0 {
		return nil
	}
	return &pl.rootNodes
}

func (pl *ProcessList) GetCurrentSiblings() *[]*ProcessNode {
	return nil
}

func (pl *ProcessList) addExecToCache(exec *ExecNode) {
	key := pl.owner.GetExecCacheKey(&exec.Process)
	pl.execCache[key] = exec

	// inc stat
	pl.Stats.IncExec()
}

func (pl *ProcessList) removeExecFromCache(exec *ExecNode) {
	key := pl.owner.GetExecCacheKey(&exec.Process)
	delete(pl.execCache, key)

	// dec stat
	pl.Stats.DecExec()
}

func (pl *ProcessList) addProcessToCache(node *ProcessNode) {
	// puts execs in cache
	for _, exec := range node.PossibleExecs {
		pl.addExecToCache(exec)
	}

	// puts process in cache
	key := pl.owner.GetProcessCacheKey(&node.CurrentExec.Process)
	pl.processCache[key] = node

	// inc stat
	pl.Stats.IncProcess()
}

func (pl *ProcessList) removeProcessFromCache(node *ProcessNode) {
	// puts execs in cache
	for _, exec := range node.PossibleExecs {
		pl.removeExecFromCache(exec)
	}

	// puts process in cache
	key := pl.owner.GetProcessCacheKey(&node.CurrentExec.Process)
	delete(pl.processCache, key)

	// dec stat
	pl.Stats.DecProcess()
}

func (pl *ProcessList) appendChild(node *ProcessNode, currentParrent bool) {
	// append child
	pl.rootNodes = append(pl.rootNodes, node)
	node.PossibleParents = append(node.PossibleParents, pl)
	if currentParrent || node.CurrentParent == nil {
		node.CurrentParent = pl
	}
}

// AppendChild appends a new root node in the ProcessList
func (pl *ProcessList) AppendChild(node *ProcessNode, currentParrent bool) {
	pl.Lock()
	defer pl.Unlock()

	pl.appendChild(node, currentParrent)
}

// UnlinkChild unlinks a root node
func (pl *ProcessList) unlinkChild(child *ProcessNode) bool {
	removed := false
	pl.rootNodes = slices.DeleteFunc(pl.rootNodes, func(node *ProcessNode) bool {
		if pl.owner.ProcessMatches(child, node) {
			removed = true
			return true
		}
		return false
	})
	return removed
}

// UnlinkChild unlinks a root node
func (pl *ProcessList) UnlinkChild(owner ProcessListOwner, child *ProcessNode) bool {
	pl.Lock()
	defer pl.Unlock()

	return pl.unlinkChild(child)
}

// marshall and save processes to the given file
func (pl *ProcessList) SaveToFile(filePath, format string) error {
	// TODO
	return nil
}

func (pl *ProcessList) ToJSON() ([]byte, error) {
	// TODO
	return nil, nil
}

func (pl *ProcessList) ToDOT() ([]byte, error) {
	// TODO
	return nil, nil
}

func (pl *ProcessList) MatchesSelector(event *model.Event) bool {
	// TODO
	return true
}

// debug prints out recursively content of each node
func (pl *ProcessList) Debug(w io.Writer) {
	pl.Lock()
	defer pl.Unlock()

	fmt.Fprintf(w, "== PROCESS LIST ==\n")
	fmt.Fprintf(w, "selector: %v\n", pl.selector)
	fmt.Fprintf(w, "valid event types: %v\n", pl.validEventTypes)
	fmt.Fprintf(w, "process list:\n")
	for _, root := range pl.rootNodes {
		root.Debug(w, "")
	}
	fmt.Fprintf(w, "== /PROCESS LIST ==\n")
}
