// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func fileCapabilitiesXattr(words ...uint32) []byte {
	data := make([]byte, 4*len(words))
	for i, word := range words {
		binary.LittleEndian.PutUint32(data[4*i:4*i+4], word)
	}
	return data
}

func TestParseFileCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want model.FileCapabilities
	}{
		{
			name: "revision-1-effective",
			data: fileCapabilitiesXattr(0x01000001, 0x80000400, 1),
			want: model.FileCapabilities{Permitted: 1<<31 | 1<<10, Inheritable: 1, Effective: true, Available: true},
		},
		{
			name: "revision-1-permitted-only",
			data: fileCapabilitiesXattr(0x01000000, 0x400, 0),
			want: model.FileCapabilities{Permitted: 1 << 10, Available: true},
		},
		{
			name: "revision-2-high-bits",
			data: fileCapabilitiesXattr(0x02000001, 0x400, 1, 0x80000100, 0x80),
			want: model.FileCapabilities{Permitted: 1<<63 | 1<<40 | 1<<10, Inheritable: 1<<39 | 1, Effective: true, Available: true},
		},
		{
			name: "revision-3-rootid-is-not-a-capability",
			data: fileCapabilitiesXattr(0x03000001, 0x400, 1, 0x80000100, 0x80, 100000),
			want: model.FileCapabilities{Permitted: 1<<63 | 1<<40 | 1<<10, Inheritable: 1<<39 | 1, Effective: true, Available: true},
		},
		{
			name: "empty-revision-1",
			data: fileCapabilitiesXattr(0x01000000, 0, 0),
			want: model.FileCapabilities{Available: true},
		},
		{
			name: "empty-revision-2",
			data: fileCapabilitiesXattr(0x02000000, 0, 0, 0, 0),
			want: model.FileCapabilities{Available: true},
		},
		{
			name: "empty-revision-3",
			data: fileCapabilitiesXattr(0x03000000, 0, 0, 0, 0, 100000),
			want: model.FileCapabilities{Available: true},
		},
		{name: "absent"},
		{name: "short-magic", data: []byte{1, 2, 3}},
		{name: "unknown-revision", data: fileCapabilitiesXattr(0x04000001, 0x400, 0, 0, 0)},
		{name: "unknown-flags", data: fileCapabilitiesXattr(0x02000002, 0x400, 0, 0, 0)},
		{name: "revision-1-wrong-size", data: fileCapabilitiesXattr(0x01000000, 0x400, 0, 0, 0)},
		{name: "revision-2-wrong-size", data: fileCapabilitiesXattr(0x02000000, 0x400, 0, 0, 0, 0)},
		{name: "revision-3-wrong-size", data: fileCapabilitiesXattr(0x03000000, 0x400, 0, 0, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseFileCapabilities(tc.data))
			if tc.want.Available {
				for size := 0; size < len(tc.data); size++ {
					assert.Equal(t, model.FileCapabilities{}, parseFileCapabilities(tc.data[:size]), "truncated to %d bytes", size)
				}
				assert.Equal(t, model.FileCapabilities{}, parseFileCapabilities(append(tc.data, 0)), "extra byte")
			}
		})
	}
}

func FuzzParseFileCapabilities(f *testing.F) {
	f.Add([]byte{})
	f.Add(fileCapabilitiesXattr(0x01000001, 0x400, 1))
	f.Add(fileCapabilitiesXattr(0x02000000, 0x400, 1, 0x100, 0x80))
	f.Add(fileCapabilitiesXattr(0x03000001, 0x400, 1, 0x100, 0x80, 100000))
	f.Fuzz(func(t *testing.T, data []byte) {
		caps := parseFileCapabilities(data)
		if !caps.Available {
			assert.Equal(t, model.FileCapabilities{}, caps)
		} else {
			assert.Contains(t, []int{12, 20, 24}, len(data))
		}
	})
}

func TestReadFileCapabilitiesFromProcfsUnavailable(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "without-capabilities")
	require.NoError(t, os.WriteFile(filename, nil, 0600))
	stat, err := utils.UnixStat(filename)
	require.NoError(t, err)

	assert.Equal(t, model.FileCapabilities{}, readFileCapabilitiesFromProcfs(filename, stat.Ino))
	assert.Equal(t, model.FileCapabilities{}, readFileCapabilitiesFromProcfs(filename, 0))
	assert.Equal(t, model.FileCapabilities{}, readFileCapabilitiesFromProcfs(filename, stat.Ino+1))
	assert.Equal(t, model.FileCapabilities{}, readFileCapabilitiesFromProcfs(filename+"-missing", stat.Ino))
}

