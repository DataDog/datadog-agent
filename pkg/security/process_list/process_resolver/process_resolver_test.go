// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package processresolver holds processresolver related files
package processresolver

import (
	"testing"
	"time"

	processlist "github.com/DataDog/datadog-agent/pkg/security/process_list"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"
)

func newFakeExecEvent(ppid, pid int, pathname string) *model.Event {
	e := model.NewFakeEvent()
	e.Type = uint32(model.ExecEventType)
	e.ProcessContext = &model.ProcessContext{}
	e.ProcessContext.PPid = uint32(ppid)
	e.ProcessContext.Pid = uint32(pid)
	e.ProcessContext.ForkTime = time.Now()
	e.ProcessContext.FileEvent.PathnameStr = pathname
	return e
}

type testStats struct {
	TotalProcessNodes   int64
	TotalExecNodes      int64
	CurrentProcessNodes int64
	CurrentExecNodes    int64
}

func (ts *testStats) AddProcess(nbTreads int64) {
	ts.TotalProcessNodes++
	ts.TotalExecNodes += nbTreads
	ts.CurrentProcessNodes++
	ts.CurrentExecNodes += nbTreads
}

func (ts *testStats) DeleteProcess(nbThreads int64) {
	ts.CurrentProcessNodes--
	ts.CurrentExecNodes -= nbThreads
}

func (ts *testStats) ValidateCounters(t *testing.T, pl *processlist.ProcessList) {
	assert.Equal(t, ts.TotalProcessNodes, pl.Stats.TotalProcessNodes)
	assert.Equal(t, ts.TotalExecNodes, pl.Stats.TotalExecNodes)
	assert.Equal(t, ts.CurrentProcessNodes, pl.Stats.CurrentProcessNodes)
	assert.Equal(t, ts.CurrentExecNodes, pl.Stats.CurrentExecNodes)
	assert.Equal(t, int(ts.CurrentProcessNodes), pl.GetProcessCacheSize())
	assert.Equal(t, int(ts.CurrentExecNodes), pl.GetExecCacheSize())
}

func checkParentality(pl *processlist.ProcessList, pc *ProcessResolver, parent, child *model.Event) bool {
	// first, get cached processes
	cachedProcessParent := pl.GetCacheProcess(pc.GetProcessCacheKey(&parent.ProcessContext.Process))
	cachedProcessChild := pl.GetCacheProcess(pc.GetProcessCacheKey(&child.ProcessContext.Process))
	if cachedProcessParent == nil || cachedProcessChild == nil {
		return false
	}
	// then, ensure child is part of parent children
	if !slices.ContainsFunc(cachedProcessParent.Children, func(c *processlist.ProcessNode) bool {
		return pc.ProcessMatches(cachedProcessChild, c)
	}) {
		return false
	}

	// validate process / exec links

	// 1/ for parent
	cachedExecParent := pl.GetCacheExec(pc.GetExecCacheKey(&parent.ProcessContext.Process))
	if cachedExecParent == nil {
		return false
	}
	if !slices.ContainsFunc(cachedProcessParent.PossibleExecs, func(e *processlist.ExecNode) bool {
		return pc.ExecMatches(e, cachedExecParent)
	}) {
		return false
	}
	if cachedExecParent.ProcessLink != cachedProcessParent {
		return false
	}

	// 1/ for child
	cachedExecChild := pl.GetCacheExec(pc.GetExecCacheKey(&child.ProcessContext.Process))
	if cachedExecChild == nil {
		return false
	}
	if !slices.ContainsFunc(cachedProcessChild.PossibleExecs, func(e *processlist.ExecNode) bool {
		return pc.ExecMatches(e, cachedExecChild)
	}) {
		return false
	}
	if cachedExecChild.ProcessLink != cachedProcessChild {
		return false
	}
	return true
}

func isProcessAndExecPresent(pl *processlist.ProcessList, pc *ProcessResolver, event *model.Event) bool {
	// first, get cached process
	cachedProcess := pl.GetCacheProcess(pc.GetProcessCacheKey(&event.ProcessContext.Process))
	if cachedProcess == nil {
		return false
	}

	// validate process / exec links
	cachedExec := pl.GetCacheExec(pc.GetExecCacheKey(&event.ProcessContext.Process))
	if cachedExec == nil {
		return false
	}
	if !slices.ContainsFunc(cachedProcess.PossibleExecs, func(e *processlist.ExecNode) bool {
		return pc.ExecMatches(e, cachedExec)
	}) {
		return false
	}
	if cachedExec.ProcessLink != cachedProcess {
		return false
	}
	return true
}

func isProcessOrExecPresent(pl *processlist.ProcessList, pc *ProcessResolver, event *model.Event) bool {
	// first, check process presence
	cachedProcess := pl.GetCacheProcess(pc.GetProcessCacheKey(&event.ProcessContext.Process))
	if cachedProcess != nil {
		return true
	}

	// then exec
	cachedExec := pl.GetCacheExec(pc.GetExecCacheKey(&event.ProcessContext.Process))
	if cachedExec != nil {
		return true
	}
	return false
}

func TestFork1st(t *testing.T) {
	pc := NewProcessResolver()
	processList := processlist.NewProcessList(cgroupModel.WorkloadSelector{Image: "*", Tag: "*"},
		[]model.EventType{model.ExecEventType, model.ForkEventType, model.ExitEventType}, pc /* ,nil  */, nil, nil)
	stats := testStats{}

	// parent
	parent := newFakeExecEvent(0, 1, "/bin/parent")
	new, err := processList.Insert(parent, true, "")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, new)
	stats.AddProcess(1)
	stats.ValidateCounters(t, processList)
	if !isProcessAndExecPresent(processList, pc, parent) {
		t.Fatal("didn't found cached parent")
	}

	// parent
	//     \ child
	child := newFakeExecEvent(1, 2, "/bin/child")
	new, err = processList.Insert(child, true, "")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, new)
	stats.AddProcess(1)
	stats.ValidateCounters(t, processList)
	if checkParentality(processList, pc, parent, child) == false {
		t.Fatal("parent / child paternality not found")
	}

	// parent
	child.Type = uint32(model.ExitEventType)
	deleted, err := processList.Insert(child, true, "")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, deleted)
	stats.DeleteProcess(1)
	stats.ValidateCounters(t, processList)
	if !isProcessAndExecPresent(processList, pc, parent) {
		t.Fatal("didn't found cached parent")
	}
	if isProcessOrExecPresent(processList, pc, child) {
		t.Fatal("child still present")
	}

	// nothing
	deleted, err = processList.DeleteProcess(pc.GetProcessCacheKey(&parent.ProcessContext.Process), "")
	assert.Equal(t, true, deleted)
	stats.DeleteProcess(1)
	stats.ValidateCounters(t, processList)
	if isProcessOrExecPresent(processList, pc, parent) {
		t.Fatal("parent still present")
	}
}

//
// TODO: tests from pkg/security/resolvers/process/resolver_test.go to add:
//

func TestFork2nd(t *testing.T)            {}
func TestForkExec(t *testing.T)           {}
func TestForkExecExec(t *testing.T)       {}
func TestOrphanExec(t *testing.T)         {}
func TestForkReuse(t *testing.T)          {}
func TestForkForkExec(t *testing.T)       {}
func TestExecBomb(t *testing.T)           {}
func TestExecLostFork(t *testing.T)       {}
func TestExecLostExec(t *testing.T)       {}
func TestIsExecExecRuntime(t *testing.T)  {}
func TestIsExecExecSnapshot(t *testing.T) {}
