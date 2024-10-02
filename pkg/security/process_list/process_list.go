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
	UnlinkChild(owner Owner, child *ProcessNode) bool
}

// Owner defines the interface to implement prior to use ProcessList
type Owner interface {
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

// ProcessStats stores stats
type ProcessStats struct {
	// Total metric since startup
	TotalProcessNodes int64
	TotalExecNodes    int64
	// TotalFileNodes    int64
	// TotalDNSNodes     int64
	// TotalSocketNodes  int64
	// TotalIMDSNodes    int64

	// Current number of nodes per type
	CurrentProcessNodes int64
	CurrentExecNodes    int64
	// CurrentFileNodes    int64
	// CurrentDNSNodes     int64
	// CurrentSocketNodes  int64
	// CurrentIMDSNodes    int64
}

func (ps *ProcessStats) incExec() {
	ps.TotalExecNodes++
	ps.CurrentExecNodes++
}

func (ps *ProcessStats) incProcess() {
	ps.TotalProcessNodes++
	ps.CurrentProcessNodes++
}

func (ps *ProcessStats) decExec() {
	ps.CurrentExecNodes--
}

func (ps *ProcessStats) decProcess() {
	ps.CurrentProcessNodes--
}

// ProcessList defines a process graph/cache of processes and their related execs
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

	owner Owner

	// internals
	Stats        ProcessStats
	statsdClient statsd.ClientInterface
	scrubber     *procutil.DataScrubber
	// TODO: redo once we have a generic resolvers interface:
	// resolvers    *resolvers // eBPF, eBPF-less or windows

	execCache    map[interface{}]*ExecNode
	processCache map[interface{}]*ProcessNode // not sure it's useful

	rootNodes []*ProcessNode
}

