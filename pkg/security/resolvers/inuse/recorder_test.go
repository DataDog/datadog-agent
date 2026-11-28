// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package inuse

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
)

const (
	containerID = "c1"
	imageID     = "sha256:abc"
	rootMount   = 42
)

// testRecorder returns a recorder of the host and of the containers, whose
// image image names and whose root mount is rootMount, the reports it sends,
// and the clock it reads.
func testRecorder(t *testing.T, image func(string) (string, bool)) (*Recorder, *[]*usage.Report, *time.Time) {
	t.Helper()
	var sent []*usage.Report
	cfg := DefaultConfig()
	cfg.Host = true
	var root syscall.Stat_t
	require.NoError(t, syscall.Stat(fmt.Sprintf("/proc/%d/root", os.Getpid()), &root))
	r := newRecorder(cfg, image, func(report *usage.Report) { sent = append(sent, report) }, t.TempDir(), root.Dev, root.Ino, false)
	now := time.Unix(1700000000, 0)
	r.now = func() time.Time { return now }
	r.readRoot = func(string, uint32) uint32 { return rootMount }
	return r, &sent, &now
}

func knownImage(string) (string, bool) { return imageID, true }

// containerRun returns a process of the container and a file it runs.
func containerRun(p string) (*model.ProcessContext, *model.FileEvent) {
	pc := &model.ProcessContext{}
	pc.Pid = uint32(os.Getpid())
	pc.ContainerContext.ContainerID = containerutils.ContainerID(containerID)
	pc.Source = model.ProcessCacheEntryFromEvent
	pc.UID, pc.EUID = 1000, 1000
	file := &model.FileEvent{}
	file.SetPathnameStr(p)
	file.Mode = syscall.S_IFREG | 0755
	file.MountID = rootMount
	return pc, file
}

// keysOf returns the keys of the files at paths, sorted.
func keysOf(paths ...string) []uint64 {
	var keys []uint64
	for _, p := range paths {
		keys = append(keys, usage.AppendKeys(nil, p)...)
	}
	slices.Sort(keys)
	return keys
}

func TestRecordContainer(t *testing.T) {
	r, sent, now := testRecorder(t, knownImage)
	pc, file := containerRun("/usr/bin/bash")
	pc.EUID = 0
	r.RecordRun(pc, file)
	r.Forward(*now)

	require.Len(t, *sent, 1)
	report := (*sent)[0]
	require.NoError(t, report.Validate())
	assert.Equal(t, usage.KeyImage, report.KeyKind)
	assert.Equal(t, imageID, report.ImageID)
	assert.Equal(t, []string{containerID}, report.ContainerIDs)
	assert.Equal(t, keysOf("/usr/bin/bash"), report.Keys)
	assert.Equal(t, []int64{now.Unix()}, report.LastSeen)
	assert.Equal(t, []uint8{usage.FlagRoot}, report.Flags)
	assert.Equal(t, now.Unix(), report.ObservedSince)

	// An unchanged run reports nothing more.
	r.RecordRun(pc, file)
	r.Forward(*now)
	assert.Len(t, *sent, 1)
}

func TestRecordOpenGates(t *testing.T) {
	r, sent, now := testRecorder(t, knownImage)

	pc, data := containerRun("/usr/share/zoneinfo/UTC")
	data.Mode = syscall.S_IFREG | 0644
	r.RecordOpen(pc, data)

	_, lib := containerRun("/usr/lib/x86_64-linux-gnu/libc.so.6")
	lib.Mode = syscall.S_IFREG | 0644
	reader, _ := containerRun("")
	reader.Comm = "ldconfig"
	r.RecordOpen(reader, lib)
	r.RecordOpen(pc, lib)

	r.Forward(*now)
	require.Len(t, *sent, 1)
	assert.Equal(t, keysOf("/usr/lib/x86_64-linux-gnu/libc.so.6"), (*sent)[0].Keys)
}

func TestRecordWaitsForWorkloadAndRoot(t *testing.T) {
	image, known := "", false
	r, sent, now := testRecorder(t, func(string) (string, bool) { return image, known })
	pc, file := containerRun("/usr/bin/bash")

	r.RecordRun(pc, file)
	r.Forward(*now)
	require.Len(t, *sent, 0, "a container of unknown image reports nothing")
	assert.Equal(t, 1, r.Stats().PendingRecords)

	image, known = imageID, true
	r.retryPending(*now)
	r.Forward(*now)
	require.Len(t, *sent, 1)
	assert.Equal(t, keysOf("/usr/bin/bash"), (*sent)[0].Keys)
	assert.Equal(t, 0, r.Stats().PendingRecords)
}

func TestRecordContainerOfUnknownImage(t *testing.T) {
	r, sent, now := testRecorder(t, func(string) (string, bool) { return "", true })
	pc, file := containerRun("/usr/bin/bash")
	r.RecordRun(pc, file)
	r.Forward(*now)
	require.Len(t, *sent, 1)
	assert.Equal(t, usage.KeyContainer, (*sent)[0].KeyKind)
	assert.Equal(t, []string{containerID}, (*sent)[0].ContainerIDs)
	require.NoError(t, (*sent)[0].Validate())
}

