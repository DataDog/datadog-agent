// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	Pid1 = 1000
	Pid2 = 1001
	Pid3 = 1002
	Pid4 = 1003
)

func testProc(pid int32, cmdline []string) *procutil.Process {
	return &procutil.Process{
		Pid:     pid,
		NsPid:   1,
		Cmdline: cmdline,
		Stats:   &procutil.Stats{CreateTime: time.Now().Unix()},
	}
}

func TestExtractor(t *testing.T) {
	fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule()).Reset()

	extractor := NewWorkloadMetaExtractor(configmock.New(t))

	var (
		proc1 = testProc(Pid1, []string{"java", "mydatabase.jar"})
		proc2 = testProc(Pid2, []string{"python", "myprogram.py"})
		proc3 = testProc(Pid3, []string{"corrina", "--at-her-best"})
		proc4 = testProc(Pid4, []string{"python", "test.py"})
	)

	// Silly test container id's for fun, doesn't matter what they are they just have to be unique.
	var (
		//nolint:revive // TODO(PROC) Fix revive linter
		ctrId1 = "containers-are-awesome"
		//nolint:revive // TODO(PROC) Fix revive linter
		ctrId2 = "we-all-live-in-a-yellow-container"
	)
	extractor.SetLastPidToCid(map[int]string{
		Pid1: ctrId1,
		Pid2: ctrId1,
		Pid3: ctrId1,
		Pid4: ctrId2,
	})

	// Assert that first run generates creation events for all processes
	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
		Pid2: proc2,
	})

	// Extractor cache should have all processes
	procs, cacheVersion := extractor.GetAllProcessEntities()
	assert.Equal(t, int32(1), cacheVersion)
	assert.Equal(t, map[string]*ProcessEntity{
		procutil.ProcessIdentity(Pid1, proc1.Stats.CreateTime, proc1.Cmdline): {
			Pid:          proc1.Pid,
			NsPid:        proc1.NsPid,
			CreationTime: proc1.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Java},
			ContainerId:  ctrId1,
		},
		procutil.ProcessIdentity(Pid2, proc2.Stats.CreateTime, proc2.Cmdline): {
			Pid:          proc2.Pid,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
			ContainerId:  ctrId1,
		},
	}, procs)

	// Diff should have creation events for all processes and 0 deletion event
	diff := <-extractor.ProcessCacheDiff()
	assert.Equal(t, int32(1), diff.cacheVersion)
	// Events are generated through map range which doesn't have a deterministic order
	assert.ElementsMatch(t, []*ProcessEntity{
		{
			Pid:          proc1.Pid,
			NsPid:        proc1.NsPid,
			CreationTime: proc1.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Java},
			ContainerId:  ctrId1,
		},
		{
			Pid:          proc2.Pid,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
			ContainerId:  ctrId1,
		},
	}, diff.Creation)
	assert.ElementsMatch(t, []*ProcessEntity{}, diff.Deletion)

	// Assert that if no process is created or terminated, the cache is not updated nor a diff generated
	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
		Pid2: proc2,
	})

	procs, cacheVersion = extractor.GetAllProcessEntities()
	assert.Equal(t, int32(1), cacheVersion) // cache version doesn't change
	assert.Equal(t, map[string]*ProcessEntity{
		procutil.ProcessIdentity(Pid1, proc1.Stats.CreateTime, proc1.Cmdline): {
			Pid:          proc1.Pid,
			NsPid:        proc1.NsPid,
			CreationTime: proc1.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Java},
			ContainerId:  ctrId1,
		},
		procutil.ProcessIdentity(Pid2, proc2.Stats.CreateTime, proc2.Cmdline): {
			Pid:          proc2.Pid,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
			ContainerId:  ctrId1,
		},
	}, procs)

	assert.Len(t, extractor.ProcessCacheDiff(), 0)

	// Process deletion generates a cache update and diff event
	extractor.Extract(map[int32]*procutil.Process{
		Pid2: proc2,
	})

	procs, cacheVersion = extractor.GetAllProcessEntities()
	assert.Equal(t, int32(2), cacheVersion)
	assert.Equal(t, map[string]*ProcessEntity{
		procutil.ProcessIdentity(Pid2, proc2.Stats.CreateTime, proc2.Cmdline): {
			Pid:          proc2.Pid,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
			ContainerId:  ctrId1,
		},
	}, procs)

	diff = <-extractor.ProcessCacheDiff()
	assert.Equal(t, int32(2), diff.cacheVersion)
	assert.ElementsMatch(t, []*ProcessEntity{}, diff.Creation)
	assert.ElementsMatch(t, []*ProcessEntity{
		{
			Pid:          Pid1,
			NsPid:        proc1.NsPid,
			CreationTime: proc1.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Java},
			ContainerId:  ctrId1,
		},
	}, diff.Deletion)

	// Process creation generates a cache update and diff event
	extractor.Extract(map[int32]*procutil.Process{
		Pid2: proc2,
		Pid3: proc3,
	})

	procs, cacheVersion = extractor.GetAllProcessEntities()
	assert.Equal(t, int32(3), cacheVersion)
	assert.Equal(t, map[string]*ProcessEntity{
		procutil.ProcessIdentity(Pid2, proc2.Stats.CreateTime, proc2.Cmdline): {
			Pid:          proc2.Pid,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
			ContainerId:  ctrId1,
		},
		procutil.ProcessIdentity(Pid3, proc3.Stats.CreateTime, proc3.Cmdline): {
			Pid:          proc3.Pid,
			NsPid:        proc3.NsPid,
			CreationTime: proc3.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Unknown},
			ContainerId:  ctrId1,
		},
	}, procs)

	diff = <-extractor.ProcessCacheDiff()
	assert.Equal(t, int32(3), diff.cacheVersion)
	assert.ElementsMatch(t, []*ProcessEntity{
		{
			Pid:          Pid3,
			NsPid:        proc3.NsPid,
			CreationTime: proc3.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Unknown},
			ContainerId:  ctrId1,
		},
	}, diff.Creation)
	assert.ElementsMatch(t, []*ProcessEntity{}, diff.Deletion)

	// Process creation and deletion generate a cache update and diff event
	extractor.Extract(map[int32]*procutil.Process{
		Pid3: proc3,
		Pid4: proc4,
	})

	procs, cacheVersion = extractor.GetAllProcessEntities()
	assert.Equal(t, int32(4), cacheVersion)
	assert.Equal(t, map[string]*ProcessEntity{
		procutil.ProcessIdentity(Pid3, proc3.Stats.CreateTime, proc3.Cmdline): {
			Pid:          proc3.Pid,
			NsPid:        proc3.NsPid,
			CreationTime: proc3.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Unknown},
			ContainerId:  ctrId1,
		},
		procutil.ProcessIdentity(Pid4, proc4.Stats.CreateTime, proc4.Cmdline): {
			Pid:          proc4.Pid,
			NsPid:        proc4.NsPid,
			CreationTime: proc4.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
			ContainerId:  ctrId2,
		},
	}, procs)

	diff = <-extractor.ProcessCacheDiff()
	assert.Equal(t, int32(4), diff.cacheVersion)
	assert.ElementsMatch(t, []*ProcessEntity{
		{
			Pid:          Pid4,
			NsPid:        proc4.NsPid,
			CreationTime: proc4.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
			ContainerId:  ctrId2,
		},
	}, diff.Creation)
	assert.ElementsMatch(t, []*ProcessEntity{
		{
			Pid:          Pid2,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
			ContainerId:  ctrId1,
		},
	}, diff.Deletion)
}

