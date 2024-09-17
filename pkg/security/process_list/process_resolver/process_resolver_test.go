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
)

func newFakeExecEvent() *model.Event {
	e := model.NewFakeEvent()
	e.Type = uint32(model.ExecEventType)
	e.ProcessContext = &model.ProcessContext{}
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

func TestFork1st(t *testing.T) {
	pc := NewProcessResolver()
	processList := processlist.NewProcessList(cgroupModel.WorkloadSelector{Image: "*", Tag: "*"},
		[]model.EventType{model.ExecEventType, model.ForkEventType, model.ExitEventType}, pc /* ,nil  */, nil, nil)
	stats := testStats{}

	// parent
	parent := newFakeExecEvent()
	parent.ProcessContext.Pid = 1
	parent.ProcessContext.Tid = 1
	parent.ProcessContext.ForkTime = time.Now()
	new, err := processList.Insert(parent, true, "")
	if err != nil {
		t.Fatal(err)
	}
	stats.AddProcess(1)
	assert.Equal(t, true, new)
	assert.Equal(t, nil, err)
	cachedParent := processList.GetCacheExec(pc.GetExecCacheKey(&parent.ProcessContext.Process))
	if cachedParent == nil {
		t.Fatal("didn't found cached parent")
	}
	stats.ValidateCounters(t, processList)

	// parent
	//     \ child
	child := newFakeExecEvent()
	child.ProcessContext.PPid = 1
	child.ProcessContext.Pid = 2
	child.ProcessContext.Tid = 2
	child.ProcessContext.ForkTime = time.Now()
	new, err = processList.Insert(child, true, "")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, new)
	assert.Equal(t, nil, err)
	stats.AddProcess(1)
	cachedChild := processList.GetCacheExec(pc.GetExecCacheKey(&child.ProcessContext.Process))
	if cachedChild == nil {
		t.Fatal("didn't found cached child")
	}
	assert.Equal(t, child.ProcessContext.Process, cachedChild.Process)
	stats.ValidateCounters(t, processList)
	assert.Equal(t, cachedParent.ProcessLink, cachedChild.ProcessLink.GetCurrentParent())
	childs := cachedParent.ProcessLink.GetChildren()
	if childs == nil {
		t.Fatal("no childs returned")
	} else if len(*childs) != 1 {
		t.Error("not only 1 child")
	}
	found := false
	for _, child := range *childs {
		if child.CurrentExec == cachedChild {
			found = true
			break
		}
	}
	if !found {
		t.Error("child not found in cached parent")
	}

	// parent
	child.Type = uint32(model.ExitEventType)
	deleted, err := processList.Insert(child, true, "")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, deleted)
	assert.Equal(t, nil, err)
	stats.DeleteProcess(1)
	cachedParent = processList.GetCacheExec(pc.GetExecCacheKey(&parent.ProcessContext.Process))
	if cachedParent == nil {
		t.Fatal("didn't found cached parent")
	}
	stats.ValidateCounters(t, processList)

	// nothing
	deleted, err = processList.DeleteProcess(pc.GetProcessCacheKey(&parent.ProcessContext.Process), "")
	assert.Equal(t, true, deleted)
	assert.Equal(t, nil, err)
	stats.DeleteProcess(1)
	cachedParent = processList.GetCacheExec(pc.GetExecCacheKey(&parent.ProcessContext.Process))
	if cachedParent != nil {
		t.Fatal("parent still present")
	}
	stats.ValidateCounters(t, processList)
}
