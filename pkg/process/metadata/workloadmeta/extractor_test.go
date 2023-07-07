// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
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
	extractor := NewWorkloadMetaExtractor(config.Mock(t))

	var (
		proc1 = testProc(Pid1, []string{"java", "mydatabase.jar"})
		proc2 = testProc(Pid2, []string{"python", "myprogram.py"})
		proc3 = testProc(Pid3, []string{"corrina", "--at-her-best"})
		proc4 = testProc(Pid4, []string{"python", "test.py"})
	)

	// Assert that first run generates creation events for all processes
	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
		Pid2: proc2,
	})

	// Extractor cache should have all processes
	procs, cacheVersion := extractor.GetAllProcessEntities()
	assert.Equal(t, int32(1), cacheVersion)
	assert.Equal(t, map[string]*ProcessEntity{
		hashProcess(Pid1, proc1.Stats.CreateTime): {
			Pid:          proc1.Pid,
			NsPid:        proc1.NsPid,
			CreationTime: proc1.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Java},
		},
		hashProcess(Pid2, proc2.Stats.CreateTime): {
			Pid:          proc2.Pid,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
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
		},
		{
			Pid:          proc2.Pid,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
		},
	}, diff.creation)
	assert.ElementsMatch(t, []*ProcessEntity{}, diff.deletion)

	// Assert that if no process is created or terminated, the cache is not updated nor a diff generated
	extractor.Extract(map[int32]*procutil.Process{
		Pid1: proc1,
		Pid2: proc2,
	})

	procs, cacheVersion = extractor.GetAllProcessEntities()
	assert.Equal(t, int32(1), cacheVersion) // cache version doesn't change
	assert.Equal(t, map[string]*ProcessEntity{
		hashProcess(Pid1, proc1.Stats.CreateTime): {
			Pid:          proc1.Pid,
			NsPid:        proc1.NsPid,
			CreationTime: proc1.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Java},
		},
		hashProcess(Pid2, proc2.Stats.CreateTime): {
			Pid:          proc2.Pid,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
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
		hashProcess(Pid2, proc2.Stats.CreateTime): {
			Pid:          proc2.Pid,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
		},
	}, procs)

	diff = <-extractor.ProcessCacheDiff()
	assert.Equal(t, int32(2), diff.cacheVersion)
	assert.ElementsMatch(t, []*ProcessEntity{}, diff.creation)
	assert.ElementsMatch(t, []*ProcessEntity{
		{
			Pid:          Pid1,
			NsPid:        proc1.NsPid,
			CreationTime: proc1.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Java},
		},
	}, diff.deletion)

	// Process creation generates a cache update and diff event
	extractor.Extract(map[int32]*procutil.Process{
		Pid2: proc2,
		Pid3: proc3,
	})

	procs, cacheVersion = extractor.GetAllProcessEntities()
	assert.Equal(t, int32(3), cacheVersion)
	assert.Equal(t, map[string]*ProcessEntity{
		hashProcess(Pid2, proc2.Stats.CreateTime): {
			Pid:          proc2.Pid,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
		},
		hashProcess(Pid3, proc3.Stats.CreateTime): {
			Pid:          proc3.Pid,
			NsPid:        proc3.NsPid,
			CreationTime: proc3.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Unknown},
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
		},
	}, diff.creation)
	assert.ElementsMatch(t, []*ProcessEntity{}, diff.deletion)

	// Process creation and deletion generate a cache update and diff event
	extractor.Extract(map[int32]*procutil.Process{
		Pid3: proc3,
		Pid4: proc4,
	})

	procs, cacheVersion = extractor.GetAllProcessEntities()
	assert.Equal(t, int32(4), cacheVersion)
	assert.Equal(t, map[string]*ProcessEntity{
		hashProcess(Pid3, proc3.Stats.CreateTime): {
			Pid:          proc3.Pid,
			NsPid:        proc3.NsPid,
			CreationTime: proc3.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Unknown},
		},
		hashProcess(Pid4, proc4.Stats.CreateTime): {
			Pid:          proc4.Pid,
			NsPid:        proc4.NsPid,
			CreationTime: proc4.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
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
		},
	}, diff.creation)
	assert.ElementsMatch(t, []*ProcessEntity{
		{
			Pid:          Pid2,
			NsPid:        proc2.NsPid,
			CreationTime: proc2.Stats.CreateTime,
			Language:     &languagemodels.Language{Name: languagemodels.Python},
		},
	}, diff.deletion)
}

func BenchmarkHashProcess(b *testing.B) {
	b.Run("itoa", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hashProcess(0, 0)
		}
	})
	b.Run("sprintf", func(b *testing.B) {
		hashProcess := func(pid int32, createTime int64) string {
			return fmt.Sprintf("pid:%v|createTime:%v", pid, createTime)
		}

		for i := 0; i < b.N; i++ {
			hashProcess(0, 0)
		}
	})
}
