// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker || (cri && containerd)

package configfilesdiscoveryimpl

import (
	"strconv"
	"testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/assert"
)

func TestReadContainerProcessCommandlines(t *testing.T) {
	redisProcess := &workloadmeta.Process{
		Pid:         101,
		ContainerID: "container-id",
		Cmdline:     []string{"redis-server", "/etc/redis/redis.conf"},
		Cwd:         "/etc/redis",
	}
	store := newProcessCommandlineTestStore(t,
		redisProcess,
		&workloadmeta.Process{
			Pid:         100,
			ContainerID: "container-id",
			Cmdline:     []string{"/usr/local/bin/tini", "--", "/etc/scripts/start_redis.sh"},
		},
		&workloadmeta.Process{
			Pid:         102,
			ContainerID: "other-container",
			Cmdline:     []string{"other-service", "/etc/other/config"},
		},
		nil,
	)
	commandlines := readContainerProcessCommandlines(store, "container-id")

	assert.ElementsMatch(t, []TargetCommandline{
		{Args: []string{"/usr/local/bin/tini", "--", "/etc/scripts/start_redis.sh"}},
		{Args: []string{"redis-server", "/etc/redis/redis.conf"}, WorkingDir: "/etc/redis"},
	}, commandlines)

	redisProcess.Cmdline[0] = "changed"
	assert.Contains(t, commandlines, TargetCommandline{Args: []string{"redis-server", "/etc/redis/redis.conf"}, WorkingDir: "/etc/redis"})
}

func TestReadContainerProcessCommandlinesRejectsUnavailableStore(t *testing.T) {
	assert.Empty(t, readContainerProcessCommandlines(nil, "container-id"))
}

func TestReadContainerProcessCommandlinesRejectsIncompleteProcesses(t *testing.T) {
	store := newProcessCommandlineTestStore(t,
		&workloadmeta.Process{
			Pid:         100,
			ContainerID: "container-id",
		},
		&workloadmeta.Process{
			Pid:         0,
			ContainerID: "container-id",
			Cmdline:     []string{"redis-server", "/etc/redis/redis.conf"},
		},
	)

	assert.Empty(t, readContainerProcessCommandlines(store, "container-id"))
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