// TestFileCapabilitiesFromProcfs uses real procfs and xattrs, but fake BPF maps:
// neither the snapshot nor the fallback is allowed to depend on the exec hooks.
func TestFileCapabilitiesFromProcfs(t *testing.T) {
	const childEnv = "FILE_CAPABILITIES_PROCFS_TEST_CHILD"
	if os.Getenv(childEnv) == "1" {
		// Reuse this test binary as a long-running executable so the fixture
		// does not depend on a host-installed sleep binary.
		time.Sleep(time.Minute)
		return
	}

	executable, err := os.Executable()
	require.NoError(t, err)
	binaryData, err := os.ReadFile(executable)
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		caps model.FileCapabilities
	}{
		{name: "no-xattr"},
		{name: "empty-xattr", caps: model.FileCapabilities{Available: true}},
		{name: "capabilities", caps: model.FileCapabilities{Permitted: 1 << 10, Inheritable: 1, Effective: true, Available: true}},
		{name: "deleted", caps: model.FileCapabilities{Permitted: 1 << 10, Effective: true, Available: true}},
		{name: "replaced", caps: model.FileCapabilities{Permitted: 1 << 10, Effective: true, Available: true}},
		{name: "unreadable", caps: model.FileCapabilities{Permitted: 1 << 10, Effective: true, Available: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "sleep")
			require.NoError(t, os.WriteFile(filename, binaryData, 0755))
			cmd := exec.Command(filename, "-test.run=^TestFileCapabilitiesFromProcfs$")
			cmd.Env = append(os.Environ(), childEnv+"=1")
			require.NoError(t, cmd.Start())
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			})
			pid := uint32(cmd.Process.Pid)

			if tc.caps.Available {
				// Attach after exec: procfs reports current file metadata, not the
				// capabilities used or acquired by the already-running process.
				magic := uint32(0x02000000)
				if tc.caps.Effective {
					magic |= 1
				}
				data := fileCapabilitiesXattr(magic, uint32(tc.caps.Permitted), uint32(tc.caps.Inheritable), 0, 0)
				if err := unix.Setxattr(filename, "security.capability", data, 0); err != nil {
					if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EOPNOTSUPP) {
						t.Skipf("setting file capabilities requires CAP_SETFCAP and xattr support: %v", err)
					}
					require.NoError(t, err)
				}
			}

			stat, err := utils.UnixStat(utils.ProcExePath(pid))
			require.NoError(t, err)
			wantCaps := tc.caps
			switch tc.name {
			case "deleted":
				require.NoError(t, os.Remove(filename))
			case "replaced":
				replacement := filename + "-new"
				require.NoError(t, os.WriteFile(replacement, binaryData, 0755))
				require.NoError(t, os.Rename(replacement, filename))
			case "unreadable":
				require.NoError(t, os.Chmod(filename, 0111))
				fd, err := unix.Open(filename, unix.O_RDONLY|unix.O_CLOEXEC, 0)
				if err == nil {
					_ = unix.Close(fd)
					t.Skip("cannot simulate an unreadable executable with DAC override privileges")
				}
				require.ErrorIs(t, err, unix.EACCES)
				wantCaps = model.FileCapabilities{}
			}

			resolver, err := newResolver()
			require.NoError(t, err)
			resolver.procCacheMap = newFakeEBPMap()
			resolver.pidCacheMap = newFakeEBPMap()
			resolver.inodeFileMap = newFakeEBPMap()
			resolver.pathIDMap = newFakeEBPMap()
			// Keep the test about this process, rather than resolving the test
			// runner's complete ancestor chain through procfs as well.
			parentPID := uint32(os.Getpid())
			resolver.entryCache[parentPID] = resolver.NewProcessCacheEntry(model.PIDContext{Pid: parentPID, Tid: parentPID})

			proc, err := process.NewProcess(int32(pid))
			require.NoError(t, err)
			resolver.SyncCache(proc)
			entry := resolver.ResolveFromCache(pid, pid, stat.Ino)
			require.NotNil(t, entry)
			assert.Equal(t, uint64(model.ProcessCacheEntryFromSnapshot), entry.Source)
			assert.Equal(t, wantCaps, entry.FileCapabilities)
			assert.Zero(t, entry.CapsUsed)
			snapshotProcess := entry.Process

			// Snapshot metadata must also survive a userspace cache miss and a
			// subsequent lookup from the kernel maps (and be inherited on fork).
			delete(resolver.entryCache, pid)
			entry = resolver.ResolveFromKernelMaps(pid, pid, parentPID, stat.Ino, nil)
			require.NotNil(t, entry)
			assert.Equal(t, wantCaps, entry.FileCapabilities)
			child := model.NewProcessCacheEntry()
			entry.Fork(child)
			assert.Equal(t, wantCaps, child.FileCapabilities)

			// Simulate a cached parent with the same executable, credentials and
			// args/envs, but different file capability availability. This covers
			// both unavailable exec metadata and xattrs removed after exec.
			parent := resolver.entryCache[parentPID]
			parent.Process = snapshotProcess
			parent.PIDContext = model.PIDContext{Pid: parentPID, Tid: parentPID}
			parent.Source = model.ProcessCacheEntryFromEvent
			parent.FileCapabilities = model.FileCapabilities{Available: !wantCaps.Available}

			// Exercise the regular procfs fallback with no child entry in either
			// cache: optional metadata must not turn this fork into an exec.
			delete(resolver.entryCache, pid)
			resolver.procCacheMap = newFakeEBPMap()
			resolver.pidCacheMap = newFakeEBPMap()
			resolver.SetState(Snapshotted)
			entry = resolver.Resolve(pid, pid, parentPID, stat.Ino, true, nil)
			require.NotNil(t, entry)
			assert.Equal(t, uint64(model.ProcessCacheEntryFromProcFS), entry.Source)
			assert.Equal(t, wantCaps, entry.FileCapabilities)
			assert.False(t, parent.Equals(entry), "strict exec equality must still include file capabilities")
			assert.False(t, entry.IsExec, "a procfs fork must keep process.is_thread true")
			assert.Same(t, parent, entry.Ancestor)
			assert.Same(t, parent.ArgsEntry, entry.ArgsEntry)
			assert.Same(t, parent.EnvsEntry, entry.EnvsEntry)
			assert.Equal(t, model.FileCapabilities{Available: !wantCaps.Available}, parent.FileCapabilities, "the parent's captured metadata must remain unchanged")

			// A delayed event can describe a different image for the same PID.
			entry = resolver.ResolveFromProcfs(pid, stat.Ino+1, nil)
			require.NotNil(t, entry)
			assert.True(t, entry.IsParentMissing)
			assert.Equal(t, model.FileCapabilities{}, entry.FileCapabilities)
			assert.Equal(t, model.FileCapabilities{}, readFileCapabilitiesFromProcfs(utils.ProcExePath(pid), stat.Ino+1))
		})
	}
}