// Occasionally, WorkloadMeta will not have the ContainerID by the first time a process collection is executed. This test
// asserts that the extractor is able to properly handle updating a ContainerID from "" to a valid cid, and
// will re-generate the EventSet for that process once the pidToCid mapping is up-to-date.
func TestLateContainerId(t *testing.T) {
	fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule()).Reset()

	extractor := NewWorkloadMetaExtractor(configmock.New(t))

	var (
		proc1 = testProc(Pid1, []string{"java", "mydatabase.jar"})
	)

	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
	})
	assert.EqualValues(t, &ProcessCacheDiff{
		cacheVersion: 1,
		Creation: []*ProcessEntity{
			{
				Pid:          proc1.Pid,
				ContainerId:  "",
				NsPid:        proc1.NsPid,
				CreationTime: proc1.Stats.CreateTime,
				Language:     &languagemodels.Language{Name: languagemodels.Java},
			},
		},
		Deletion: []*ProcessEntity{},
	}, <-extractor.ProcessCacheDiff())

	var (
		//nolint:revive // TODO(PROC) Fix revive linter
		ctrId1 = "containers-are-awesome"
	)
	extractor.SetLastPidToCid(map[int]string{
		Pid1: ctrId1,
	})

	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
	})
	assert.EqualValues(t, &ProcessCacheDiff{
		cacheVersion: 2,
		Creation: []*ProcessEntity{
			{
				Pid:          proc1.Pid,
				ContainerId:  ctrId1,
				NsPid:        proc1.NsPid,
				CreationTime: proc1.Stats.CreateTime,
				Language:     &languagemodels.Language{Name: languagemodels.Java},
			},
		},
		Deletion: []*ProcessEntity{},
	}, <-extractor.ProcessCacheDiff())
}