// NewProcessList returns a new process list
func NewProcessList(selector cgroupModel.WorkloadSelector, validEventTypes []model.EventType, owner Owner,
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

// NewProcessListFromFile returns a new process list from a file
// nolint: all
func NewProcessListFromFile(owner Owner /* , resolvers *resolvers */) (*ProcessList, error) {
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

// Insert tries to insert (or delete) the given event ot the process list graph, using cache if possible
func (pl *ProcessList) Insert(event *model.Event, insertMissingProcesses bool, imageTag string) (newEntryAdded bool, err error) {
	pl.Lock()
	defer pl.Unlock()

	valid, err := pl.isEventValid(event)
	if !valid || err != nil {
		return false, err
	}

	// special case, on exit we remove the associated process and all its childs
	if event.GetEventType() == model.ExitEventType {
		// if we can get a key from a process we should be able to retrieve it
		key := pl.owner.GetProcessCacheKey(&event.ProcessContext.Process)
		if key != nil {
			return pl.deleteCachedProcess(key, imageTag)
		}
		return false, errors.New("process not found in cache")
	}

	// Process list take only care of execs
	exec, newNode, err := pl.findOrInsertExec(event, insertMissingProcesses, imageTag)
	if err != nil {
		return newNode, err
	}

	if event.GetEventType() == model.ExecEventType || event.GetEventType() == model.ForkEventType {
		return newNode, nil
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
// nolint: all
func (pl *ProcessList) findOrInsertExec(event *model.Event, insertMissingProcesses bool, imageTag string) (exec *ExecNode, newNode bool, err error) {
	// check if we already have the exec cached
	execKey := pl.owner.GetExecCacheKey(&event.ProcessContext.Process)
	if execKey != nil {
		exec, ok := pl.execCache[execKey]
		if ok {
			return exec, false, nil
		}
	}

	// check if we already have its related process
	processKey := pl.owner.GetProcessCacheKey(&event.ProcessContext.Process)
	if processKey != nil {
		process, ok := pl.processCache[processKey]
		if ok {
			exec := NewExecNodeFromEvent(event, processKey)
			process.AppendExec(exec, true)
			pl.addExecToCache(exec)
			return exec, true, nil
		}
	}

	// then, check if can be added as root node
	if pl.owner.IsAValidRootNode(&event.ProcessContext.Process) {
		process := NewProcessExecNodeFromEvent(event, processKey, execKey)
		pl.appendChild(process, true)
		pl.addProcessToCache(process)
		return process.CurrentExec, true, nil
	}

	// check if we already have its parent
	parentKey := pl.owner.GetParentProcessCacheKey(event)
	if parentKey != nil {
		parent, ok := pl.processCache[parentKey]
		if ok {
			process := NewProcessExecNodeFromEvent(event, processKey, execKey)
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

// GetCacheExec retrieve the cached exec matching the given key
func (pl *ProcessList) GetCacheExec(key interface{}) *ExecNode {
	pl.Lock()
	defer pl.Unlock()

	if exec, ok := pl.execCache[key]; ok {
		return exec
	}
	return nil
}

// GetCacheProcess retrieve the cached process matching the given key
func (pl *ProcessList) GetCacheProcess(key interface{}) *ProcessNode {
	pl.Lock()
	defer pl.Unlock()

	if process, ok := pl.processCache[key]; ok {
		return process
	}
	return nil
}

// GetExecCacheSize returns the exec cache size
func (pl *ProcessList) GetExecCacheSize() int {
	return len(pl.execCache)
}

// GetProcessCacheSize returns the process cache size
func (pl *ProcessList) GetProcessCacheSize() int {
	return len(pl.processCache)
}

// nolint: all
func (pl *ProcessList) Contains(event *model.Event, insertMissingProcesses bool, imageTag string) (newEntryAdded bool, err error) {
	pl.Lock()
	defer pl.Unlock()

	// ~same as Insert()
	// TODO
	return false, nil
}

func (pl *ProcessList) unlinkIfNoMoreImageTags(process *ProcessNode) bool {
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
		pl.removeProcessFromCache(process)
		return true
	}
	return false
}

// TODO: delete if not useful
// nolint: unused
func (pl *ProcessList) deleteProcess(process *ProcessNode, imageTag string) (entryDeleted bool) {
	// remove imageTag from the list
	process.ImageTags = slices.DeleteFunc(process.ImageTags, func(tag string) bool {
		return tag == imageTag
	})

	// recursively remove childs:
	children := process.GetChildren()
	if children != nil {
		for _, child := range *children {
			_ = pl.deleteProcess(child, imageTag)
		}
	}

	// if there is no more versions for this node, unlink it from its parent(s)
	return pl.unlinkIfNoMoreImageTags(process)
}

func (pl *ProcessList) deleteCachedProcess(key interface{}, imageTag string) (entryDeleted bool, err error) {
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
			if _, err := pl.deleteCachedProcess(childkey, imageTag); err != nil {
				return false, err
			}
		}
	}

	// if there is no more versions for this node, unlink it from its parent(s)
	return pl.unlinkIfNoMoreImageTags(process), nil
}

// DeleteCachedProcess deletes the process matching the provided key, and all its children
func (pl *ProcessList) DeleteCachedProcess(key interface{}, imageTag string) (entryDeleted bool, err error) {
	pl.Lock()
	defer pl.Unlock()

	return pl.deleteCachedProcess(key, imageTag)
}

// GetCurrentParent returns nil (process list don't have parent)
func (pl *ProcessList) GetCurrentParent() ProcessNodeIface {
	return nil
}

// GetPossibleParents returns nil (process list don't have parent)
func (pl *ProcessList) GetPossibleParents() []ProcessNodeIface {
	return nil
}

// GetChildren returns the root nodes
func (pl *ProcessList) GetChildren() *[]*ProcessNode {
	pl.Lock()
	defer pl.Unlock()

	if len(pl.rootNodes) == 0 {
		return nil
	}
	return &pl.rootNodes
}

// GetCurrentSiblings returns nil (process list don't have siblings)
func (pl *ProcessList) GetCurrentSiblings() *[]*ProcessNode {
	return nil
}

func (pl *ProcessList) addExecToCache(exec *ExecNode) {
	pl.execCache[exec.Key] = exec

	// inc stat
	pl.Stats.incExec()
}

func (pl *ProcessList) removeExecFromCache(exec *ExecNode) {
	key := pl.owner.GetExecCacheKey(&exec.Process)
	if key != nil {
		delete(pl.execCache, key)
	}

	// dec stat
	pl.Stats.decExec()
}

func (pl *ProcessList) addProcessToCache(node *ProcessNode) {
	// puts execs in cache
	for _, exec := range node.PossibleExecs {
		pl.addExecToCache(exec)
	}

	// puts process in cache
	pl.processCache[node.Key] = node

	// inc stat
	pl.Stats.incProcess()
}

func (pl *ProcessList) removeProcessFromCache(node *ProcessNode) {
	// puts execs in cache
	for _, exec := range node.PossibleExecs {
		pl.removeExecFromCache(exec)
	}

	// puts process in cache
	key := pl.owner.GetProcessCacheKey(&node.CurrentExec.Process)
	if key != nil {
		delete(pl.processCache, key)
	}

	// dec stat
	pl.Stats.decProcess()
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
func (pl *ProcessList) UnlinkChild(_ Owner, child *ProcessNode) bool {
	pl.Lock()
	defer pl.Unlock()

	return pl.unlinkChild(child)
}

// marshall and save processes to the given file
// nolint: all
func (pl *ProcessList) SaveToFile(filePath, format string) error {
	// TODO
	return nil
}

// nolint: all
func (pl *ProcessList) ToJSON() ([]byte, error) {
	// TODO
	return nil, nil
}

// nolint: all
func (pl *ProcessList) ToDOT() ([]byte, error) {
	// TODO
	return nil, nil
}

// nolint: all
func (pl *ProcessList) MatchesSelector(event *model.Event) bool {
	// TODO
	return true
}

// Walk walks recursively the process nodes
func (pl *ProcessList) Walk(f func(node *ProcessNode) (stop bool)) (stop bool) {
	pl.Lock()
	defer pl.Unlock()

	for _, root := range pl.rootNodes {
		stop = f(root)
		if stop {
			return stop
		}
		stop = root.Walk(f)
		if stop {
			return stop
		}
	}
	return stop
}

// Debug prints out recursively content of each node
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
