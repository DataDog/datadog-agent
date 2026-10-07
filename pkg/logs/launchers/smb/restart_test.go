// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && smb && !goexperiment.systemcrypto && !goexperiment.boringcrypto && !requirefips

package smb

// Tests of what survives a restart of the launcher (an Agent restart) or of a
// source (a secret refresh) while a rotated file is being drained, and of a
// password change on the server while a source tails a file.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client/fake"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/smb"
)

// commitOffsets does what the auditor does once the pipeline delivered the
// messages received so far: it stores each identifier's latest offset.
func (h *harness) commitOffsets() {
	h.out.flush()
	for _, msg := range h.out.messages() {
		if msg.Origin != nil && msg.Origin.Identifier != "" {
			h.registry.SetOffset(msg.Origin.Identifier, msg.Origin.Offset)
		}
	}
}

// restart stops the scanner, commits the offsets of what it sent, and returns
// a new launcher's scanner of the same source configuration on the same share
// and registry, as after an Agent restart. Its messages go to the same
// collector.
func (h *harness) restart() *harness {
	h.t.Helper()
	h.stop()
	h.commitOffsets()
	return h.relaunch()
}

// crash is restart after an Agent crash: nothing the scanner sent since the
// last commitOffsets was delivered, so none of it is committed.
func (h *harness) crash() *harness {
	h.t.Helper()
	h.stop()
	return h.relaunch()
}

// relaunch returns a new launcher's scanner of h's source configuration on
// the same share and registry, whose messages go to the same collector.
func (h *harness) relaunch() *harness {
	h.t.Helper()
	l := newTestLauncher(h.share, h.clock)
	l.pipelineProvider = h.launcher.pipelineProvider
	l.registry = h.registry
	l.processingRules = h.launcher.processingRules
	source := sources.NewLogSource(h.source.Name, h.source.Config)
	key, c := l.acquireClient(source.Config.SMB)
	s, err := newScanner(l, source, c, key)
	require.NoError(h.t, err)
	next := &harness{
		t:        h.t,
		share:    h.share,
		clock:    h.clock,
		launcher: l,
		registry: h.registry,
		source:   source,
		scanner:  s,
		out:      h.out,
		ctx:      h.ctx,
	}
	h.t.Cleanup(next.stop)
	return next
}

// refresh replaces the scanner with a scanner of the same source
// configuration that continues its work, as Launcher.replace does when a
// secret refresh configures the source again, and returns it. Its messages go
// to the same collector.
func (h *harness) refresh() *harness {
	h.t.Helper()
	source := sources.NewLogSource(h.source.Name, h.source.Config)
	key, c := h.launcher.acquireClient(source.Config.SMB)
	s, err := newScanner(h.launcher, source, c, key)
	require.NoError(h.t, err)
	h.scanner.replaced.Store(true)
	h.stop()
	s.resumeFrom(h.scanner)
	next := &harness{
		t:        h.t,
		share:    h.share,
		clock:    h.clock,
		launcher: h.launcher,
		registry: h.registry,
		source:   source,
		scanner:  s,
		out:      h.out,
		ctx:      h.ctx,
	}
	h.t.Cleanup(next.stop)
	return next
}

// rotateWithLockedDrain rotates app/app.log right after "line 3" was appended
// to it, while the rotated file is locked: the scan that sees the rotation
// knows line 3 is there but cannot read it, so the drain still holds it.
// Lines 1 and 2 were delivered and committed before the rotation. It returns
// the FileIds of the old and new files.
func rotateWithLockedDrain(t *testing.T, h *harness) (oldID, newID uint64) {
	t.Helper()
	oldID = h.share.Write("app/app.log", []byte(lines(1, 2)))
	h.scan()
	h.out.waitLines(t, 2)
	h.commitOffsets()
	h.share.Append("app/app.log", []byte(lines(3, 3)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	newID = h.share.Write("app/app.log", []byte(lines(4, 4)))
	h.share.FailNextPath(fake.OpReadAt, "app/app.log.1", fake.ErrSharing)
	h.scan()
	h.out.waitLines(t, 3)
	require.Len(t, h.scanner.draining, 1)
	require.Equal(t, int64(len(lines(3, 3))), h.scanner.draining[0].t.UnreadBytes())
	return oldID, newID
}

func TestRestartMidDrainResumesThePathsNewFile(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t)
	_, newID := rotateWithLockedDrain(t, h)

	restarted := h.restart()
	// The drain's messages committed no offset: the path's identifier holds
	// the position of its new file, whatever the drain read.
	file, offset, ok := tailer.DecodeOffset(h.registry.GetOffset(identifier("app/app.log")))
	require.True(t, ok)
	assert.Equal(t, newID, file.FileID)
	assert.Equal(t, int64(len(lines(4, 4))), offset)
	assert.Len(t, h.registry.StoredOffsets, 1, "nothing is committed under another identifier")

	restarted.share.Append("app/app.log", []byte(lines(5, 5)))
	restarted.scan()
	assert.Equal(t, []string{"line 1", "line 2", "line 4", "line 5"}, restarted.finish(),
		"the new file resumes where it stopped, and the rotated file, which the pattern does not match, is not read again")

	// Line 3 is lost with the drain: nothing resumes the rotated file after the
	// restart, since the path's registry entry names its new file. The stop
	// reports it missed.
	snapshot := metrics.MissedBytesSnapshot()
	require.Len(t, snapshot, 1)
	assert.Equal(t, int64(len(lines(3, 3))), snapshot[0].Bytes, "the drain's unread line")
}

// TestRestartMidDrainBeforeThePathsNewFileCommits restarts while a rotated
// file is drained and the path's new file has not committed an offset yet:
// the path's registry entry still names the rotated file, so the restarted
// launcher reads the rest of it from there, and the stop reports nothing
// missed.
func TestRestartMidDrainBeforeThePathsNewFileCommits(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t)
	oldID := h.share.Write("app/app.log", []byte(lines(1, 2)))
	h.scan()
	h.out.waitLines(t, 2)
	h.commitOffsets()
	h.share.Append("app/app.log", []byte(lines(3, 3)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", nil)
	h.share.FailNextPath(fake.OpReadAt, "app/app.log.1", fake.ErrSharing)
	h.scan()
	require.Len(t, h.scanner.draining, 1)
	require.Equal(t, int64(len(lines(3, 3))), h.scanner.draining[0].t.UnreadBytes())

	restarted := h.restart()
	assert.Empty(t, metrics.MissedBytesSnapshot(), "the restart resumes the rotated file")
	restarted.scan()
	require.Len(t, restarted.scanner.draining, 1)
	assert.Equal(t, oldID, restarted.scanner.draining[0].t.FileID(), "the rotated file is drained from its committed offset")
	restarted.share.Append("app/app.log", []byte(lines(4, 4)))
	restarted.scan()
	restarted.scan()
	assert.ElementsMatch(t, want(1, 4), restarted.finish(), "each line once")
	assert.Empty(t, metrics.MissedBytesSnapshot())
}

// TestRestartMidDrainWhileThePathsNewFileIsFilteredOut restarts while a
// rotated file is drained and the processing rules drop every line the
// path's new file sent: those lines commit nothing, so the path's registry
// entry still names the rotated file and the restart reads the rest of it
// from there. Once a line of the new file passes the rules, it commits under
// the path, and the drain's unread line is reported missed instead.
func TestRestartMidDrainWhileThePathsNewFileIsFilteredOut(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rule    harnessOption
		newFile string
		kept    bool // a line of the new file passes the rules
	}{
		{name: "every line dropped", rule: withExcludeAtMatch("^skip"), newFile: "skip 1\nskip 2\n"},
		{name: "every line dropped by a global rule", rule: withGlobalExcludeAtMatch("^skip"), newFile: "skip 1\nskip 2\n"},
		{name: "a line kept", rule: withExcludeAtMatch("^skip"), newFile: "skip 1\nline 4\n", kept: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metrics.ResetMissedBytesForTest()
			t.Cleanup(metrics.ResetMissedBytesForTest)
			h := newHarness(t, tc.rule)
			oldID := h.share.Write("app/app.log", []byte(lines(1, 2)))
			h.scan()
			h.out.waitLines(t, 2)
			h.commitOffsets()
			h.share.Append("app/app.log", []byte(lines(3, 3)))
			require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
			newID := h.share.Write("app/app.log", []byte(tc.newFile))
			h.share.FailNextPath(fake.OpReadAt, "app/app.log.1", fake.ErrSharing)
			h.scan()
			require.Len(t, h.scanner.draining, 1)
			require.Equal(t, int64(len(lines(3, 3))), h.scanner.draining[0].t.UnreadBytes())

			restarted := h.restart()
			file, _, ok := tailer.DecodeOffset(h.registry.GetOffset(identifier("app/app.log")))
			require.True(t, ok)
			if !tc.kept {
				assert.Equal(t, oldID, file.FileID, "the dropped lines committed nothing")
				assert.Empty(t, metrics.MissedBytesSnapshot(), "the restart resumes the rotated file")
				restarted.scan()
				require.Len(t, restarted.scanner.draining, 1)
				assert.Equal(t, oldID, restarted.scanner.draining[0].t.FileID(), "the rotated file is drained from its committed offset")
				restarted.scan()
				assert.ElementsMatch(t, want(1, 3), restarted.finish(), "each line once")
				assert.Empty(t, metrics.MissedBytesSnapshot())
				return
			}
			assert.Equal(t, newID, file.FileID, "the kept line committed under the path")
			snapshot := metrics.MissedBytesSnapshot()
			require.Len(t, snapshot, 1)
			assert.Equal(t, int64(len(lines(3, 3))), snapshot[0].Bytes, "the drain's unread line")
			restarted.scan()
			assert.Empty(t, restarted.scanner.draining, "the rotated file is not read again")
			assert.ElementsMatch(t, []string{"line 1", "line 2", "line 4"}, restarted.finish())
		})
	}
}