// TestExecCmdlineChange tests that when a process has a different cmdline but same PID and same createTime
// the extractor treats it as a new process and generates creation/deletion events accordingly.
func TestExecCmdlineChange(t *testing.T) {
	fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule()).Reset()

	extractor := NewWorkloadMetaExtractor(configmock.New(t))

	// Create a process with a specific createTime
	createTime := time.Now().Unix()
	// Use different NsPid values to differentiate bash vs htop in assertions
	// (In reality exec() preserves NsPid, but this helps verify correct events)
	bashNsPid := int32(100)
	htopNsPid := int32(200)

	proc1Bash := &procutil.Process{
		Pid:     Pid1,
		NsPid:   bashNsPid,
		Cmdline: []string{"bash"},
		Stats:   &procutil.Stats{CreateTime: createTime},
	}

	// First extraction: bash process
	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1Bash,
	})

	procs, cacheVersion := extractor.GetAllProcessEntities()
	assert.Equal(t, int32(1), cacheVersion)
	assert.Len(t, procs, 1)

	diff := <-extractor.ProcessCacheDiff()
	assert.Equal(t, int32(1), diff.cacheVersion)
	assert.Len(t, diff.Creation, 1)
	assert.Equal(t, proc1Bash.Pid, diff.Creation[0].Pid)
	assert.Equal(t, bashNsPid, diff.Creation[0].NsPid)
	assert.Len(t, diff.Deletion, 0)

	proc1htop := &procutil.Process{
		Pid:     Pid1,
		NsPid:   htopNsPid,
		Cmdline: []string{"htop"},
		Stats:   &procutil.Stats{CreateTime: createTime}, // Same createTime as before!
	}

	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1htop,
	})

	procs, cacheVersion = extractor.GetAllProcessEntities()
	assert.Equal(t, int32(2), cacheVersion)
	assert.Len(t, procs, 1) // Still 1 process, but it should be the new one

	diff = <-extractor.ProcessCacheDiff()
	assert.Equal(t, int32(2), diff.cacheVersion)

	// We should generate a creation event for the new process (htop)
	assert.Len(t, diff.Creation, 1, "Expected creation event for exec'd process")
	assert.Equal(t, int32(Pid1), diff.Creation[0].Pid)
	assert.Equal(t, htopNsPid, diff.Creation[0].NsPid, "Creation event should be for htop process")

	// We should generate a deletion event for the old process (bash)
	assert.Len(t, diff.Deletion, 1, "Expected deletion event for original process")
	assert.Equal(t, int32(Pid1), diff.Deletion[0].Pid)
	assert.Equal(t, bashNsPid, diff.Deletion[0].NsPid, "Deletion event should be for bash process")
}
