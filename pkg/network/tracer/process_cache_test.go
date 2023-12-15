// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/network/events"
)

func TestProcessCacheProcessEvent(t *testing.T) {

	const ddService = "DD_SERVICE"
	const ddVersion = "DD_VERSION"
	const ddEnv = "DD_ENV"

	envs := map[string]string{
		ddService: "service",
		ddVersion: "version",
		ddEnv:     "env",
	}

	tests := []struct {
		envs        []string
		filter      []string
		filtered    []string
		containerID string
	}{
		{},
		{envs: nil, filter: defaultFilteredEnvs, filtered: nil},
		{envs: []string{ddEnv}, filter: defaultFilteredEnvs, filtered: []string{ddEnv}},
		{envs: []string{ddVersion}, filter: defaultFilteredEnvs, filtered: []string{ddVersion}},
		{envs: []string{ddService}, filter: defaultFilteredEnvs, filtered: []string{ddService}},
		{envs: []string{ddEnv, ddVersion}, filter: defaultFilteredEnvs, filtered: []string{ddEnv, ddVersion}},
		{envs: []string{ddEnv, ddService}, filter: defaultFilteredEnvs, filtered: []string{ddEnv, ddService}},
		{envs: []string{ddVersion, ddService}, filter: defaultFilteredEnvs, filtered: []string{ddVersion, ddService}},
		{envs: []string{ddService, ddVersion, ddEnv}, filter: defaultFilteredEnvs, filtered: defaultFilteredEnvs},
		{envs: []string{ddService, ddVersion, ddEnv, "foo=bar"}, filter: defaultFilteredEnvs, filtered: defaultFilteredEnvs},
		{envs: []string{"foo"}, filter: defaultFilteredEnvs, filtered: []string{}},
		{envs: []string{ddEnv}},
		{envs: []string{ddVersion}},
		{envs: []string{ddService}},
		{envs: []string{ddEnv, ddVersion}},
		{envs: []string{ddEnv, ddService}},
		{envs: []string{ddVersion, ddService}},
		{envs: []string{ddService, ddVersion, ddEnv}},
	}

	testFunc := func(t *testing.T, entry *events.Process) {
		for i, te := range tests {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				pc, err := newProcessCache(10, te.filter)
				require.NoError(t, err)
				t.Cleanup(pc.Stop)

				var values []string
				for _, e := range te.envs {
					values = append(values, e+"="+envs[e])
				}

				entry.Envs = values

				p := pc.processEvent(entry)
				if entry.ContainerID == nil && len(te.filter) > 0 && len(te.filtered) == 0 {
					assert.Nil(t, p)
				} else {
					assert.NotNil(t, p)
					assert.Equal(t, entry.Pid, p.Pid)
					if entry.ContainerID != nil {
						containerID, ok := p.ContainerID.Get().(string)
						assert.True(t, ok)
						assert.Equal(t, entry.ContainerID.Get(), containerID)
					}
					l := te.envs
					if len(te.filter) > 0 {
						l = te.filtered
					}
					assert.Len(t, p.Envs, len(l))
					for _, e := range l {
						assert.Equal(t, envs[e], p.Env(e))
					}
				}
			})
		}
	}

	t.Run("without container id", func(t *testing.T) {
		entry := events.Process{Pid: 1234}

		testFunc(t, &entry)
	})

	t.Run("with container id", func(t *testing.T) {
		entry := events.Process{
			Pid:         1234,
			ContainerID: intern.GetByString("container"),
		}

		testFunc(t, &entry)
	})
}