// TestRestartMidDrainCountsWhatTheStopFlushes stops the source while the
// path's new file has a line in its decoder only, held by a multi_line rule:
// stopping flushes it, and its offset commits under the path. The rotated
// file can then no longer be resumed from the path's registry entry, so its
// unread line is reported missed.
func TestRestartMidDrainCountsWhatTheStopFlushes(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t, withMultiLine("line"))
	configmock.New(t).SetInTest("logs_config.aggregation_timeout", 60000) // ms: nothing is flushed by time
	h.share.Write("app/app.log", []byte(lines(1, 2)))
	h.scan()
	h.out.waitLines(t, 1) // line 2 waits for the next line
	h.commitOffsets()
	h.share.Append("app/app.log", []byte(lines(3, 3)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	newID := h.share.Write("app/app.log", []byte(lines(4, 4)))
	h.share.FailNextPath(fake.OpReadAt, "app/app.log.1", fake.ErrSharing)
	h.scan()
	require.Len(t, h.scanner.draining, 1)
	require.False(t, h.activeTailer("app/app.log").Committed(), "line 4 waits in the decoder")

	restarted := h.restart()
	file, _, ok := tailer.DecodeOffset(h.registry.GetOffset(identifier("app/app.log")))
	require.True(t, ok)
	assert.Equal(t, newID, file.FileID, "line 4, flushed by the stop, committed under the path")
	snapshot := metrics.MissedBytesSnapshot()
	require.Len(t, snapshot, 1)
	assert.Equal(t, int64(len(lines(3, 3))), snapshot[0].Bytes, "the drain's unread line")
	restarted.scan()
	assert.Empty(t, restarted.scanner.draining, "the rotated file is not read again")
	assert.ElementsMatch(t, []string{"line 1", "line 2", "line 4"}, restarted.finish())
}

// TestStopMidDrainReportsUnreadBytes stops a source for good, as the Agent
// stops or the source is removed, while one of its rotated files is drained
// with a line it could not read yet. The pattern does not match the rotated
// name and the path's new file committed an offset, so nothing resumes the
// rotated file: the line is reported missed.
func TestStopMidDrainReportsUnreadBytes(t *testing.T) {
	for _, removed := range []bool{false, true} {
		t.Run(fmt.Sprintf("source removed %t", removed), func(t *testing.T) {
			metrics.ResetMissedBytesForTest()
			t.Cleanup(metrics.ResetMissedBytesForTest)
			st := startLauncher(t)
			source := newSMBSource("smb-test")
			st.share.Write("app/app.log", []byte(lines(1, 2)))
			st.sources.AddSource(source)
			st.out.waitLines(t, 2)

			st.share.Append("app/app.log", []byte(lines(3, 3)))
			require.NoError(t, st.share.Rename("app/app.log", "app/app.log.1"))
			st.share.Write("app/app.log", []byte(lines(4, 4)))
			drainRead := make(chan struct{})
			var once sync.Once
			st.share.SetHook(func(op fake.Op, p string) {
				if op == fake.OpReadAt && p == "app/app.log.1" {
					once.Do(func() { close(drainRead) })
				}
			})
			st.share.FailNextPath(fake.OpReadAt, "app/app.log.1", fake.ErrSharing)
			st.clock.Add(time.Second)
			st.out.waitLines(t, 3)
			select {
			case <-drainRead:
			case <-time.After(testTimeout):
				require.FailNow(t, "the drain never tried to read the rotated file")
			}

			if removed {
				st.sources.RemoveSource(source)
				st.waitFor(t, func() bool { return st.share.LiveSessions() == 0 }, "the source stops")
			} else {
				st.launcher.Stop()
			}
			snapshot := metrics.MissedBytesSnapshot()
			require.Len(t, snapshot, 1)
			assert.Equal(t, int64(len(lines(3, 3))), snapshot[0].Bytes, "the drain's unread line")
			assert.Equal(t, []string{strconv.Itoa(len(lines(3, 3)))}, source.GetInfoStatus()["Bytes Missed"], "agent status shows the loss on the source")
			st.out.flush()
			assert.Equal(t, []string{"line 1", "line 2", "line 4"}, st.out.lines())
		})
	}
}

// drainUnderAMatchedName writes "line 1" to app/app.log, then rotates it to
// app/app.log.1, which the pattern matches, appends "line 2" to it and writes
// "line 10" to the path's new file: the drain reads line 2 and commits its
// offsets under app.log.1. Every offset sent is committed. It returns the
// FileId of the rotated file.
func drainUnderAMatchedName(t *testing.T, h *harness) uint64 {
	t.Helper()
	rotated := h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.out.waitLines(t, 1)
	h.commitOffsets()
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", []byte(lines(10, 10)))
	h.share.Append("app/app.log.1", []byte(lines(2, 2)))
	h.scan()
	h.out.waitLines(t, 3)
	h.commitOffsets()
	require.Len(t, h.scanner.draining, 1)
	require.Equal(t, identifier("app/app.log.1"), h.scanner.draining[0].t.CommitIdentifier())
	return rotated
}

// moveDrainToAnExcludedName renames the file drainUnderAMatchedName drains to
// a name the pattern excludes and appends "line 3" to it while it is locked:
// the drain commits no offset anymore, and still has line 3 to read.
func moveDrainToAnExcludedName(t *testing.T, h *harness) {
	t.Helper()
	require.NoError(t, h.share.Rename("app/app.log.1", "app/app.log.1.tmp"))
	h.share.Append("app/app.log.1.tmp", []byte(lines(3, 3)))
	h.share.FailNextPath(fake.OpReadAt, "app/app.log.1.tmp", fake.ErrSharing)
	h.scan()
	require.Len(t, h.scanner.draining, 1)
	require.Empty(t, h.scanner.draining[0].t.CommitIdentifier())
	require.Equal(t, int64(len(lines(3, 3))), h.scanner.draining[0].t.UnreadBytes())
}

// restartAfterARotationToTheNameTheDrainLeft restarts h, whose drain
// moveDrainToAnExcludedName moved: the stop reports nothing missed, since the
// registry entry of app.log.1 still holds the rotated file. While the Agent
// is down, app.log rotates to app.log.1 in turn: the restarted launcher finds
// another file there, so it reads the rest of the rotated file from that
// entry.
func restartAfterARotationToTheNameTheDrainLeft(t *testing.T, h *harness, rotated uint64) {
	t.Helper()
	restarted := h.restart()
	assert.Empty(t, metrics.MissedBytesSnapshot(), "the registry entry of app.log.1 resumes the rotated file")
	file, _, ok := tailer.DecodeOffset(h.registry.GetOffset(identifier("app/app.log.1")))
	require.True(t, ok)
	require.Equal(t, rotated, file.FileID)

	require.NoError(t, restarted.share.Rename("app/app.log", "app/app.log.1"))
	restarted.share.Write("app/app.log", []byte(lines(20, 20)))
	for range 3 {
		restarted.scan()
	}
	assert.ElementsMatch(t, []string{"line 1", "line 10", "line 2", "line 20", "line 3"}, restarted.finish(), "each line once")
	assert.Empty(t, metrics.MissedBytesSnapshot())
}

// TestRestartMidDrainResumesFromAMatchedNameItLeft stops the source while a
// rotated file is drained under a name the pattern excludes, after the drain
// committed offsets under a matched name the file had before. That name's
// registry entry still holds the file, so a restart can resume it there: the
// stop reports nothing missed.
func TestRestartMidDrainResumesFromAMatchedNameItLeft(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t, withPath("app/app.log*"), withExcludes("app/*.tmp"))
	rotated := drainUnderAMatchedName(t, h)
	moveDrainToAnExcludedName(t, h)
	restartAfterARotationToTheNameTheDrainLeft(t, h, rotated)
}

