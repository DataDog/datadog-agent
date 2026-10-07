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
	"strings"
	"sync"
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
			h := newHarness(t, withPath("app/*"), withStartPosition(mode))
			h.scan() // before app.log is created: it is read from the beginning
			rotateWithLockedDrain(t, h)
			require.Nil(t, h.scanner.active["app/app.log.1"], "app.log.1 waits for the drain")
			assert.Equal(t, identifier("app/app.log.1"), h.scanner.draining[0].t.CommitIdentifier())
			rotated := h.fileOf("app/app.log.1")

			restarted := h.restart()
			assert.Equal(t, tailer.EncodeOffset(rotated, int64(len(lines(1, 2)))), h.registry.GetOffset(identifier("app/app.log.1")),
				"the drain recorded where the rotated file stood under its new name")
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
				return
			}
			snapshot := metrics.MissedBytesSnapshot()
			require.Len(t, snapshot, 1)
			assert.Equal(t, int64(len(lines(3, 3))), snapshot[0].Bytes, "the line the replaced scanner listed but did not read")
		})
	}
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
	out := newCollector(t, provider.NextPipelineChan())
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
