// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package darwin

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usergroup"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// TestParseProcArgs covers the fiddliest part of the snapshot: KERN_PROCARGS2 is
// a packed blob and getting the padding wrong silently yields either no argv or
// the environment.
func TestParseProcArgs(t *testing.T) {
	var blob []byte
	blob = binary.LittleEndian.AppendUint32(blob, 2) // argc
	blob = append(blob, []byte("/usr/local/bin/npm\x00\x00\x00")...)
	blob = append(blob, []byte("npm\x00install\x00")...)
	blob = append(blob, []byte("SECRET_TOKEN=hunter2\x00")...) // environment

	execPath, argv, err := parseProcArgs(blob)
	require.NoError(t, err)

	assert.Equal(t, "/usr/local/bin/npm", execPath)
	assert.Equal(t, []string{"npm", "install"}, argv)
	assert.NotContains(t, argv, "SECRET_TOKEN=hunter2",
		"the environment must never be read out of procargs2")
}

// TestParseProcArgsNeverReadsEnvironment is the privacy guard stated as its own
// property: whatever argc says, nothing past the arguments may be returned.
func TestParseProcArgsNeverReadsEnvironment(t *testing.T) {
	var blob []byte
	blob = binary.LittleEndian.AppendUint32(blob, 1) // argc: one argument only
	blob = append(blob, []byte("/bin/sh\x00")...)
	blob = append(blob, []byte("sh\x00")...)
	blob = append(blob, []byte("AWS_SECRET_ACCESS_KEY=abc123\x00")...)
	blob = append(blob, []byte("GITHUB_TOKEN=ghp_xyz\x00")...)

	_, argv, err := parseProcArgs(blob)
	require.NoError(t, err)

	require.Len(t, argv, 1, "exactly argc arguments must be returned")
	for _, a := range argv {
		assert.NotContains(t, a, "SECRET")
		assert.NotContains(t, a, "TOKEN")
	}
}

func TestParseProcArgsRejectsTruncatedBlob(t *testing.T) {
	_, _, err := parseProcArgs([]byte{1, 2})
	assert.Error(t, err, "a blob too short to hold argc must error")

	// argc present but no NUL-terminated exec path.
	short := binary.LittleEndian.AppendUint32(nil, 1)
	_, _, err = parseProcArgs(append(short, []byte("nonulhere")...))
	assert.Error(t, err)
}

// TestSnapshotFindsCurrentProcess needs no fixture: the test binary itself is a
// running process with a known pid, parent and argv.
func TestSnapshotFindsCurrentProcess(t *testing.T) {
	pr, err := process.NewEBPFLessResolver(nil, nil, testScrubber(t), process.NewResolverOpts())
	require.NoError(t, err)

	ug, err := usergroup.NewResolver()
	require.NoError(t, err)

	n, err := Snapshot(pr, ug)
	require.NoError(t, err)
	assert.Positive(t, n, "snapshot must find processes")

	self := pr.Resolve(process.CacheResolverKey{Pid: uint32(os.Getpid())})
	require.NotNil(t, self, "the test process itself must be in the snapshot")

	assert.NotEmpty(t, self.FileEvent.PathnameStr, "own executable path")
	assert.NotZero(t, self.PPid, "own ppid")

	// argv comes from KERN_PROCARGS2, which is the part most likely to be wrong.
	argv, _ := pr.GetProcessArgvScrubbed(&self.Process)
	assert.NotEmpty(t, argv, "argv must be recovered from KERN_PROCARGS2")

	parent := pr.Resolve(process.CacheResolverKey{Pid: uint32(os.Getppid())})
	assert.NotNil(t, parent, "the parent process must also be in the snapshot")
}

// BenchmarkSnapshot measures collector startup cost: the snapshot runs once before
// subscribing, and it walks every process on the machine (roughly 870 on this
// laptop), reading KERN_PROCARGS2 for each. If it were slow it would widen the
// window in which events are missed between snapshot and subscription.
func BenchmarkSnapshot(b *testing.B) {
	scrubber, err := utils.NewScrubber(nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	ug, err := usergroup.NewResolver()
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	var inserted int
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		pr, err := process.NewEBPFLessResolver(nil, nil, scrubber, process.NewResolverOpts())
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		inserted, err = Snapshot(pr, ug)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	b.ReportMetric(float64(inserted), "processes")
}

// TestSnapshotLinksParents is what the snapshot is for: without it every process
// tree truncates at collector startup, which showed up in the staging UI as a
// blank ancestor row.
func TestSnapshotLinksParents(t *testing.T) {
	pr, err := process.NewEBPFLessResolver(nil, nil, testScrubber(t), process.NewResolverOpts())
	require.NoError(t, err)

	ug, err := usergroup.NewResolver()
	require.NoError(t, err)

	_, err = Snapshot(pr, ug)
	require.NoError(t, err)

	self := pr.Resolve(process.CacheResolverKey{Pid: uint32(os.Getpid())})
	require.NotNil(t, self)

	require.NotNil(t, self.Ancestor,
		"the snapshot must link a process to its parent, or trees stay truncated")
	assert.NotEmpty(t, self.Ancestor.FileEvent.PathnameStr,
		"the linked ancestor must carry an executable, not be a blank row")
	assert.EqualValues(t, os.Getppid(), self.Ancestor.Pid)
}