func TestRecordSkipsOtherMounts(t *testing.T) {
	r, sent, now := testRecorder(t, knownImage)
	pc, file := containerRun("/usr/bin/bash")
	file.MountID = rootMount + 1
	r.RecordRun(pc, file)
	r.Forward(*now)
	require.Len(t, *sent, 1, "a new workload reports its start")
	assert.Empty(t, (*sent)[0].Keys)
}

// TestRecordDrift checks that the files of the upper layer of a container
// count for its image until its package databases change, while the files of
// the image count all along, and bytecode counts for its source.
func TestRecordDrift(t *testing.T) {
	r, sent, now := testRecorder(t, knownImage)
	databases := "before"
	r.fingerprint = func(string) string { return databases }
	run := func(p string, layer int32) {
		pc, file := containerRun(p)
		file.Flags = layer
		r.RecordRun(pc, file)
	}

	run("/usr/bin/copied-up", model.UpperLayer)
	databases = "after"
	*now = now.Add(driftCheckInterval)
	run("/usr/bin/installed", model.UpperLayer)
	run("/usr/bin/bash", model.LowerLayer)
	run("/usr/lib/python3/dist-packages/six/__pycache__/six.cpython-312.pyc", model.UpperLayer)
	r.Forward(*now)

	require.Len(t, *sent, 1)
	assert.Equal(t, keysOf("/usr/bin/copied-up", "/usr/bin/bash", "/usr/lib/python3/dist-packages/six/six.py"), (*sent)[0].Keys)
	assert.Equal(t, 1, r.Stats().Drifted)
}

func TestRecordSkipsPlaceholder(t *testing.T) {
	r, sent, now := testRecorder(t, knownImage)
	pc, file := containerRun("/usr/bin/bash")
	pc.Source = model.ProcessCacheEntryFromPlaceholder
	r.RecordRun(pc, file)
	r.Forward(*now)
	assert.Empty(t, *sent)
}

func TestRecordDeletedFile(t *testing.T) {
	r, sent, now := testRecorder(t, knownImage)
	pc, file := containerRun("/usr/bin/bash" + DeletedSuffix)
	r.RecordRun(pc, file)
	r.Forward(*now)
	require.Len(t, *sent, 1)
	assert.Equal(t, keysOf("/usr/bin/bash"), (*sent)[0].Keys)
}

func TestRecordBounds(t *testing.T) {
	r, sent, now := testRecorder(t, knownImage)
	r.cfg.ContainerKeys = 2
	for _, p := range []string{"/usr/bin/a", "/usr/bin/b", "/usr/bin/c"} {
		pc, file := containerRun(p)
		r.RecordRun(pc, file)
	}
	r.Forward(*now)
	require.Len(t, *sent, 1)
	assert.Len(t, (*sent)[0].Keys, 2)
	assert.True(t, (*sent)[0].Truncated)
	assert.Equal(t, 2, r.Stats().Keys)
}

func TestRecordLatches(t *testing.T) {
	r, sent, now := testRecorder(t, knownImage)
	pc, file := containerRun("/usr/bin/bash")
	r.RecordRun(pc, file)
	r.Forward(*now)
	require.Len(t, *sent, 1)

	*now = now.Add(r.cfg.RunGap / 2)
	r.RecordRun(pc, file)
	r.Forward(*now)
	assert.Len(t, *sent, 1, "a run within the gap waits")

	pc.UID = 0
	r.RecordRun(pc, file)
	r.Forward(*now)
	require.Len(t, *sent, 2, "a new flag goes out")
	assert.Equal(t, []uint8{usage.FlagRoot}, (*sent)[1].Flags)

	*now = now.Add(r.cfg.RunGap * 2)
	r.RecordRun(pc, file)
	r.Forward(*now)
	require.Len(t, *sent, 3, "a run past the gap goes out")
	assert.Equal(t, []int64{now.Unix()}, (*sent)[2].LastSeen)

	r.Resend()
	r.Forward(*now)
	assert.Len(t, *sent, 4, "a new stream gets every workload")
}