func TestProcessCacheAdd(t *testing.T) {
	t.Run("fewer than maxProcessListSize", func(t *testing.T) {
		pc, err := newProcessCache(5, nil)
		require.NoError(t, err)
		require.NotNil(t, pc)
		t.Cleanup(pc.Stop)

		pc.add(&events.Process{
			Pid:       1234,
			StartTime: 1,
		})

		p, ok := pc.Get(1234, 1)
		require.True(t, ok)
		require.NotNil(t, p)
		assert.Equal(t, uint32(1234), p.Pid)
		assert.Equal(t, int64(1), p.StartTime)
	})

	t.Run("greater than maxProcessListSize", func(t *testing.T) {
		pc, err := newProcessCache(10, nil)
		require.NoError(t, err)
		require.NotNil(t, pc)
		t.Cleanup(pc.Stop)

		for i := 0; i < maxProcessListSize+1; i++ {
			pc.add(&events.Process{
				Pid:       1234,
				StartTime: int64(i),
			})
		}

		p, ok := pc.Get(1234, 0)
		require.False(t, ok)
		require.Nil(t, p)

		// verify all other processes are correct
		for i := 1; i < maxProcessListSize+1; i++ {
			p, ok = pc.Get(1234, int64(i))
			require.True(t, ok)
			assert.Equal(t, uint32(1234), p.Pid)
			assert.Equal(t, int64(i), p.StartTime)
		}
	})

	t.Run("process evicted, same pid", func(t *testing.T) {
		pc, err := newProcessCache(2, nil)
		require.NoError(t, err)
		require.NotNil(t, pc)
		t.Cleanup(pc.Stop)

		pc.add(&events.Process{
			Pid:       1234,
			StartTime: 1,
		})

		pc.add(&events.Process{
			Pid:       1234,
			StartTime: 2,
		})

		pc.add(&events.Process{
			Pid:       1234,
			StartTime: 3,
		})

		p, ok := pc.Get(1234, 1)
		assert.False(t, ok)
		require.Nil(t, p)

		for _, startTime := range []int64{2, 3} {
			p, ok = pc.Get(1234, startTime)
			assert.True(t, ok)
			require.NotNil(t, p)
			assert.Equal(t, uint32(1234), p.Pid)
			assert.Equal(t, startTime, p.StartTime)
		}

		// replace pid 1234 with 4567
		pc.add(&events.Process{
			Pid:       4567,
			StartTime: 2,
		})

		pc.add(&events.Process{
			Pid:       4567,
			StartTime: 3,
		})

		for _, startTime := range []int64{1, 2, 3} {
			p, ok = pc.Get(1234, startTime)
			assert.False(t, ok)
			assert.Nil(t, p)
		}
	})

	t.Run("process evicted, different pid", func(t *testing.T) {
		pc, err := newProcessCache(1, nil)
		require.NoError(t, err)
		require.NotNil(t, pc)
		t.Cleanup(pc.Stop)

		pc.add(&events.Process{
			Pid:       1234,
			StartTime: 1,
		})

		pc.add(&events.Process{
			Pid:       1235,
			StartTime: 2,
		})

		p, ok := pc.Get(1234, 1)
		assert.False(t, ok)
		assert.Nil(t, p)

		p, ok = pc.Get(1235, 2)
		assert.True(t, ok)
		require.NotNil(t, p)
		assert.Equal(t, uint32(1235), p.Pid)
		assert.Equal(t, int64(2), p.StartTime)
	})

	t.Run("process updated", func(t *testing.T) {
		pc, err := newProcessCache(1, nil)
		require.NoError(t, err)
		require.NotNil(t, pc)
		t.Cleanup(pc.Stop)

		pc.add(&events.Process{
			Pid:       1234,
			StartTime: 1,
			Envs:      []string{"foo=bar"},
		})

		p, ok := pc.Get(1234, 1)
		assert.True(t, ok)
		require.NotNil(t, p)
		assert.Equal(t, uint32(1234), p.Pid)
		assert.Equal(t, int64(1), p.StartTime)
		assert.Equal(t, p.Env("foo"), "bar")

		pc.add(&events.Process{
			Pid:       1234,
			StartTime: 1,
			Envs:      []string{"bar=foo"},
		})

		p, ok = pc.Get(1234, 1)
		assert.True(t, ok)
		require.NotNil(t, p)
		assert.Equal(t, uint32(1234), p.Pid)
		assert.Equal(t, int64(1), p.StartTime)
		assert.Equal(t, p.Env("bar"), "foo")
		assert.NotContains(t, p.Envs, "foo")
	})
}

func TestProcessCacheGet(t *testing.T) {
	pc, err := newProcessCache(10, nil)
	require.NoError(t, err)
	require.NotNil(t, pc)
	t.Cleanup(pc.Stop)

	pc.add(&events.Process{
		Pid:       1234,
		StartTime: 5,
	})

	pc.add(&events.Process{
		Pid:       1234,
		StartTime: 14,
	})

	pc.add(&events.Process{
		Pid:       1234,
		StartTime: 10,
	})

	t.Run("pid not found", func(t *testing.T) {
		p, ok := pc.Get(1235, 0)
		assert.False(t, ok)
		assert.Nil(t, p)
	})

	tests := []struct {
		ts int64

		ok        bool
		startTime int64
	}{
		{ts: 1, ok: false, startTime: 5},
		{ts: 5, ok: true, startTime: 5},
		{ts: 6, ok: true, startTime: 5},
		{ts: 9, ok: true, startTime: 5},
		{ts: 10, ok: true, startTime: 10},
		{ts: 11, ok: true, startTime: 10},
		{ts: 12, ok: true, startTime: 10},
		{ts: 14, ok: true, startTime: 14},
		{ts: 15, ok: true, startTime: 14},
		{ts: 16, ok: true, startTime: 14},
	}

	for i, te := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			p, ok := pc.Get(1234, te.ts)
			assert.Equal(t, te.ok, ok)
			if !te.ok {
				assert.Nil(t, p)
				return
			}
			require.NotNil(t, p)
			assert.Equal(t, te.startTime, p.StartTime)
		})
	}

}