// TestSecretRefreshBeforeDeliveryThenStopMidDrain replaces a source before
// the pipeline delivered the offsets its tailer sent, then the file rotates
// with a line the drain cannot read yet, and the Agent stops. The new
// scanner knows from the handoff that the path's registry entry holds the
// rotated file once those offsets are delivered, so the stop reports nothing
// missed and the restart reads the line from there. When the path's new file
// commits a line there before the stop, nothing resumes the rotated file
// anymore and its line is reported missed.
func TestSecretRefreshBeforeDeliveryThenStopMidDrain(t *testing.T) {
	for _, tc := range []struct {
		tailed         bool // the new scanner tailed the file before it rotated
		newFileCommits bool // the path's new file has a line
	}{
		{tailed: false},
		{tailed: true},
		{tailed: true, newFileCommits: true},
	} {
		t.Run(fmt.Sprintf("new scanner tailed the file %t, new file commits %t", tc.tailed, tc.newFileCommits), func(t *testing.T) {
			metrics.ResetMissedBytesForTest()
			t.Cleanup(metrics.ResetMissedBytesForTest)
			h := newHarness(t)
			h.share.Write("app/app.log", []byte(lines(1, 2)))
			h.scan()
			h.out.waitLines(t, 2)
			refreshed := h.refresh() // the registry holds no offset yet
			if tc.tailed {
				refreshed.scan()
				require.NotNil(t, refreshed.scanner.active["app/app.log"])
			}
			refreshed.share.Append("app/app.log", []byte(lines(3, 3)))
			require.NoError(t, refreshed.share.Rename("app/app.log", "app/app.log.1"))
			var newFile []byte
			if tc.newFileCommits {
				newFile = []byte(lines(4, 4))
			}
			refreshed.share.Write("app/app.log", newFile)
			refreshed.share.FailNextPath(fake.OpReadAt, "app/app.log.1", fake.ErrSharing)
			refreshed.scan()
			require.Len(t, refreshed.scanner.draining, 1)

			if tc.newFileCommits {
				refreshed.out.waitLines(t, 3)
				restarted := refreshed.restart()
				snapshot := metrics.MissedBytesSnapshot()
				require.Len(t, snapshot, 1, "line 4 committed under the path: nothing resumes the rotated file")
				assert.Equal(t, int64(len(lines(3, 3))), snapshot[0].Bytes)
				restarted.scan()
				assert.Empty(t, restarted.scanner.draining, "the rotated file is not read again")
				assert.ElementsMatch(t, []string{"line 1", "line 2", "line 4"}, restarted.finish())
				return
			}
			assert.True(t, refreshed.scanner.draining[0].resumePaths["app/app.log"], "the path's registry entry will hold the rotated file")
			restarted := refreshed.restart()
			assert.Empty(t, metrics.MissedBytesSnapshot(), "the restart resumes the rotated file from the path's registry entry")
			for range 3 {
				restarted.scan()
			}
			assert.ElementsMatch(t, want(1, 3), restarted.finish(), "each line once")
			assert.Empty(t, metrics.MissedBytesSnapshot())
		})
	}
}

