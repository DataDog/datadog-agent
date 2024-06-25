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
	testFunc := func(t *testing.T, name string, entry *events.Process) {
		pc, err := newProcessCache(10)
		require.NoError(t, err)
		t.Cleanup(pc.Stop)

		p := pc.processEvent(entry)
		if entry.ContainerID == nil && len(entry.Tags) == 0 {
			assert.Nil(t, p)
		} else {
			assert.Equal(t, entry, p)
		}
	}

	t.Run("without container id", func(t *testing.T) {
		entry := events.Process{Pid: 1234}

		testFunc(t, t.Name(), &entry)
	})

	t.Run("with container id", func(t *testing.T) {
		entry := events.Process{
			Pid:         1234,
			ContainerID: intern.GetByString("container"),
		}

		testFunc(t, t.Name(), &entry)
	})

	t.Run("without container id, with tags", func(t *testing.T) {
		entry := events.Process{Pid: 1234, Tags: []*intern.Value{intern.GetByString("foo"), intern.GetByString("bar")}}

		testFunc(t, t.Name(), &entry)
	})

	t.Run("with container id, with tags", func(t *testing.T) {
		entry := events.Process{
			Pid:         1234,
			ContainerID: intern.GetByString("container"),
			Tags:        []*intern.Value{intern.GetByString("foo"), intern.GetByString("bar")},
		}

		testFunc(t, t.Name(), &entry)
	})

	t.Run("empty container id", func(t *testing.T) {
		entry := events.Process{
			Pid:         1234,
			ContainerID: intern.GetByString(""),
		}

		testFunc(t, t.Name(), &entry)
	})
}

func TestProcessCacheAdd(t *testing.T) {
	t.Run("fewer than maxProcessListSize", func(t *testing.T) {
		pc, err := newProcessCache(5)
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
		pc, err := newProcessCache(10)
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
		pc, err := newProcessCache(2)
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
		pc, err := newProcessCache(1)
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
		pc, err := newProcessCache(1)
		require.NoError(t, err)
		require.NotNil(t, pc)
		t.Cleanup(pc.Stop)

		pc.add(&events.Process{
			Pid:       1234,
			StartTime: 1,
			Tags:      []*intern.Value{intern.GetByString("foo:bar")},
		})

		p, ok := pc.Get(1234, 1)
		assert.True(t, ok)
		require.NotNil(t, p)
		assert.Equal(t, uint32(1234), p.Pid)
		assert.Equal(t, int64(1), p.StartTime)
		assert.Contains(t, p.Tags, intern.GetByString("foo:bar"))

		pc.add(&events.Process{
			Pid:       1234,
			StartTime: 1,
			Tags:      []*intern.Value{intern.GetByString("bar:foo")},
		})

		p, ok = pc.Get(1234, 1)
		assert.True(t, ok)
		require.NotNil(t, p)
		assert.Equal(t, uint32(1234), p.Pid)
		assert.Equal(t, int64(1), p.StartTime)
		assert.Contains(t, p.Tags, intern.GetByString("bar:foo"))
		assert.NotContains(t, p.Tags, intern.GetByString("foo:bar"))
	})
}

func TestProcessCacheGet(t *testing.T) {
	pc, err := newProcessCache(10)
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