func TestRecordHost(t *testing.T) {
	r, sent, now := testRecorder(t, knownImage)

	pc := &model.ProcessContext{}
	pc.Pid = uint32(os.Getpid())
	pc.Source = model.ProcessCacheEntryFromEvent
	run := &model.FileEvent{}
	run.SetPathnameStr("/usr/bin/su")
	run.Mode = syscall.S_IFREG | 04755
	r.RecordRun(pc, run)

	// An open counts once settled, when the file is in place and holds code.
	lib := filepath.Join(r.hostRoot, "usr/lib/libfoo.so.1")
	require.NoError(t, os.MkdirAll(filepath.Dir(lib), 0o755))
	require.NoError(t, os.WriteFile(lib, []byte("\x7fELF"), 0o644))
	var st syscall.Stat_t
	require.NoError(t, syscall.Stat(lib, &st))
	open := &model.FileEvent{}
	open.SetPathnameStr("/usr/lib/libfoo.so.1")
	open.Mode = syscall.S_IFREG | 0644
	open.Inode = st.Ino
	r.RecordOpen(pc, open)
	gone := &model.FileEvent{}
	gone.SetPathnameStr("/usr/lib/libgone.so.1")
	gone.Mode = syscall.S_IFREG | 0644
	gone.Inode = 1
	r.RecordOpen(pc, gone)

	r.recordSettled(*now)
	r.Forward(*now)
	require.Len(t, *sent, 1)
	assert.Equal(t, keysOf("/usr/bin/su"), (*sent)[0].Keys, "the opens wait")

	r.recordSettled(now.Add(settleDelay))
	r.Forward(*now)
	require.Len(t, *sent, 2)
	report := (*sent)[1]
	assert.Equal(t, usage.KeyHost, report.KeyKind)
	assert.Equal(t, keysOf("/usr/bin/su", "/usr/lib/libfoo.so.1"), report.Keys)
	require.NoError(t, report.Validate())
}

func TestSendStats(t *testing.T) {
	r, _, _ := testRecorder(t, knownImage)
	client := statsdclient.NewStatsdClient()
	r.statsdClient = client
	pc, file := containerRun("/usr/bin/bash")
	r.RecordRun(pc, file)
	r.CountForwarded()
	r.CountForwardDropped(2)

	require.NoError(t, r.SendStats())
	assert.EqualValues(t, 1, client.Get(metrics.MetricSBOMUsageWorkloads))
	assert.EqualValues(t, 1, client.Get(metrics.MetricSBOMUsageKeys))
	assert.EqualValues(t, 1, client.Get(metrics.MetricSBOMResolverEnrichedSBOMForwarded))
	assert.EqualValues(t, 2, client.Get(metrics.MetricSBOMResolverEnrichedSBOMForwardDropped))

	// The counts start again from zero.
	require.NoError(t, r.SendStats())
	assert.EqualValues(t, 1, client.Get(metrics.MetricSBOMResolverEnrichedSBOMForwarded))
}

func TestCGroupDeleted(t *testing.T) {
	for _, tc := range []struct {
		name      string
		image     func(string) (string, bool)
		remaining bool
		kept      bool
	}{
		{name: "image workload", image: knownImage, kept: true},
		{name: "container workload", image: func(string) (string, bool) { return "", true }},
		{name: "another cgroup of the container", image: func(string) (string, bool) { return "", true }, remaining: true, kept: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _, _ := testRecorder(t, tc.image)
			pc, file := containerRun("/usr/bin/bash")
			r.RecordRun(pc, file)

			cg := cgroupModel.NewCacheEntry(model.ContainerContext{ContainerID: containerutils.ContainerID(containerID)}, model.CGroupContext{}, 0)
			r.OnCGroupDeleted(cg, func(containerutils.ContainerID) bool { return tc.remaining })
			assert.Equal(t, tc.kept, r.Stats().Workloads == 1, "workloads %d", r.Stats().Workloads)
		})
	}
}

func TestReadsCode(t *testing.T) {
	for _, tc := range []struct {
		comm, parent string
		want         bool
	}{
		{comm: "bash"},
		{comm: "ldconfig", want: true},
		{comm: "pip3.11", want: true},
		{comm: "python3.11", parent: "pip", want: true},
		{comm: "python3", parent: "py3compile", want: true},
		{comm: "ld-linux-x86-64", want: true},
		{comm: "bundle"},
		{comm: "python3", parent: "bash"},
	} {
		pc := &model.ProcessContext{}
		pc.Comm = tc.comm
		if tc.parent != "" {
			pc.Parent = &model.Process{}
			pc.Parent.Comm = tc.parent
		}
		assert.Equal(t, tc.want, ReadsCode(pc), "%s under %s", tc.comm, tc.parent)
	}
}

func TestHoldsCode(t *testing.T) {
	for _, tc := range []struct {
		path string
		perm uint16
		want bool
	}{
		{"/usr/share/zoneinfo/Europe/Paris", 0644, false},
		{"/usr/bin/tcpdump", 0755, true},
		{"/usr/lib/x86_64-linux-gnu/libc.so.6", 0644, true},
		{"/usr/lib/python3/dist-packages/requests/api.py", 0644, true},
		{"/opt/tomcat/webapps/app.war", 0644, true},
		{"/usr/lib/python3/site-packages/foo-1.0-py3.12.egg", 0644, true},
		{"/etc/init.d/ssh", 0755, false},
	} {
		file := &model.FileEvent{}
		file.SetPathnameStr(tc.path)
		file.Mode = syscall.S_IFREG | tc.perm
		assert.Equal(t, tc.want, HoldsCode(file), "%s %o", tc.path, tc.perm)
	}
}