// TestRestartMidDrainResumesARotatedFileMatchedByThePattern covers a pattern
// that matches the rotated name too. The rotated file's path gets its own
// tailer only when the drain ends and hands it over; until then, the drain
// commits its offsets under that path's identifier, starting with where the
// file stood when it rotated, so a restart before the handoff resumes the
// file there, whatever start_position says.
func TestRestartMidDrainResumesARotatedFileMatchedByThePattern(t *testing.T) {
	for _, mode := range []string{"beginning", "end"} {
		t.Run("start_position "+mode, func(t *testing.T) {
			metrics.ResetMissedBytesForTest()
			t.Cleanup(metrics.ResetMissedBytesForTest)
			h := newHarness(t, withPath("app/*"), withStartPosition(mode))
			h.scan() // before app.log is created: it is read from the beginning
			rotateWithLockedDrain(t, h)
			require.Nil(t, h.scanner.active["app/app.log.1"], "app.log.1 waits for the drain")
			assert.Equal(t, identifier("app/app.log.1"), h.scanner.draining[0].t.CommitIdentifier())
			rotated := h.fileOf("app/app.log.1")

			restarted := h.restart()
			assert.Equal(t, tailer.EncodeOffset(rotated, int64(len(lines(1, 2)))), h.registry.GetOffset(identifier("app/app.log.1")),
				"the drain recorded where the rotated file stood under its new name")
			assert.Empty(t, metrics.MissedBytesSnapshot(), "the stop reports nothing missed: the restart resumes the drain")
			restarted.scan()
			assert.ElementsMatch(t, want(1, 4), restarted.finish(), "each line once")
		})
	}
}

// TestDrainCommitsUnderTheMatchedPathItSitsAt follows the offsets a drain
// commits: under the identifier of the matched path its file is renamed to,
// none once the file is renamed to a name the pattern does not match, and
// again under the next matched name.
func TestDrainCommitsUnderTheMatchedPathItSitsAt(t *testing.T) {
	h := newHarness(t, withPath("app/app.log*"), withExcludes("app/*.tmp"))
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.out.waitLines(t, 1)
	h.commitOffsets()

	// The rotated file keeps growing, so the drain goes on.
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", nil)
	h.share.Append("app/app.log.1", []byte(lines(2, 2)))
	h.scan()
	h.out.waitLines(t, 2)
	require.Len(t, h.scanner.draining, 1)
	drain := h.scanner.draining[0].t
	assert.Equal(t, identifier("app/app.log.1"), drain.CommitIdentifier())
	assert.True(t, h.registry.TailedSources[identifier("app/app.log.1")])

	require.NoError(t, h.share.Rename("app/app.log.1", "app/app.log.1.tmp"))
	h.share.Append("app/app.log.1.tmp", []byte(lines(3, 3)))
	h.scan()
	h.out.waitLines(t, 3)
	assert.Empty(t, drain.CommitIdentifier(), "the excluded name is not tailed: nothing commits there")
	assert.False(t, h.registry.TailedSources[identifier("app/app.log.1")])

	require.NoError(t, h.share.Rename("app/app.log.1.tmp", "app/app.log.2"))
	h.share.Append("app/app.log.2", []byte(lines(4, 4)))
	h.scan()
	assert.Equal(t, identifier("app/app.log.2"), drain.CommitIdentifier())

	h.out.waitLines(t, 4)
	h.out.flush()
	file := h.fileOf("app/app.log.2")
	var commits []string
	for _, msg := range h.out.messages() {
		commits = append(commits, string(msg.GetContent())+" @ "+msg.Origin.Identifier+" "+msg.Origin.Offset)
	}
	assert.Equal(t, []string{
		"line 1 @ " + identifier("app/app.log") + " " + tailer.EncodeOffset(file, int64(len(lines(1, 1)))),
		"line 2 @ " + identifier("app/app.log.1") + " " + tailer.EncodeOffset(file, int64(len(lines(1, 2)))),
		"line 3 @  ",
		"line 4 @ " + identifier("app/app.log.2") + " " + tailer.EncodeOffset(file, int64(len(lines(1, 4)))),
	}, commits)
	assert.Equal(t, tailer.EncodeOffset(file, int64(len(lines(1, 1)))), h.registry.GetOffset(identifier("app/app.log.2")),
		"the drain records under a new name what was committed for its file, never what was only read")
	assert.Equal(t, want(1, 4), h.finish())
}

// TestDrainAfterAResumeRecordsOnlyWhatWasDelivered chains a drain, the tailer
// that resumes its file where the drain ended, and that tailer's own drain
// after the file is renamed again. The second drain records under its new
// path the offset delivered for the file, not the one the first drain read up
// to: when the Agent crashes before the first drain's line is delivered, the
// restarted Agent sends that line again instead of losing it.
func TestDrainAfterAResumeRecordsOnlyWhatWasDelivered(t *testing.T) {
	for _, delivered := range []bool{false, true} {
		t.Run(fmt.Sprintf("line 2 delivered %t", delivered), func(t *testing.T) {
			h := newHarness(t, withPath("app/app.log*"))
			h.share.Write("app/app.log", []byte(lines(1, 1)))
			h.scan()
			h.out.waitLines(t, 1)
			h.commitOffsets()

			h.share.Append("app/app.log", []byte(lines(2, 2)))
			require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
			h.share.Write("app/app.log", nil)
			for range 4 {
				h.scan() // the drain reads line 2 and ends; app.log.1's tailer resumes the file after it
			}
			require.Empty(t, h.scanner.draining)
			require.EqualValues(t, len(lines(1, 2)), h.activeTailer("app/app.log.1").Offset())
			h.out.waitLines(t, 2)
			if delivered {
				h.commitOffsets()
			}

			require.NoError(t, h.share.Rename("app/app.log.1", "app/app.log.2"))
			h.scan()
			require.Len(t, h.scanner.draining, 1)
			require.Equal(t, identifier("app/app.log.2"), h.scanner.draining[0].t.CommitIdentifier())
			committed := lines(1, 1)
			if delivered {
				committed = lines(1, 2)
			}
			assert.Equal(t, tailer.EncodeOffset(h.fileOf("app/app.log.2"), int64(len(committed))), h.registry.GetOffset(identifier("app/app.log.2")),
				"the drain records what was delivered of its file, not what was read")

			restarted := h.crash()
			sent := len(h.out.lines())
			restarted.scan()
			again := restarted.finish()[sent:]
			if delivered {
				assert.Empty(t, again)
			} else {
				assert.Equal(t, []string{"line 2"}, again, "the line that was not delivered is sent again")
			}
		})
	}
}

