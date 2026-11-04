// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && smb && !goexperiment.systemcrypto && !goexperiment.boringcrypto && !requirefips

package smb

// Tests of the limit on the tailers of all the SMB sources
// (logs_config.open_files_limit): which files start, that nothing is stopped for
// it, and that nothing is lost or read twice when a waiting file starts.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/smb"
)

func minute(n int) time.Time {
	return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Minute)
}

func withLimit(n int) harnessOption {
	return withLauncher(func(l *Launcher) { l.setOpenFilesLimit(n) })
}

// put writes the file p with content, last modified at mt.
func (h *harness) put(p, content string, mt time.Time) {
	h.t.Helper()
	h.share.Write(p, []byte(content))
	h.share.SetModTime(p, mt)
}

// another returns the scanner of a second source of the same launcher, on the
// same share, whose path is pattern. Its messages go to the same collector.
func (h *harness) another(pattern string) *harness {
	h.t.Helper()
	source := newSMBSource("smb-other", func(c *config.LogsConfig) { c.Path = pattern })
	require.NoError(h.t, source.Config.Validate())
	key, c := h.launcher.acquireClient(source.Config.SMB)
	s, err := newScanner(h.launcher, source, c, key)
	require.NoError(h.t, err)
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

// held counts the tailers and the drains of the scanners.
func held(hs ...*harness) int {
	n := 0
	for _, h := range hs {
		n += len(h.scanner.active) + len(h.scanner.draining)
	}
	return n
}

func TestOpenFilesLimitOption(t *testing.T) {
	assert.Equal(t, 7, NewLauncher(closeTimeout, WithOpenFilesLimit(7)).openFilesLimit)
	assert.Equal(t, defaultOpenFilesLimit, NewLauncher(closeTimeout).openFilesLimit)
	assert.Equal(t, defaultOpenFilesLimit, NewLauncher(closeTimeout, WithOpenFilesLimit(0)).openFilesLimit, "0 or less means the default, 500")
	assert.Equal(t, 500, defaultOpenFilesLimit)
}

// TestOpenFilesLimitIsSharedByAllSources: the limit counts the tailers of every
// SMB source together, as the file launcher counts those of every file source.
func TestOpenFilesLimitIsSharedByAllSources(t *testing.T) {
	h := newHarness(t, withLimit(4), withPath("app/*.log"))
	for i := 1; i <= 3; i++ {
		h.put("app/a"+string(rune('0'+i))+".log", lines(i, i), minute(i))
		h.put("other/b"+string(rune('0'+i))+".log", lines(10+i, 10+i), minute(10+i))
	}
	second := h.another("other/*.log")

	h.scan()
	second.scan()
	assert.Len(t, h.scanner.active, 3)
	assert.Equal(t, []string{"other/b3.log"}, sortedKeys(second.scanner.active), "the one slot left goes to the newest file")
	for range 3 {
		h.scan()
		second.scan()
	}
	assert.Equal(t, 4, held(h, second), "the limit holds, whatever the number of scans")
	assert.Empty(t, h.source.Messages.GetMessages())
	require.Len(t, second.source.Messages.GetMessages(), 1)
	assert.Contains(t, second.source.Messages.GetMessages()[0], "2 files not tailed (open_files_limit reached)")
	assert.True(t, second.source.Status().IsSuccess(), "files left out are no error")
}

// TestNewestFilesStartFirst: of the files that wait for a slot, the most recently
// modified start, whatever their names.
func TestNewestFilesStartFirst(t *testing.T) {
	h := newHarness(t, withLimit(2))
	for name, mt := range map[string]time.Time{"f1": minute(5), "f2": minute(1), "f3": minute(4), "f4": minute(2), "f5": minute(3)} {
		h.put("app/"+name+".log", lines(1, 1), mt)
	}
	h.scan()
	assert.Equal(t, []string{"app/f1.log", "app/f3.log"}, sortedKeys(h.scanner.active))
	require.Len(t, h.source.Messages.GetMessages(), 1)
	assert.Contains(t, h.source.Messages.GetMessages()[0], "3 files not tailed (open_files_limit reached)")
}

// TestOpenFilesLimitNeverStopsATailer: a newer file never takes the slot of a
// running tailer, however long the tailer reads nothing, and a stale file keeps
// its slot. That is the cost of a limit that never loses a line to a swap: the
// path of a source must not match files that stay for ever.
func TestOpenFilesLimitNeverStopsATailer(t *testing.T) {
	h := newHarness(t, withLimit(1))
	h.put("app/old.log", lines(1, 1), minute(1))
	h.scan()
	require.Equal(t, []string{"app/old.log"}, sortedKeys(h.scanner.active))

	h.put("app/new.log", lines(2, 2), minute(60))
	for range 5 {
		h.clock.Add(closeTimeout)
		h.scan()
	}
	assert.Equal(t, []string{"app/old.log"}, sortedKeys(h.scanner.active), "the tailer that reads nothing keeps its slot")
	assert.Empty(t, h.scanner.draining)
	h.share.Append("app/old.log", []byte(lines(3, 3)))
	h.scan()
	assert.Equal(t, []string{"line 1", "line 3"}, h.out.waitLines(t, 2), "and goes on reading; the newer file waits")
	require.Len(t, h.source.Messages.GetMessages(), 1)
	assert.Contains(t, h.source.Messages.GetMessages()[0], "1 files not tailed")
}

// TestRotationAtTheLimitDoesNotStallTheNewFile: the file a rotation leaves at the
// path starts at once, in the slot its old tailer held, while the drain of the
// rotated file holds a slot of the drain budget, so that the lines of the active
// file do not wait for the drain. A file that waits gets neither.
func TestRotationAtTheLimitDoesNotStallTheNewFile(t *testing.T) {
	h := newHarness(t, withLimit(1), withPath("app/app.log*"))
	h.put("app/app.log", lines(1, 1), minute(2))
	h.put("app/app.log.zz", lines(9, 9), minute(1)) // waits: no slot
	h.scan()
	h.out.waitLines(t, 1)
	require.Equal(t, []string{"app/app.log"}, sortedKeys(h.scanner.active))

	h.share.Append("app/app.log", []byte(lines(2, 2)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.put("app/app.log", lines(3, 3), minute(3))
	h.scan()
	assert.Equal(t, []string{"app/app.log"}, sortedKeys(h.scanner.active), "the new file has a tailer at once")
	require.Len(t, h.scanner.draining, 1, "and the rotated file a drain")
	assert.Equal(t, 2, held(h), "a tailer and a drain: each budget is full, none is over")
	assert.ElementsMatch(t, want(1, 3), h.out.waitLines(t, 3))

	// The drain ends once its file had no new data for a close timeout: the
	// limit holds again, and the file that waited did not take the slot.
	h.scanAfterCloseTimeout()
	assert.Empty(t, h.scanner.draining)
	assert.Equal(t, 1, held(h))
	assert.Equal(t, []string{"app/app.log"}, sortedKeys(h.scanner.active))
	assert.ElementsMatch(t, want(1, 3), h.finish())
}

// TestTruncationAtTheLimitRestartsTheFile: a file truncated at the limit is read
// again from its beginning, as it is without a limit.
func TestTruncationAtTheLimitRestartsTheFile(t *testing.T) {
	h := newHarness(t, withLimit(1))
	h.put("app/a.log", lines(1, 3), minute(1))
	h.put("app/b.log", lines(7, 7), minute(0))
	h.scan()
	h.out.waitLines(t, 3)
	require.NoError(t, h.share.Truncate("app/a.log", 0))
	h.share.Append("app/a.log", []byte(lines(4, 4)))
	h.share.SetModTime("app/b.log", time.Now().Add(time.Hour)) // the waiting file is now the newest: it must not take the slot
	h.scan()
	h.scan()
	assert.Equal(t, []string{"app/a.log"}, sortedKeys(h.scanner.active))
	assert.Equal(t, []string{"line 1", "line 2", "line 3", "line 4"}, h.out.waitLines(t, 4))
}

// TestWaitingFileStartsWithoutLossOrDuplicationWhenASlotFrees: the file that
// waits starts when a slot frees, from the beginning like any file that
// appeared after the source started, and each line of both files is sent once.
func TestWaitingFileStartsWithoutLossOrDuplicationWhenASlotFrees(t *testing.T) {
	h := newHarness(t, withLimit(1))
	h.put("app/f1.log", lines(1, 2), minute(2))
	h.put("app/f2.log", lines(3, 4), minute(1))
	h.scan()
	h.out.waitLines(t, 2)
	require.Equal(t, []string{"app/f1.log"}, sortedKeys(h.scanner.active))
	h.share.Append("app/f2.log", []byte(lines(5, 5))) // while it waits
	h.scan()

	require.NoError(t, h.share.Delete("app/f1.log"))
	h.scan() // the drain of the deleted file ends: the slot frees
	h.scan()
	assert.Equal(t, []string{"app/f2.log"}, sortedKeys(h.scanner.active))
	assert.Equal(t, want(1, 5), h.finish(), "each line once")
	assert.Empty(t, h.source.Messages.GetMessages())
}

// TestWaitingFileResumesFromItsRegistryOffset: a file that waited because of the
// limit starts where the registry says its previous tailer stopped, as any new
// tailer of the path does, and not from the beginning.
func TestWaitingFileResumesFromItsRegistryOffset(t *testing.T) {
	h := newHarness(t, withLimit(1), withStartPosition("end"))
	h.put("app/f1.log", lines(1, 1), minute(2))
	h.put("app/f2.log", lines(2, 4), minute(1))
	file := h.fileOf("app/f2.log")
	h.registry.SetOffset(identifier("app/f2.log"), tailer.EncodeOffset(file, int64(len(lines(2, 2)))))

	h.scan()
	require.Equal(t, []string{"app/f1.log"}, sortedKeys(h.scanner.active))
	require.NoError(t, h.share.Delete("app/f1.log"))
	h.scan()
	h.scan()
	assert.Equal(t, []string{"app/f2.log"}, sortedKeys(h.scanner.active))
	assert.Equal(t, want(3, 4), h.finish(), "from the registry offset: line 2 was delivered before")
}

// TestStoppedSourceGivesItsSlotsBack: the tailers of a source that stops, and the
// scanner a secret refresh replaces, no longer count.
func TestStoppedSourceGivesItsSlotsBack(t *testing.T) {
	h := newHarness(t, withLimit(1), withPath("app/*.log"))
	h.put("app/a.log", lines(1, 1), minute(1))
	h.put("other/b.log", lines(2, 2), minute(2))
	second := h.another("other/*.log")
	h.scan()
	second.scan()
	assert.Empty(t, second.scanner.active, "the limit is spent")

	h.scanner.stopTailers()
	second.scan()
	assert.Equal(t, []string{"other/b.log"}, sortedKeys(second.scanner.active))
	assert.Equal(t, 1, held(second))

	// A refreshed secret replaces the scanner: the new one takes the slots of
	// the old one, which no longer counts.
	refreshed := second.refresh()
	refreshed.scan()
	assert.Equal(t, []string{"other/b.log"}, sortedKeys(refreshed.scanner.active))
	assert.ElementsMatch(t, want(1, 2), refreshed.finish())
}

// TestLimitMessageClearsWhenEverythingStarts: the status message goes away once
// no file waits.
func TestLimitMessageClearsWhenEverythingStarts(t *testing.T) {
	h := newHarness(t, withLimit(1))
	h.put("app/f1.log", lines(1, 1), minute(2))
	h.put("app/f2.log", lines(2, 2), minute(1))
	h.scan()
	require.Len(t, h.source.Messages.GetMessages(), 1)
	h.launcher.setOpenFilesLimit(2)
	h.scan()
	assert.Empty(t, h.source.Messages.GetMessages())
	assert.Len(t, h.scanner.active, 2)
}

// TestWaitingFileNeverTakesARotatedPathsSlot: a writer that renames the file and
// creates the new one later (the rotated file keeps being written meanwhile, so
// its drain lasts) does not lose the path's slot to a file that waits, however
// long the new file takes: the path's next tailer takes over the slot its old
// tailer held, so the active file never waits.
func TestWaitingFileNeverTakesARotatedPathsSlot(t *testing.T) {
	h := newHarness(t, withLimit(1), withPath("app/app.log*"))
	h.put("app/app.log", lines(1, 1), minute(2))
	h.put("app/app.log.zz", lines(9, 9), minute(1)) // waits: no slot
	h.scan()
	h.out.waitLines(t, 1)

	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.scan() // sees the rename only
	require.Len(t, h.scanner.draining, 1)
	assert.Empty(t, h.scanner.active, "the file that waits does not take the slot of the path that rotated")

	for i := range 4 { // two close timeouts: the drain goes on, the writer still appends to the rotated file
		h.share.Append("app/app.log.1", []byte(lines(10+i, 10+i)))
		h.clock.Add(closeTimeout / 2)
		h.scan()
		assert.Empty(t, h.scanner.active, "scan %d", i)
		require.Len(t, h.scanner.draining, 1)
	}

	h.put("app/app.log", lines(3, 3), minute(3))
	h.scan()
	assert.Equal(t, []string{"app/app.log"}, sortedKeys(h.scanner.active), "the new file takes the slot of the path, past no limit")
	assert.Equal(t, 2, held(h), "one tailer and one drain, each within its own budget of 1")
	require.Len(t, h.source.Messages.GetMessages(), 1)
	assert.Contains(t, h.source.Messages.GetMessages()[0], "1 files not tailed", "the file that waited still waits")
	assert.Contains(t, h.out.waitLines(t, 6), "line 3")
}

// TestRotatedPathThatNeverReturnsGivesItsSlotBack: a path whose file was renamed
// away and never created again does not keep its tailer slot for ever: it ends
// with the rotated file's drain, and the file that waited starts.
func TestRotatedPathThatNeverReturnsGivesItsSlotBack(t *testing.T) {
	h := newHarness(t, withLimit(1), withPath("app/*.log"))
	h.put("app/day1.log", lines(1, 1), minute(2))
	h.put("app/day0.log", lines(9, 9), minute(1)) // waits: no slot
	h.scan()
	h.out.waitLines(t, 1)

	require.NoError(t, h.share.Rename("app/day1.log", "app/day1.gz"))
	h.scan()
	require.Len(t, h.scanner.draining, 1)
	assert.Empty(t, h.scanner.active)

	h.scanAfterCloseTimeout() // the drain ends: nothing is there to read
	require.Empty(t, h.scanner.draining)
	h.scan()
	assert.Equal(t, []string{"app/day0.log"}, sortedKeys(h.scanner.active), "the slot is free for the file that waited")
	assert.ElementsMatch(t, []string{"line 1", "line 9"}, h.finish())
}

// TestManyRotationsInOneScanAtFullBudgets: a writer that renames matched files at
// every scan fills the drain budget (open_files_limit drains). From then on the
// active files never stall, since the next tailer of a rotated path takes over
// the slot of the one it replaces, and each rotated file either has a drain or
// has the bytes the Agent had not read reported missed. The tailers never exceed
// the limit, and the drains do not either.
func TestManyRotationsInOneScanAtFullBudgets(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	const limit = 3
	h := newHarness(t, withLimit(limit), withPath("app/*.log"))
	names := []string{"a", "b", "c"}
	var paths []string
	for i, n := range names {
		paths = append(paths, "app/"+n+".log")
		h.put("app/"+n+".log", fmt.Sprintf("first %s\n", n), minute(i))
	}
	h.scan()
	require.Equal(t, paths, sortedKeys(h.scanner.active))
	h.out.waitLines(t, limit)

	var appended int64
	rotate := func(r int, rotating ...string) {
		for _, n := range rotating {
			cur := "app/" + n + ".log"
			rest := fmt.Sprintf("rest %s %d\n", n, r)
			h.share.Append(cur, []byte(rest))
			appended += int64(len(rest))
			require.NoError(t, h.share.Rename(cur, fmt.Sprintf("app/%s.%d", n, r)))
			h.put(cur, fmt.Sprintf("new %s %d\n", n, r), minute(10+r))
		}
		h.scan()
		require.Equal(t, paths, sortedKeys(h.scanner.active), "round %d: every active file has its tailer", r)
		assert.LessOrEqual(t, len(h.scanner.draining), limit, "round %d", r)
		assert.Empty(t, h.source.Messages.GetMessages(), "round %d: no file waits", r)
	}
	for r, rotating := range [][]string{{"a", "b"}, {"a", "b"}, {"a", "b", "c"}, {"a", "b", "c"}, {"c", "b"}} {
		rotate(r, rotating...)
	}
	require.Len(t, h.scanner.draining, limit, "the drain budget is full")

	// A drain that ends frees its slot: the next rotation gets a drain again.
	h.scanAfterCloseTimeout()
	require.Empty(t, h.scanner.draining)
	rotate(5, "a")
	assert.Len(t, h.scanner.draining, 1)

	var drained int64
	for _, line := range h.finish() {
		if strings.HasPrefix(line, "rest ") {
			drained += int64(len(line) + 1)
		}
	}
	var missed int64
	for _, summary := range metrics.MissedBytesSnapshot() {
		missed += summary.Bytes
	}
	require.Positive(t, drained, "the drains that fit read their file")
	require.Positive(t, missed, "the files that got no drain are reported missed")
	assert.Equal(t, appended, drained+missed, "every rotated file is drained or reported missed")
}

// TestRestartAtAFullDrainBudgetReportsTheRegistryPositionsRest: two files rotated
// away while the Agent was down, with one drain slot. The registry says where
// each was read to, not its size, so the unread rest of the file with no drain
// is the size the listing shows past that offset, reported missed. A resume
// point keeps its next tailer, when the file turns up at a matched name, from
// reading the bytes again or sending its lines.
func TestRestartAtAFullDrainBudgetReportsTheRegistryPositionsRest(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t, withPath("app/*.log"))
	h.put("app/a.log", lines(1, 2), minute(1))
	h.put("app/b.log", lines(11, 12), minute(2))
	h.scan()
	h.out.waitLines(t, 4)
	restarted := h.restart()
	restarted.launcher.setOpenFilesLimit(1)

	// Both files rotate while the Agent is down, to names the pattern does not
	// match. The registry offset of b is the larger, so b gets the one drain.
	for _, f := range []struct {
		name  string
		rest  string
		fresh string
		mt    time.Time
	}{{"a", lines(3, 3), lines(4, 4), minute(3)}, {"b", lines(13, 13), lines(14, 14), minute(4)}} {
		h.share.Append("app/"+f.name+".log", []byte(f.rest))
		require.NoError(t, h.share.Rename("app/"+f.name+".log", "app/"+f.name+".old"))
		h.put("app/"+f.name+".log", f.fresh, f.mt)
	}
	restarted.scan()
	require.Len(t, restarted.scanner.draining, 1, "one drain slot")
	assert.Equal(t, "app/b.old", restarted.scanner.draining[0].t.ReadPath())
	assert.Equal(t, []string{"app/b.log"}, sortedKeys(restarted.scanner.active), "the newest new file starts; the other waits")

	snapshot := metrics.MissedBytesSnapshot()
	require.Len(t, snapshot, 1, "the rest of a.old is reported missed")
	assert.EqualValues(t, len(lines(3, 3)), snapshot[0].Bytes)

	restarted.launcher.setOpenFilesLimit(5)
	require.NoError(t, h.share.Rename("app/a.old", "app/a2.log")) // the file with no drain turns up at a matched name
	restarted.scan()
	restarted.scan()
	assert.ElementsMatch(t, []string{"app/a.log", "app/a2.log", "app/b.log"}, sortedKeys(restarted.scanner.active))
	assert.ElementsMatch(t, []string{"line 1", "line 2", "line 11", "line 12", "line 13", "line 4", "line 14"}, restarted.finish(),
		"line 3 is missed, reported once, and not sent when its file turns up; every other line once")
}

// TestReplacedSourceKeepsItsSlots: the scanner that replaces another (a secret
// refresh, or a configuration change) holds the slots of the files it inherits,
// so another source whose files wait cannot take them before its first scan.
func TestReplacedSourceKeepsItsSlots(t *testing.T) {
	h := newHarness(t, withLimit(2), withPath("app/*.log"))
	h.put("app/a1.log", lines(1, 1), minute(1))
	h.put("app/a2.log", lines(2, 2), minute(2))
	h.put("other/b1.log", lines(3, 3), minute(3))
	second := h.another("other/*.log")
	h.scan()
	second.scan()
	require.Len(t, h.scanner.active, 2)
	require.Empty(t, second.scanner.active)

	refreshed := h.refresh() // the old scanner stopped, the new one has not scanned yet
	second.scan()
	assert.Empty(t, second.scanner.active, "the slots of the replaced source are not free for another source")

	refreshed.scan()
	assert.ElementsMatch(t, []string{"app/a1.log", "app/a2.log"}, sortedKeys(refreshed.scanner.active))
	assert.Equal(t, 2, held(refreshed, second))
	second.scan()
	assert.Empty(t, second.scanner.active)
	assert.ElementsMatch(t, want(1, 2), refreshed.finish(), "nothing read twice")
}

// TestSecretRefreshKeepsTheSlotOfAPathThatRotatedMeanwhile: a path the replaced
// scanner tailed rotates before the replacement's first scan. The replacement
// holds the path's slot for its next tailer, as for the paths that did not
// rotate, so the files that wait in another source take none of them.
func TestSecretRefreshKeepsTheSlotOfAPathThatRotatedMeanwhile(t *testing.T) {
	h := newHarness(t, withLimit(2), withPath("app/*.log"))
	h.put("app/a1.log", lines(1, 1), minute(1))
	h.put("app/a2.log", lines(2, 2), minute(2))
	h.put("other/b1.log", lines(3, 3), minute(3))
	second := h.another("other/*.log")
	h.scan()
	second.scan()
	h.out.waitLines(t, 2)
	require.Empty(t, second.scanner.active)

	refreshed := h.refresh()
	h.share.Append("app/a1.log", []byte(lines(5, 5)))
	require.NoError(t, h.share.Rename("app/a1.log", "app/a1.old"))
	h.put("app/a1.log", lines(6, 6), minute(5))
	second.scan()
	assert.Empty(t, second.scanner.active, "the slots of the replaced source are not free for another source")

	refreshed.scan()
	assert.Equal(t, []string{"app/a1.log", "app/a2.log"}, sortedKeys(refreshed.scanner.active))
	require.Len(t, refreshed.scanner.draining, 1)
	second.scan()
	assert.Empty(t, second.scanner.active)
	assert.ElementsMatch(t, []string{"line 1", "line 2", "line 5", "line 6"}, refreshed.finish())
}
