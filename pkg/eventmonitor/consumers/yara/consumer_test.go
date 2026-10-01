// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

var testNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func newTestConsumer(t *testing.T, handler ExecHandler) (*ExecConsumer, *Stats) {
	t.Helper()
	stats := &Stats{}
	if handler == nil {
		handler = func(ExecFile) {}
	}
	c, err := newExecConsumer(&Config{ChanSize: 10}, stats, handler)
	require.NoError(t, err)
	c.now = func() time.Time { return testNow }
	return c, stats
}

func newExecEvent(process *model.Process) *model.Event {
	ev := model.NewFakeEvent()
	ev.Type = uint32(model.ExecEventType)
	ev.Exec.Process = process
	return ev
}

func newTestProcess() *model.Process {
	return &model.Process{
		PIDContext: model.PIDContext{Pid: 4242, Tid: 4242},
		FileEvent: model.FileEvent{
			FileFields: model.FileFields{
				CTime:   1700000000000000000,
				PathKey: model.PathKey{Inode: 123, MountID: 45},
			},
			PathnameStr: "/usr/bin/true",
			Filesystem:  "overlay",
		},
		CGroup:           model.CGroupContext{CGroupID: containerutils.CGroupID("/kubepods/pod1/abc")},
		ContainerContext: model.ContainerContext{ContainerID: containerutils.ContainerID("abc")},
	}
}

func TestCopyMainBinary(t *testing.T) {
	c, _ := newTestConsumer(t, nil)

	copied := c.Copy(newExecEvent(newTestProcess()))
	require.NotNil(t, copied)
	files, ok := copied.(*execFiles)
	require.True(t, ok)

	assert.Equal(t, ExecFile{
		PID:         4242,
		CGroupID:    "/kubepods/pod1/abc",
		ContainerID: "abc",
		Path:        "/usr/bin/true",
		MountID:     45,
		Inode:       123,
		CTime:       1700000000000000000,
		Filesystem:  "overlay",
		IsScript:    false,
		SeenAt:      testNow,
	}, files.main)
	assert.False(t, files.hasInterpreter)
}

// In the CWS model, Process.FileEvent is the executed file (the script, for a #! exec) and
// LinuxBinprm.FileEvent is the interpreter, which /proc/<pid>/exe points at.
func TestCopyInterpreter(t *testing.T) {
	c, _ := newTestConsumer(t, nil)

	p := newTestProcess()
	p.FileEvent.PathnameStr = "/tmp/script.sh"
	p.LinuxBinprm.FileEvent = model.FileEvent{
		FileFields: model.FileFields{
			CTime:   1700000000000000001,
			PathKey: model.PathKey{Inode: 999, MountID: 46},
		},
		PathnameStr: "/usr/bin/bash",
	}
	require.True(t, p.HasInterpreter())

	files, ok := c.Copy(newExecEvent(p)).(*execFiles)
	require.True(t, ok)

	// the script must not be read through /proc/<pid>/exe, which is the interpreter
	assert.Equal(t, ExecFile{
		PID:         4242,
		CGroupID:    "/kubepods/pod1/abc",
		ContainerID: "abc",
		Path:        "/tmp/script.sh",
		MountID:     45,
		Inode:       123,
		CTime:       1700000000000000000,
		Filesystem:  "overlay",
		IsScript:    true,
		SeenAt:      testNow,
	}, files.main)

	require.True(t, files.hasInterpreter)
	assert.Equal(t, ExecFile{
		PID:         4242,
		CGroupID:    "/kubepods/pod1/abc",
		ContainerID: "abc",
		Path:        "/usr/bin/bash",
		MountID:     46,
		Inode:       999,
		CTime:       1700000000000000001,
		// not resolved for the interpreter
		Filesystem: "",
		IsScript:   false,
		SeenAt:     testNow,
	}, files.interpreter)
}

// resolvingFieldHandlers resolves paths lazily, like the eBPF field handlers
type resolvingFieldHandlers struct {
	*model.FakeFieldHandlers
	paths    map[uint64]string
	resolved int
}

func (fh *resolvingFieldHandlers) ResolveFilePath(_ *model.Event, f *model.FileEvent) string {
	fh.resolved++
	f.SetPathnameStr(fh.paths[f.Inode])
	return f.PathnameStr
}