// TestRestartSpanningARotation rotates a file while the Agent is down: the
// registry holds the offset of the file renamed to a name the pattern does not
// match. The restarted launcher drains the rest of the rotated file from that
// offset, and reads the path's new file from the beginning.
func TestRestartSpanningARotation(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 2)))
	h.scan()
	h.out.waitLines(t, 2)

	restarted := h.restart()
	h.share.Append("app/app.log", []byte(lines(3, 3)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", []byte(lines(4, 4)))

	restarted.scan()
	require.Len(t, restarted.scanner.draining, 1, "the rotated file is drained from its stored offset")
	assert.Equal(t, "app/app.log.1", restarted.scanner.draining[0].t.ReadPath())
	restarted.scan()
	restarted.scan()
	assert.Empty(t, restarted.scanner.draining)
	assert.ElementsMatch(t, want(1, 4), restarted.finish(), "each line once")
	assert.Empty(t, metrics.MissedBytesSnapshot())
}

// TestRestartSpanningARotationToAMatchedName is TestRestartSpanningARotation
// with a pattern that matches the rotated name, which sorts before the path:
// the tailer of the rotated name resumes the file at its stored offset,
// although the scan starts it before it looks at the path.
func TestRestartSpanningARotationToAMatchedName(t *testing.T) {
	h := newHarness(t, withPath("app/*"))
	first := h.share.Write("app/b.log", []byte(lines(1, 2)))
	h.scan()
	h.out.waitLines(t, 2)

	restarted := h.restart()
	h.share.Append("app/b.log", []byte(lines(3, 3)))
	require.NoError(t, h.share.Rename("app/b.log", "app/a.log"))
	h.share.Write("app/b.log", []byte(lines(4, 4)))

	restarted.scan()
	assert.Empty(t, restarted.scanner.draining)
	assert.Equal(t, first, restarted.activeTailer("app/a.log").FileID())
	assert.ElementsMatch(t, want(1, 4), restarted.finish(), "each line once")
}

// TestRestartResumesARotatedFileFromItsFurthestStoredOffset: the registry
// holds offsets of one file under two paths, the one it rotated away from,
// whose new file sent nothing, and the matched name it rotated to. A restart
// finds other files at both, and the file at a name the pattern excludes: its
// drain starts from the furthest of the two offsets, whatever the order of the
// paths.
func TestRestartResumesARotatedFileFromItsFurthestStoredOffset(t *testing.T) {
	for _, rotatedTo := range []string{"app/app.log.1", "app/0.app.log"} {
		t.Run("rotated to "+rotatedTo, func(t *testing.T) {
			h := newHarness(t, withPath("app/*app.log*"), withExcludes("app/app.log.2"))
			h.share.Write("app/app.log", []byte(lines(1, 1)))
			h.scan()
			h.out.waitLines(t, 1)
			h.commitOffsets()
			// app.log rotates to a matched name; its new file stays empty.
			h.share.Append("app/app.log", []byte(lines(2, 2)))
			require.NoError(t, h.share.Rename("app/app.log", rotatedTo))
			h.share.Write("app/app.log", nil)
			h.scan()
			h.out.waitLines(t, 2)
			for range 2 {
				h.clock.Add(closeTimeout) // the drain ends
				h.scan()
			}
			require.Empty(t, h.scanner.draining)
			h.share.Append(rotatedTo, []byte(lines(3, 3)))
			h.scan() // the tailer of the rotated name resumes the file
			h.out.waitLines(t, 3)
			file := h.fileOf(rotatedTo)

			restarted := h.restart()
			require.Equal(t, tailer.EncodeOffset(file, int64(len(lines(1, 1)))), h.registry.GetOffset(identifier("app/app.log")))
			require.Equal(t, tailer.EncodeOffset(file, int64(len(lines(1, 3)))), h.registry.GetOffset(identifier(rotatedTo)))
			// While the Agent is down, the file moves to the excluded app.log.2
			// and both matched paths get other files.
			h.share.Append(rotatedTo, []byte(lines(4, 4)))
			require.NoError(t, h.share.Rename(rotatedTo, "app/app.log.2"))
			require.NoError(t, h.share.Rename("app/app.log", rotatedTo))
			h.share.Write("app/app.log", []byte(lines(5, 5)))
			sent := len(h.out.lines())

			restarted.scan()
			require.Len(t, restarted.scanner.draining, 1)
			for range 2 {
				restarted.clock.Add(closeTimeout) // the drain ends
				restarted.scan()
			}
			assert.Empty(t, restarted.scanner.draining)
			assert.ElementsMatch(t, want(4, 5), restarted.finish()[sent:], "lines 2 and 3, committed under the rotated name, are not sent again")
		})
	}
}

// TestStopAfterARestartThatShiftedTwoFilesDownReportsTheLockedDrain: X is
// committed under app.log.1 and Y under app.log. While the Agent is down both
// rotate one name down (X to the excluded app.log.2, Y to app.log.1) and a new
// app.log appears. The restart drains X from the registry entry of app.log.1,
// and the tailer of app.log.1 resumes Y there: Y is read to its end, so it
// forwards nothing, and its seed is the only thing that replaces X's offset in
// that entry. A stop while X is locked must report X's unread line, since the
// next restart finds Y under app.log.1 and nothing for X.
func TestStopAfterARestartThatShiftedTwoFilesDownReportsTheLockedDrain(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t, withPath("app/app.log*"), withExcludes("app/app.log.2"))
	x := h.share.Write("app/app.log.1", []byte(lines(1, 2)))
	y := h.share.Write("app/app.log", []byte(lines(11, 12)))
	h.scan()
	h.out.waitLines(t, 4)

	restarted := h.restart()
	require.Equal(t, tailer.EncodeOffset(h.fileOf("app/app.log.1"), int64(len(lines(1, 2)))), h.registry.GetOffset(identifier("app/app.log.1")))
	// While the Agent is down, X is locked and has a line more.
	require.NoError(t, h.share.Rename("app/app.log.1", "app/app.log.2"))
	h.share.Append("app/app.log.2", []byte(lines(3, 3)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", []byte(lines(21, 21)))
	h.share.FailNextPath(fake.OpReadAt, "app/app.log.2", fake.ErrSharing)

	restarted.scan()
	require.Len(t, restarted.scanner.draining, 1, "X is drained from its stored offset")
	require.Equal(t, x, restarted.scanner.draining[0].t.FileID())
	require.Equal(t, y, restarted.activeTailer("app/app.log.1").FileID(), "app.log.1 resumes Y")
	require.Equal(t, int64(len(lines(3, 3))), restarted.scanner.draining[0].t.UnreadBytes())
	restarted.out.waitLines(t, 5)

	restarted.stop()
	snapshot := metrics.MissedBytesSnapshot()
	require.Len(t, snapshot, 1, "nothing resumes X: app.log.1 holds Y")
	assert.Equal(t, int64(len(lines(3, 3))), snapshot[0].Bytes, "X's unread line")
	file, _, ok := tailer.DecodeOffset(h.registry.GetOffset(identifier("app/app.log.1")))
	require.True(t, ok)
	assert.Equal(t, y, file.FileID, "the registry entry of app.log.1 holds Y")
}

// TestRestartSpanningARotationToAMatchedNameAtTheEndOfTheFile is
// TestRestartSpanningARotationToAMatchedName with a file read to its end: the
// tailer that resumes it at its new name forwards nothing, and the path's new
// file overwrites the registry entry of the old name. Where the rotated file
// was read up to is recorded under its new name, so that a second restart
// neither reads it again (beginning) nor skips what was appended to it (end).
func TestRestartSpanningARotationToAMatchedNameAtTheEndOfTheFile(t *testing.T) {
	for _, mode := range []string{"beginning", "end"} {
		t.Run("start_position "+mode, func(t *testing.T) {
			h := newHarness(t, withPath("app/app.log*"), withStartPosition(mode))
			h.scan() // before app.log exists: it is read from the beginning
			h.share.Write("app/app.log", []byte(lines(1, 2)))
			h.scan()
			h.out.waitLines(t, 2)

			restarted := h.restart()
			require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
			h.share.Write("app/app.log", []byte(lines(3, 3)))
			restarted.scan()
			restarted.out.waitLines(t, 3)
			require.EqualValues(t, len(lines(1, 2)), restarted.activeTailer("app/app.log.1").Offset())
			assert.Equal(t, tailer.EncodeOffset(restarted.fileOf("app/app.log.1"), int64(len(lines(1, 2)))),
				h.registry.GetOffset(identifier("app/app.log.1")),
				"the resumed position is recorded under the rotated file's new name")

			again := restarted.restart() // commits line 3 under app.log
			again.share.Append("app/app.log.1", []byte(lines(4, 4)))
			again.scan()
			again.out.waitLines(t, 4)
			assert.ElementsMatch(t, want(1, 4), again.finish(), "each line once")
		})
	}
}

// TestSecretRefreshKeepsThePathsADrainCommittedUnder is
// TestPathADrainCommittedUnderIsNotResumedFromItsRegistryOffset with a secret
// refresh after the drain ended: the new scanner does not take the offset the
// replaced scanner's drain recorded under app.log.1 for a position from before
// it started either. The first file, which the drain read to its end, is not
// read again when app.log.1 receives the next rotated file.
func TestSecretRefreshKeepsThePathsADrainCommittedUnder(t *testing.T) {
	h := newHarness(t, withPath("app/app.log*"), withExcludes("app/app.log.2"))
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.out.waitLines(t, 1)
	h.commitOffsets()

	logs := captureLogs(t, func() {
		h.share.Append("app/app.log", []byte(lines(2, 2)))
		require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
		h.share.Write("app/app.log", []byte(lines(3, 3)))
		h.scan()
		require.Len(t, h.scanner.draining, 1)
		require.Equal(t, identifier("app/app.log.1"), h.scanner.draining[0].t.CommitIdentifier())

		require.NoError(t, h.share.Rename("app/app.log.1", "app/app.log.2"))
		h.scan()
		for i := 0; len(h.scanner.draining) > 0 && i < 5; i++ {
			h.clock.Add(closeTimeout)
			h.scan() // the drain ends at app.log.2, which the source excludes
		}
		require.Empty(t, h.scanner.draining)
		require.NotEmpty(t, h.registry.GetOffset(identifier("app/app.log.1")), "the drain committed under app.log.1")

		h = h.refresh()
		h.scan()
		require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
		h.share.Write("app/app.log", []byte(lines(4, 4)))
		for range 4 {
			h.scan()
		}
	})
	assert.ElementsMatch(t, want(1, 4), h.out.waitLines(t, 4))
	assert.ElementsMatch(t, want(1, 4), h.finish(), "each line is sent once")
	assert.NotContains(t, logs, "while it was not tailed")
}

// TestSecretRefreshMidDrainUnderAMatchedName replaces the scanner, as after a
// secret refresh, while a drain commits its offsets under the matched name its
// file sits at, then rotates the files again before the new scanner's first
// scan: the drained file leaves that name for one the source excludes. The new
// scanner still reads the rest of the drained file, and each line is sent
// once.
func TestSecretRefreshMidDrainUnderAMatchedName(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t, withPath("app/app.log*"), withExcludes("app/app.log.2"))
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.out.waitLines(t, 1)
	h.commitOffsets()
	// The rotated file is locked: its drain still has line 2 to read.
	h.share.Append("app/app.log", []byte(lines(2, 2)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", []byte(lines(3, 3)))
	h.share.FailNextPath(fake.OpReadAt, "app/app.log.1", fake.ErrSharing)
	h.scan()
	h.out.waitLines(t, 2)
	require.Len(t, h.scanner.draining, 1)
	require.Equal(t, identifier("app/app.log.1"), h.scanner.draining[0].t.CommitIdentifier())
	h.commitOffsets()

	refreshed := h.refresh()
	require.NoError(t, refreshed.share.Rename("app/app.log.1", "app/app.log.2"))
	require.NoError(t, refreshed.share.Rename("app/app.log", "app/app.log.1"))
	refreshed.share.Write("app/app.log", []byte(lines(4, 4)))
	refreshed.scan()
	refreshed.scan()
	assert.ElementsMatch(t, want(1, 4), refreshed.out.waitLines(t, 4))
	assert.ElementsMatch(t, want(1, 4), refreshed.finish(), "each line is sent once")
	assert.Empty(t, metrics.MissedBytesSnapshot())
}

// TestSecretRefreshSpanningARotation replaces a source, as after a secret
// refresh, right after its file rotated, before the scanner of the replaced
// source saw the rotation: the new scanner reads the rest of the rotated file
// from where the replaced one stopped. When the rotated file is gone already,
// the bytes the replaced scanner knew it held are reported missed.
func TestSecretRefreshSpanningARotation(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(fmt.Sprintf("rotated file deleted %t", deleted), func(t *testing.T) {
			metrics.ResetMissedBytesForTest()
			t.Cleanup(metrics.ResetMissedBytesForTest)
			st := startLauncher(t)
			entry := func(password string) *sources.LogSource {
				return newSMBSource("demo", func(c *config.LogsConfig) {
					c.IntegrationSource = "file:/etc/datadog-agent/conf.d/demo.d/conf.yaml"
					c.SMB.Password = password
				})
			}
			st.share.Write("app/app.log", []byte(lines(1, 2)))
			st.sources.AddSource(entry("old-key"))
			st.out.waitLines(t, 2)

			// The replaced scanner lists line 3 but cannot read it yet.
			st.share.Append("app/app.log", []byte(lines(3, 3)))
			reads := st.share.Calls(fake.OpReadAt)
			st.share.FailNextPath(fake.OpReadAt, "app/app.log", fake.ErrSharing)
			st.clock.Add(time.Second)
			st.waitFor(t, func() bool { return st.share.Calls(fake.OpReadAt) > reads }, "the scan tries to read line 3")

			require.NoError(t, st.share.Rename("app/app.log", "app/app.log.1"))
			if deleted {
				require.NoError(t, st.share.Delete("app/app.log.1"))
			}
			st.share.Write("app/app.log", []byte(lines(4, 4)))
			refreshed := entry("new-key")
			st.sources.AddSource(refreshed)
			wantLines := want(1, 4)
			if deleted {
				wantLines = []string{"line 1", "line 2", "line 4"}
			}
			st.out.waitLines(t, len(wantLines))

			st.launcher.Stop()
			st.out.flush()
			assert.ElementsMatch(t, wantLines, st.out.lines())
			if !deleted {
				assert.Empty(t, metrics.MissedBytesSnapshot())
				assert.NotContains(t, refreshed.GetInfoStatus(), "Bytes Missed")
				return
			}
			snapshot := metrics.MissedBytesSnapshot()
			require.Len(t, snapshot, 1)
			assert.Equal(t, int64(len(lines(3, 3))), snapshot[0].Bytes, "the line the replaced scanner listed but did not read")
			assert.EqualValues(t, len(lines(3, 3)), refreshed.BytesMissed.Get(), "counted for the source that found the loss")
		})
	}
}

// TestSecretRefreshMidDrain replaces a source, as autodiscovery does when its
// password secret is refreshed, while one of its rotated files is drained.
func TestSecretRefreshMidDrain(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	st := startLauncher(t)
	entry := func(password string) *sources.LogSource {
		return newSMBSource("demo", func(c *config.LogsConfig) {
			c.IntegrationSource = "file:/etc/datadog-agent/conf.d/demo.d/conf.yaml"
			c.SMB.Password = password
		})
	}
	st.share.Write("app/app.log", []byte(lines(1, 2)))
	previous := entry("old-key")
	st.sources.AddSource(previous)
	st.out.waitLines(t, 2)

	st.share.Append("app/app.log", []byte(lines(3, 3)))
	require.NoError(t, st.share.Rename("app/app.log", "app/app.log.1"))
	st.share.Write("app/app.log", []byte(lines(4, 4)))
	st.share.FailNextPath(fake.OpReadAt, "app/app.log.1", fake.ErrSharing)
	st.clock.Add(time.Second)
	st.out.waitLines(t, 3) // lines 1, 2 and 4; line 3 waits in the drain

	refreshed := entry("new-key")
	st.sources.AddSource(refreshed)
	st.waitFor(t, func() bool { return len(refreshed.GetInputs()) == 1 }, "the refreshed source tails the path")
	st.share.Append("app/app.log", []byte(lines(5, 5)))
	for range 3 {
		st.clock.Add(time.Second)
	}
	st.out.waitLines(t, 4)

	st.launcher.Stop()
	st.out.flush()
	// The new scanner goes on with the drain, with its own client: it reads
	// line 3 once the file is no longer locked.
	assert.ElementsMatch(t, want(1, 5), st.out.lines(), "each line once across the replacement")
	assert.Empty(t, metrics.MissedBytesSnapshot())
}

// draining reports whether agent status lists a drain.
func (st *started) draining() bool {
	for _, tl := range st.tracker.All() {
		if strings.Contains(tl.GetID(), "(rotated") {
			return true
		}
	}
	return false
}

// refreshableSource returns a source of the configuration entry
// TestSecretRefreshMidDrain uses, with password: adding one with another
// password replaces the previous one, as after a secret refresh.
func refreshableSource(password string, opts ...func(*config.LogsConfig)) *sources.LogSource {
	return newSMBSource("demo", append([]func(*config.LogsConfig){func(c *config.LogsConfig) {
		c.IntegrationSource = "file:/etc/datadog-agent/conf.d/demo.d/conf.yaml"
		c.SMB.Password = password
	}}, opts...)...)
}

// TestSecretRefreshMidDrainHandsTheDrainOver replaces a source while the
// rotated file it drains is locked, holding a line neither scanner reads: the
// new scanner's drain knows the line is there and ends as the replaced one
// would have, so the line is reported missed once, when the file disappears
// or at the deadline set when the file rotated.
func TestSecretRefreshMidDrainHandsTheDrainOver(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(fmt.Sprintf("rotated file deleted %t", deleted), func(t *testing.T) {
			metrics.ResetMissedBytesForTest()
			t.Cleanup(metrics.ResetMissedBytesForTest)
			st := startLauncher(t)
			var drainReads atomic.Int32
			st.share.SetHook(func(op fake.Op, p string) {
				if op == fake.OpReadAt && p == "app/app.log.1" {
					drainReads.Add(1)
				}
			})
			// scan runs one scan, one poll_interval later, and waits until
			// its drain tried to read the rotated file.
			scan := func() {
				t.Helper()
				reads := drainReads.Load()
				st.clock.Add(time.Second)
				st.waitFor(t, func() bool { return drainReads.Load() > reads }, "the drain polls the rotated file")
			}
			st.share.Write("app/app.log", []byte(lines(1, 2)))
			st.sources.AddSource(refreshableSource("old-key"))
			st.out.waitLines(t, 2)

			st.share.Append("app/app.log", []byte(lines(3, 3)))
			require.NoError(t, st.share.Rename("app/app.log", "app/app.log.1"))
			st.share.Write("app/app.log", []byte(lines(4, 4)))
			locked := make([]error, 200)
			for i := range locked {
				locked[i] = fake.ErrSharing
			}
			st.share.FailNextPath(fake.OpReadAt, "app/app.log.1", locked...)
			scan() // the rotation is seen: the drain ends by closeTimeout (a minute) from now
			st.out.waitLines(t, 3)
			for range 30 {
				scan()
			}

			if deleted {
				require.NoError(t, st.share.Delete("app/app.log.1"))
			}
			reads := drainReads.Load()
			refreshed := refreshableSource("new-key")
			st.sources.AddSource(refreshed)
			st.waitFor(t, func() bool { return len(refreshed.GetInputs()) == 1 }, "the refreshed source tails the path")
			if !deleted {
				st.waitFor(t, func() bool { return drainReads.Load() > reads }, "the new scanner polls the drain")
				for range 29 {
					scan()
				}
				assert.Empty(t, metrics.MissedBytesSnapshot(), "the drain goes on until its deadline")
				st.clock.Add(time.Second) // a minute after the rotation, not after the replacement
			}
			st.waitFor(t, func() bool { return !st.draining() }, "the drain ends")
			snapshot := metrics.MissedBytesSnapshot()
			require.Len(t, snapshot, 1)
			assert.Equal(t, int64(len(lines(3, 3))), snapshot[0].Bytes, "the line neither scanner could read")
			assert.EqualValues(t, len(lines(3, 3)), refreshed.BytesMissed.Get(), "counted for the source whose drain ended")

			st.launcher.Stop()
			st.out.flush()
			assert.Equal(t, []string{"line 1", "line 2", "line 4"}, st.out.lines())
			assert.Len(t, metrics.MissedBytesSnapshot(), 1, "reported once")
		})
	}
}

// TestSecretRefreshAfterADrainEnded replaces a source right after the drain of
// a file rotated to a matched name ended, before the tailer of that name
// started: the new scanner's tailer resumes the file where the drain ended,
// although the registry does not hold the drain's last offsets yet (the
// pipeline has not delivered them).
func TestSecretRefreshAfterADrainEnded(t *testing.T) {
	st := startLauncher(t)
	matchAll := func(c *config.LogsConfig) { c.Path = "app/*" }
	var drainReads atomic.Int32
	st.share.SetHook(func(op fake.Op, p string) {
		if op == fake.OpReadAt && p == "app/app.log.1" {
			drainReads.Add(1)
		}
	})
	st.share.Write("app/app.log", []byte(lines(1, 2)))
	st.sources.AddSource(refreshableSource("old-key", matchAll))
	st.out.waitLines(t, 2)

	st.share.Append("app/app.log", []byte(lines(3, 3)))
	require.NoError(t, st.share.Rename("app/app.log", "app/app.log.1"))
	st.share.Write("app/app.log", []byte(lines(4, 4)))
	// The drain reads line 3, then two polls find nothing new: it ends. Each
	// scan reads the rotated file once.
	for scan := int32(1); scan <= 3; scan++ {
		st.clock.Add(time.Second)
		st.waitFor(t, func() bool { return drainReads.Load() == scan }, "the drain polls the rotated file")
	}
	st.waitFor(t, func() bool { return !st.draining() }, "the drain ends")
	st.out.waitLines(t, 4)

	refreshed := refreshableSource("new-key", matchAll)
	st.sources.AddSource(refreshed)
	st.waitFor(t, func() bool { return len(refreshed.GetInputs()) == 2 }, "the refreshed source tails both files")
	st.launcher.Stop()
	st.out.flush()
	assert.ElementsMatch(t, want(1, 4), st.out.lines(), "each line once across the replacement")
}

// TestSecretRefreshMidDrainKeepsTheMatchedNamesItLeft is
// TestRestartMidDrainResumesFromAMatchedNameItLeft with a secret refresh
// after the drain committed offsets under the matched name: the new scanner's
// drain knows that the registry entry of that name holds its file, so its
// stop reports nothing missed either.
func TestSecretRefreshMidDrainKeepsTheMatchedNamesItLeft(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t, withPath("app/app.log*"), withExcludes("app/*.tmp"))
	rotated := drainUnderAMatchedName(t, h)
	refreshed := h.refresh()
	moveDrainToAnExcludedName(t, refreshed)
	restartAfterARotationToTheNameTheDrainLeft(t, refreshed, rotated)
}

