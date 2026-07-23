// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"strconv"
	"testing"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/assert"
)

func TestReadContainerProcessCommandlines(t *testing.T) {
	wrapperStart := time.Unix(100, 0).UTC()
	redisStart := time.Unix(101, 0).UTC()
	redisProcess := &workloadmeta.Process{
		Pid:          101,
		ContainerID:  "container-id",
		CreationTime: redisStart,
		Cmdline:      []string{"redis-server", "/etc/redis/redis.conf"},
		Cwd:          "/stale/cwd",
	}
	store := newProcessCommandlineTestStore(t,
		redisProcess,
		&workloadmeta.Process{
			Pid:          100,
			ContainerID:  "container-id",
			CreationTime: wrapperStart,
			Cmdline:      []string{"/usr/local/bin/tini", "--", "/etc/scripts/start_redis.sh"},
		},
		&workloadmeta.Process{
			Pid:          102,
			ContainerID:  "other-container",
			CreationTime: time.Unix(102, 0).UTC(),
			Cmdline:      []string{"other-service", "/etc/other/config"},
		},
		nil,
	)
	readProcessWorkingDir := fakeProcessWorkingDirReader(map[int32]string{
		100: "",
		101: "/etc/redis",
	})

	commandlines := readContainerProcessCommandlines(context.Background(), store, "container-id", readProcessWorkingDir)

	assert.ElementsMatch(t, []TargetCommandline{
		{Args: []string{"/usr/local/bin/tini", "--", "/etc/scripts/start_redis.sh"}},
		{Args: []string{"redis-server", "/etc/redis/redis.conf"}, WorkingDir: "/etc/redis"},
	}, commandlines)

	redisProcess.Cmdline[0] = "changed"
	assert.Contains(t, commandlines, TargetCommandline{Args: []string{"redis-server", "/etc/redis/redis.conf"}, WorkingDir: "/etc/redis"})
}

func TestReadContainerProcessCommandlinesRejectsUnavailableProcesses(t *testing.T) {
	creationTime := time.Unix(101, 0).UTC()
	store := newProcessCommandlineTestStore(t, &workloadmeta.Process{
		Pid:          101,
		ContainerID:  "container-id",
		CreationTime: creationTime,
		Cmdline:      []string{"redis-server", "/etc/redis/redis.conf"},
	})

	tests := []struct {
		name                  string
		store                 workloadmeta.Component
		readProcessWorkingDir func(context.Context, *workloadmeta.Process) (string, bool)
	}{
		{
			name:                  "process store unavailable",
			readProcessWorkingDir: fakeProcessWorkingDirReader(nil),
		},
		{
			name:  "live process unavailable",
			store: store,
			readProcessWorkingDir: func(context.Context, *workloadmeta.Process) (string, bool) {
				return "", false
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Empty(t, readContainerProcessCommandlines(context.Background(), tt.store, "container-id", tt.readProcessWorkingDir))
		})
	}
}

func TestMatchesProcessCreationTime(t *testing.T) {
	secondPrecision := time.Unix(101, 0).UTC()
	subsecondPrecision := secondPrecision.Add(250 * time.Millisecond)

	assert.True(t, matchesProcessCreationTime(secondPrecision, secondPrecision.Add(860*time.Millisecond)))
	assert.True(t, matchesProcessCreationTime(subsecondPrecision, subsecondPrecision))
	assert.False(t, matchesProcessCreationTime(subsecondPrecision, secondPrecision.Add(860*time.Millisecond)))
	assert.False(t, matchesProcessCreationTime(secondPrecision, secondPrecision.Add(time.Second)))
}

func TestMatchesLiveProcessIdentity(t *testing.T) {
	process := &workloadmeta.Process{
		Cmdline: []string{"redis-server", "/etc/redis/redis.conf"},
		Exe:     "/usr/bin/redis-server",
	}

	assert.True(t, matchesLiveProcessIdentity(process, []string{"redis-server *:6379"}, "/usr/bin/redis-server"))
	assert.False(t, matchesLiveProcessIdentity(process, []string{"nginx"}, "/usr/sbin/nginx"))
	assert.False(t, matchesLiveProcessIdentity(process, process.Cmdline, ""))

	process.Exe = ""
	assert.True(t, matchesLiveProcessIdentity(process, process.Cmdline, ""))
	assert.False(t, matchesLiveProcessIdentity(process, []string{"nginx"}, ""))
}

func fakeProcessWorkingDirReader(workingDirs map[int32]string) func(context.Context, *workloadmeta.Process) (string, bool) {
	return func(_ context.Context, process *workloadmeta.Process) (string, bool) {
		workingDir, ok := workingDirs[process.Pid]
		if !ok {
			return "", false
		}
		return workingDir, true
	}
}

func newProcessCommandlineTestStore(t *testing.T, processes ...*workloadmeta.Process) workloadmeta.Component {
	t.Helper()

	store := newWorkloadMetaMock(t)
	for _, process := range processes {
		if process == nil {
			continue
		}
		process.EntityID = workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(process.Pid)),
		}
		store.Set(process)
	}
	return store
}