func TestCopyResolvesPaths(t *testing.T) {
	c, _ := newTestConsumer(t, nil)

	p := newTestProcess()
	p.FileEvent.PathnameStr = ""
	p.LinuxBinprm.FileEvent = model.FileEvent{FileFields: model.FileFields{PathKey: model.PathKey{Inode: 999}}}
	ev := newExecEvent(p)
	fh := &resolvingFieldHandlers{
		FakeFieldHandlers: &model.FakeFieldHandlers{},
		paths:             map[uint64]string{123: "/tmp/script.sh", 999: "/usr/bin/bash"},
	}
	ev.FieldHandlers = fh

	files, ok := c.Copy(ev).(*execFiles)
	require.True(t, ok)
	assert.Equal(t, "/tmp/script.sh", files.main.Path)
	assert.Equal(t, "/usr/bin/bash", files.interpreter.Path)
	assert.Equal(t, 2, fh.resolved)

	// already resolved paths are not resolved again
	_, ok = c.Copy(ev).(*execFiles)
	require.True(t, ok)
	assert.Equal(t, 2, fh.resolved)
}

func TestCopySkips(t *testing.T) {
	c, _ := newTestConsumer(t, nil)

	t.Run("kworker", func(t *testing.T) {
		p := newTestProcess()
		p.IsKworker = true
		assert.Nil(t, c.Copy(newExecEvent(p)))
	})

	t.Run("no process", func(t *testing.T) {
		assert.Nil(t, c.Copy(newExecEvent(nil)))
	})

	t.Run("not an exec", func(t *testing.T) {
		ev := newExecEvent(newTestProcess())
		ev.Type = uint32(model.ExitEventType)
		assert.Nil(t, c.Copy(ev))
	})
}

func TestHandleEvent(t *testing.T) {
	var got []ExecFile
	c, stats := newTestConsumer(t, func(f ExecFile) { got = append(got, f) })

	p := newTestProcess()
	c.HandleEvent(c.Copy(newExecEvent(p)))
	require.Len(t, got, 1)
	assert.Equal(t, "/usr/bin/true", got[0].Path)
	assert.EqualValues(t, 1, stats.ExecsReceived.Load())

	p.LinuxBinprm.FileEvent.Inode = 999
	p.LinuxBinprm.FileEvent.PathnameStr = "/usr/bin/bash"
	c.HandleEvent(c.Copy(newExecEvent(p)))
	require.Len(t, got, 3)
	// the executed file (the script) first, then the interpreter
	assert.True(t, got[1].IsScript)
	assert.Equal(t, "/usr/bin/true", got[1].Path)
	assert.False(t, got[2].IsScript)
	assert.Equal(t, "/usr/bin/bash", got[2].Path)
	assert.EqualValues(t, 2, stats.ExecsReceived.Load())

	// unexpected payloads are ignored
	c.HandleEvent("unexpected")
	assert.Len(t, got, 3)
	assert.EqualValues(t, 2, stats.ExecsReceived.Load())
}

func TestConsumerInterface(t *testing.T) {
	c, _ := newTestConsumer(t, nil)
	assert.Equal(t, ConsumerID, c.ID())
	assert.Equal(t, 10, c.ChanSize())
	assert.Equal(t, []model.EventType{model.ExecEventType}, c.EventTypes())
	assert.NoError(t, c.Start())
	c.Stop()
}

func TestNewExecConsumerValidation(t *testing.T) {
	_, err := newExecConsumer(nil, nil, func(ExecFile) {})
	assert.Error(t, err)
	_, err = newExecConsumer(nil, &Stats{}, nil)
	assert.Error(t, err)

	c, err := newExecConsumer(&Config{ChanSize: 0}, &Stats{}, func(ExecFile) {})
	require.NoError(t, err)
	assert.Equal(t, defaultChanSize, c.ChanSize())
}

func TestConfigDefaults(t *testing.T) {
	cfg := newConfigFrom(mock.NewSystemProbe(t))
	assert.Equal(t, &Config{
		Enabled:           false,
		RulesDir:          "",
		ChanSize:          500,
		Workers:           2,
		QueueSize:         64,
		MaxFileSize:       64 * 1024 * 1024,
		ScanTimeout:       10 * time.Second,
		IdentityCacheSize: 20000,
		RecheckTTL:        24 * time.Hour,
	}, cfg)
}

func TestConfigOverrides(t *testing.T) {
	t.Setenv("DD_EVENT_MONITORING_CONFIG_YARA_ENABLED", "true")
	t.Setenv("DD_EVENT_MONITORING_CONFIG_YARA_RULES_DIR", "/etc/yara")
	t.Setenv("DD_EVENT_MONITORING_CONFIG_YARA_SCAN_TIMEOUT", "3s")
	cfg := newConfigFrom(mock.NewSystemProbe(t))
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "/etc/yara", cfg.RulesDir)
	assert.Equal(t, 3*time.Second, cfg.ScanTimeout)
}