// passwordServer is a dial function in front of a fake share that only
// accepts one password, which the test changes like an administrator rotating
// the account's password or storage key.
type passwordServer struct {
	share *fake.Share
	mu    sync.Mutex
	valid string
	dials map[string]int // by password
}

func (p *passwordServer) setPassword(password string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.valid = password
}

func (p *passwordServer) dialsWith(password string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dials[password]
}

func (p *passwordServer) dial(ctx context.Context, cfg client.Config) (client.Client, error) {
	p.mu.Lock()
	p.dials[cfg.Password]++
	ok := cfg.Password == p.valid
	p.mu.Unlock()
	if !ok {
		// The server's message names the account, never the password; the
		// error carries it anyway to check that nothing prints it.
		return nil, fmt.Errorf("session setup for %s with %s: %w", cfg.Username, cfg.Password, fake.ErrAuth)
	}
	return p.share.Dial(ctx, cfg)
}

// TestPasswordChangedOnTheServerWhileTailing covers a password (or storage
// account key) rotated on the server while a source tails a file: the source
// reports an authentication error once its session is gone, without either
// password, then recovers when its configuration gets the new password,
// without waiting for the old password's authentication backoff and without
// sending a line twice or skipping one.
func TestPasswordChangedOnTheServerWhileTailing(t *testing.T) {
	configmock.New(t)
	share := fake.New()
	share.Mkdir("app")
	clk := clock.NewMock()
	server := &passwordServer{share: share, valid: "old-key", dials: make(map[string]int)}
	l := newTestLauncher(share, clk)
	l.dial = server.dial
	provider := mock.NewMockProvider()
	logSources := sources.NewLogSources()
	out := newCollector(t, provider.NextPipelineChan(), nil)
	l.Start(logSources, provider, auditorMock.NewMockRegistry(), tailers.NewTailerTracker())
	t.Cleanup(l.Stop)
	waitFor := func(cond func() bool, msg string) {
		t.Helper()
		require.Eventually(t, cond, testTimeout, time.Millisecond, msg)
	}

	entry := func(password string) *sources.LogSource {
		return newSMBSource("demo", func(c *config.LogsConfig) {
			c.IntegrationSource = "file:/etc/datadog-agent/conf.d/demo.d/conf.yaml"
			c.SMB.Password = password
		})
	}
	share.Write("app/app.log", []byte(lines(1, 2)))
	previous := entry("old-key")
	var statuses []string
	logs := captureLogs(t, func() {
		logSources.AddSource(previous)
		out.waitLines(t, 2)

		// The key is rotated on the server, which then drops the session (its
		// ticket expired, the server restarted). The session served for long
		// enough that its loss is routine: the client dials again at once.
		server.setPassword("new-key")
		clk.Add(30 * time.Second)
		share.DropSessions()
		share.Append("app/app.log", []byte(lines(3, 3)))
		clk.Add(time.Second) // the scan finds the session gone
		waitFor(func() bool { return previous.Status().IsError() }, "error status for the lost session")
		clk.Add(time.Second) // the next one dials with the old key
		waitFor(func() bool {
			return strings.Contains(previous.Status().GetError(), "rejected the credentials")
		}, "authentication error status")
		statuses = append(statuses, previous.Status().GetError())
		clk.Add(time.Second)
		clk.Add(time.Second)
		assert.Equal(t, 2, server.dialsWith("old-key"), "the rejected key is not tried again during its 30s backoff")

		// The secret is refreshed: the new source dials with the new key right
		// away, although the old key is still in its 30s backoff.
		refreshed := entry("new-key")
		logSources.AddSource(refreshed)
		waitFor(func() bool { return refreshed.Status().IsSuccess() }, "the refreshed source connects")
		assert.Equal(t, 1, server.dialsWith("new-key"))
		assert.True(t, previous.IsHiddenFromStatus())
		share.Append("app/app.log", []byte(lines(4, 4)))
		clk.Add(time.Second)
		out.waitLines(t, 4)
		statuses = append(statuses, refreshed.Status().GetError(), refreshed.Dump(true), previous.Dump(true))
		clk.Add(time.Minute)
		assert.Equal(t, 2, server.dialsWith("old-key"), "the replaced source's client is closed: the old key is never tried again")

		l.Stop()
	})
	out.flush()
	assert.Equal(t, want(1, 4), out.lines(), "each line once, across the password change")
	for _, s := range append(statuses, logs) {
		assert.NotContains(t, s, "old-key")
		assert.NotContains(t, s, "new-key")
	}
}
