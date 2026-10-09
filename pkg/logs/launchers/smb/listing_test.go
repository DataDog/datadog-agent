// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && smb && !goexperiment.systemcrypto && !goexperiment.boringcrypto && !requirefips

package smb

// Tests of the directories the client does not list, because they are too large
// or because the scan has listed enough: what the scanner still reads there, and
// what it never loses or reads twice when they come back.

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client/fake"
)

// tooLarge returns n listing errors for dir, which the fake share returns for
// the next n listings of it.
func tooLarge(dir string, n int) []error {
	errs := make([]error, n)
	for i := range errs {
		errs[i] = client.TooManyEntries(dir)
	}
	return errs
}

func repeatErr(err error, n int) []error {
	errs := make([]error, n)
	for i := range errs {
		errs[i] = err
	}
	return errs
}

// listings counts, by directory, the listings the scanner asks the share for
// from now on.
func (h *harness) listings() func() map[string]int {
	var (
		mu     sync.Mutex
		counts = make(map[string]int)
	)
	h.share.SetHook(func(op fake.Op, p string) {
		if op == fake.OpListDir {
			mu.Lock()
			counts[p]++
			mu.Unlock()
		}
	})
	return func() map[string]int {
		mu.Lock()
		defer mu.Unlock()
		out := make(map[string]int, len(counts))
		for p, n := range counts {
			out[p] = n
		}
		clear(counts)
		return out
	}
}

// TestOversizedDirectoryStatus covers a directory the client refuses to list
// because it has too many entries: the status says so, the listing is not
// retried on every poll, and the source recovers when the directory shrinks.
func TestOversizedDirectoryStatus(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.share.FailNextPath(fake.OpListDir, "app", client.TooManyEntries("app"), client.TooManyEntries("app"))

	h.scan()
	require.True(t, h.source.Status().IsError())
	msg := h.source.Status().GetError()
	assert.Contains(t, msg, "has more than 100000 entries")
	assert.Contains(t, msg, "Point the source's path at a directory with fewer files")
	assert.NotContains(t, msg, testPassword)
	assert.Empty(t, h.scanner.active, "nothing is tailed from a directory that cannot be listed")
	assert.Equal(t, 1, h.share.Calls(fake.OpListDir))

	// The scans of the next minute reuse the answer instead of reading 100,000
	// entries again.
	for range 5 {
		h.clock.Add(10 * time.Second)
		h.scan()
	}
	assert.Equal(t, 1, h.share.Calls(fake.OpListDir), "no listing for a minute")
	assert.True(t, h.source.Status().IsError())

	h.clock.Add(15 * time.Second)
	h.scan()
	assert.Equal(t, 2, h.share.Calls(fake.OpListDir), "asked again after a minute: still too large")
	assert.True(t, h.source.Status().IsError())

	// It was still too large: the next wait is twice as long.
	h.clock.Add(oversizedRetry)
	h.scan()
	assert.Equal(t, 2, h.share.Calls(fake.OpListDir), "no listing before twice the wait")

	// The directory shrinks.
	h.clock.Add(oversizedRetry)
	h.scan()
	assert.Equal(t, 3, h.share.Calls(fake.OpListDir))
	assert.True(t, h.source.Status().IsSuccess())
	assert.Contains(t, sortedKeys(h.scanner.active), "app/app.log")
	assert.Empty(t, h.scanner.oversized)
}

// TestRefusedListingsShowInTheStatus covers the other listings the client
// refuses, which take more memory than the limit or have a name no file has: the
// status says why, and the listing is not retried on every poll.
func TestRefusedListingsShowInTheStatus(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		text string
	}{
		"byte budget": {client.ListingTooLarge("app"), "take more than 64 MiB"},
		"long name":   {client.NameTooLong("app"), "longer than 255 characters"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.share.Write("app/app.log", []byte(lines(1, 1)))
			h.share.FailNextPath(fake.OpListDir, "app", tc.err)

			h.scan()
			require.True(t, h.source.Status().IsError())
			msg := h.source.Status().GetError()
			assert.Contains(t, msg, tc.text)
			assert.Contains(t, msg, "finds no new file there")
			assert.Empty(t, h.scanner.active)

			for range 5 {
				h.clock.Add(10 * time.Second)
				h.scan()
			}
			assert.Equal(t, 1, h.share.Calls(fake.OpListDir), "no listing for a minute")

			h.clock.Add(oversizedRetry)
			h.scan()
			assert.True(t, h.source.Status().IsSuccess())
			assert.Contains(t, sortedKeys(h.scanner.active), "app/app.log")
		})
	}
}

// TestOversizedRetryBacksOffPerDirectory: a directory that is still too large
// when the scanner asks again waits twice as long, up to a limit, so that what
// the scanner spends on the directories that stay too large stops growing with
// each minute.
func TestOversizedRetryBacksOffPerDirectory(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.share.FailNextPath(fake.OpListDir, "app", tooLarge("app", 100)...)
	listed := h.listings()
	var at []int
	h.scan()
	if listed()["app"] > 0 {
		at = append(at, 0)
	}
	for m := 1; m <= 31; m++ {
		h.clock.Add(time.Minute)
		h.scan()
		if listed()["app"] > 0 {
			at = append(at, m)
		}
	}
	assert.Equal(t, []int{0, 1, 3, 7, 15, 30}, at, "minutes at which the directory was listed")
	assert.Contains(t, h.scanner.statusError(&scanErrors{kind: client.ErrTooLarge, err: errors.New("too large")}).Error(), "less and less often")
}

// TestOversizedDirectoryKeepsTailingKnownFiles: a directory that grew past what
// the client lists does not stop the tailers of its files: they are read by
// name.
func TestOversizedDirectoryKeepsTailingKnownFiles(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.out.waitLines(t, 1)

	h.share.FailNextPath(fake.OpListDir, "app", tooLarge("app", 50)...)
	h.scan()
	require.True(t, h.source.Status().IsError(), "the status says the directory is not listed")
	h.share.Append("app/app.log", []byte(lines(2, 2)))
	for range 4 {
		h.scan()
	}
	assert.Equal(t, want(1, 2), h.out.waitLines(t, 2), "the file is still read")
	assert.Contains(t, h.scanner.active, "app/app.log")
}

// TestExactPathInOversizedDirectoryIsReadByName: a source that names its file
// needs no listing of the directory, however large it is.
func TestExactPathInOversizedDirectoryIsReadByName(t *testing.T) {
	h := newHarness(t, withPath("app/app.log"))
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.share.FailNextPath(fake.OpListDir, "app", tooLarge("app", 1)...)
	h.scan()
	h.scan()
	assert.Equal(t, []string{"line 1"}, h.out.waitLines(t, 1))
	h.share.Append("app/app.log", []byte(lines(2, 2)))
	h.scan()
	assert.Equal(t, want(1, 2), h.out.waitLines(t, 2))
	assert.True(t, h.source.Status().IsError(), "the status still says the directory cannot be listed")
}

// TestInheritedPathsInCappedDirectoryStart: a source replaced while a directory is
// not listed goes on tailing its files there.
func TestInheritedPathsInCappedDirectoryStart(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.out.waitLines(t, 1)
	h.share.FailNextPath(fake.OpListDir, "app", tooLarge("app", 50)...)
	h.scan()

	h2 := h.refresh()
	h2.share.Append("app/app.log", []byte(lines(2, 2)))
	for range 8 {
		h2.scan()
	}
	assert.Equal(t, want(1, 2), h2.out.waitLines(t, 2))
	assert.Contains(t, h2.scanner.active, "app/app.log")
}

// TestCappedDirRotationReadmitsPath: a file rotated in a directory that cannot be
// listed is read from its new file by name, and when the directory comes back
// the rotated file is resumed where it was, not read from its beginning.
func TestCappedDirRotationReadmitsPath(t *testing.T) {
	h := newHarness(t, withPath("app/app.log*"))
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.out.waitLines(t, 1)

	h.share.FailNextPath(fake.OpListDir, "app", tooLarge("app", 1)...)
	h.scan()
	h.share.Append("app/app.log", []byte(lines(2, 2)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", []byte(lines(3, 3)))
	for range 8 {
		h.scan()
	}
	assert.Contains(t, h.scanner.active, "app/app.log", "the path has a tailer for its new file")
	h.out.waitLines(t, 2)

	h.clock.Add(oversizedRetry)
	for range 3 {
		h.scan()
	}
	assert.ElementsMatch(t, want(1, 3), h.finish(), "each line once: the rotated file resumes where it was")
}

// TestDrainInOversizedDirectoryEndsAndReportsMissedBytes: a drain whose file is in
// a directory that cannot be listed any more does not last for ever: it ends at
// its deadline, and the bytes it could not read are reported missed.
func TestDrainInOversizedDirectoryEndsAndReportsMissedBytes(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t, withPath("app/app.log*"))
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.out.waitLines(t, 1)
	h.share.Append("app/app.log", []byte(lines(2, 2)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", []byte(lines(3, 3)))
	h.share.FailNextPath(fake.OpReadAt, "app/app.log.1", repeatErr(fake.ErrSharing, 80)...)
	h.scan()
	require.Len(t, h.scanner.draining, 1)

	h.share.FailNextPath(fake.OpListDir, "app", tooLarge("app", 200)...)
	for i := 0; i < 30 && len(h.scanner.draining) > 0; i++ {
		h.clock.Add(closeTimeout)
		h.scan()
	}
	require.Empty(t, h.scanner.draining, "the drain ended at its deadline")
	snapshot := metrics.MissedBytesSnapshot()
	require.Len(t, snapshot, 1)
	assert.Equal(t, int64(len(lines(2, 2))), snapshot[0].Bytes)
}

// TestOversizedEntryForgottenWhenTheDirectoryIsGone: what the scanner keeps of a
// directory it refused to list is forgotten when the directory is gone.
func TestOversizedEntryForgottenWhenTheDirectoryIsGone(t *testing.T) {
	h := newHarness(t, withPath("*/app.log"))
	h.share.Mkdir("x")
	h.share.Write("x/app.log", []byte(lines(1, 1)))
	h.share.Write("app/app.log", []byte(lines(2, 2)))
	h.share.FailNextPath(fake.OpListDir, "x", tooLarge("x", 1)...)
	h.scan()
	require.Contains(t, h.scanner.oversized, "x")

	h.share.Rmdir("x")
	h.clock.Add(oversizedRetry)
	h.scan()
	assert.Empty(t, h.scanner.oversized)
}

// TestOversizedDirectoryDoesNotMakeLaterFilesInitial: a directory that cannot be
// listed does not keep the files created in the other directories afterwards
// from being read from their beginning. The files of the directory itself that
// were there when the source started keep start_position when it comes back.
func TestOversizedDirectoryDoesNotMakeLaterFilesInitial(t *testing.T) {
	h := newHarness(t, withPath("*/*.log"), withStartPosition("end"))
	h.share.Mkdir("big")
	h.share.Write("big/old.log", []byte(lines(1, 2)))
	h.share.Write("app/old.log", []byte(lines(3, 3)))
	h.share.FailNextPath(fake.OpListDir, "big", tooLarge("big", 1)...)
	h.scan()
	assert.Equal(t, []string{"app/old.log"}, sortedKeys(h.scanner.active))

	time.Sleep(5 * time.Millisecond)                  // the share stamps files with the wall clock
	h.share.Write("app/new.log", []byte(lines(4, 4))) // created after the source started
	h.scan()
	assert.Equal(t, []string{"line 4"}, h.out.waitLines(t, 1), "read from its beginning")

	// The directory shrinks: what was there stays where it was, what is written
	// to it after is read.
	h.clock.Add(oversizedRetry)
	h.scan()
	assert.Contains(t, h.scanner.active, "big/old.log")
	h.share.Append("big/old.log", []byte(lines(5, 5)))
	h.scan()
	assert.Equal(t, []string{"line 4", "line 5"}, h.out.waitLines(t, 2), "the history of the directory is not shipped")
	assert.Equal(t, []string{"line 4", "line 5"}, h.finish())
}

// TestFileCreatedWhileADirectoryWasCappedIsReadInFull: start_position applies to
// the files that were there when the source started. A directory that could not
// be listed at the start, and can be later, holds files that were created in the
// meantime: they are new, whatever the directory's history, and are read from
// their beginning. Only the files created before the source started keep
// start_position.
func TestFileCreatedWhileADirectoryWasCappedIsReadInFull(t *testing.T) {
	h := newHarness(t, withPath("*/*.log"), withStartPosition("end"))
	h.share.Mkdir("big")
	h.share.Write("big/old.log", []byte(lines(1, 2)))
	h.share.Write("app/old.log", []byte(lines(3, 3)))
	h.share.FailNextPath(fake.OpListDir, "big", tooLarge("big", 1)...)
	h.scan()
	require.Equal(t, []string{"app/old.log"}, sortedKeys(h.scanner.active))

	time.Sleep(5 * time.Millisecond) // the share stamps files with the wall clock
	h.share.Write("big/new.log", []byte(lines(4, 4)))
	h.clock.Add(oversizedRetry)
	h.scan() // the directory can be listed again
	h.share.Append("big/old.log", []byte(lines(5, 5)))
	h.scan()
	assert.Contains(t, h.scanner.active, "big/new.log")
	assert.ElementsMatch(t, []string{"line 4", "line 5"}, h.finish(), "the new file in full, the old file from where the source started")
}

// TestFileDeletedWhileTheSourceWasDownIsNotDrainedInACappedSource: the file the
// registry holds for a path is gone when the source starts again, and another
// directory cannot be listed. The file was listed in the directory of the path,
// and is not in it any more: there is nothing to drain.
func TestFileDeletedWhileTheSourceWasDownIsNotDrainedInACappedSource(t *testing.T) {
	h := newHarness(t, withPath("*/app.log"))
	h.share.Mkdir("big")
	h.share.Write("app/app.log", []byte(lines(1, 2)))
	h.scan()
	h.out.waitLines(t, 2)
	h.commitOffsets()

	restarted := h.restart()
	require.NoError(t, h.share.Delete("app/app.log"))
	h.share.Write("app/app.log", []byte(lines(3, 3)))
	h.share.FailNextPath(fake.OpListDir, "big", tooLarge("big", 500)...)
	restarted.scan()
	assert.Empty(t, restarted.scanner.draining, "the old file is nowhere: no drain for it")
	assert.Equal(t, want(1, 3), restarted.finish())
}

// manyDirectories creates n directories of files files each, d0 to d<n-1>, and
// returns the paths of their files.
func manyDirectories(h *harness, n, files int) []string {
	var paths []string
	for d := range n {
		for f := range files {
			p := fmt.Sprintf("d%d/f%d.log", d, f)
			h.share.Write(p, []byte("old\n"))
			paths = append(paths, p)
		}
	}
	return paths
}

// TestScanBudgetBoundsTheWholeScan: one scan lists the directories of a pattern
// until it has listed as many entries as its budget allows, and the directories
// it leaves are listed first by the next one, so that every directory is listed
// in a few scans.
func TestScanBudgetBoundsTheWholeScan(t *testing.T) {
	h := newHarness(t, withPath("d*/f*.log"), withLauncher(func(l *Launcher) { l.listEntries = 50 }))
	paths := manyDirectories(h, 10, 20)

	h.scan()
	assert.LessOrEqual(t, h.share.Calls(fake.OpListDir), 4, "the root and the directories that fit the budget")
	require.Len(t, h.source.Messages.GetMessages(), 1)
	assert.Contains(t, h.source.Messages.GetMessages()[0], "directories were not listed")
	assert.Less(t, len(h.scanner.active), len(paths))

	for range 8 {
		h.scan()
	}
	assert.Len(t, h.scanner.active, len(paths), "every directory was listed")
	assert.Len(t, h.source.Messages.GetMessages(), 1, "there are more entries than one scan lists: the message stays")
	assert.Len(t, h.out.waitLines(t, len(paths)), len(paths))
}

// TestScanBudgetRotatesAndKeepsStartPositionEnd: the directories that a scan could
// not list when the source started keep start_position when they are listed,
// and a directory that the budget always cut first is not left out for ever.
func TestScanBudgetRotatesAndKeepsStartPositionEnd(t *testing.T) {
	h := newHarness(t, withPath("d*/f*.log"), withStartPosition("end"), withLauncher(func(l *Launcher) {
		l.listEntries = 50
		// The fake gives each new file a creation time at least a microsecond
		// after the previous one's, which runs ahead of the clock for a
		// loop that creates 200: the source starts after all of them.
		l.wallNow = func() time.Time { return time.Now().Add(time.Second) }
	}))
	manyDirectories(h, 10, 20)
	for range 9 {
		h.scan()
	}
	assert.Len(t, h.scanner.active, 200)
	assert.Empty(t, h.out.lines(), "no history of the files that were there is shipped")

	h.share.Write("d9/f99.log", []byte("fresh\n"))
	h.share.Append("d5/f3.log", []byte("tail\n"))
	for range 9 {
		h.scan()
	}
	assert.ElementsMatch(t, []string{"fresh", "tail"}, h.out.waitLines(t, 2))
}

// TestOversizedDirectoriesAreChargedToTheScanBudget: the client reads as much of
// a directory as it lists before it refuses it for its size, so a refused
// listing counts for the scan's budget like a listing. A pattern that matches
// many directories that are too large lists them in turn, one a scan, not all of
// them in one scan that blocks the polling of the source's tailers.
func TestOversizedDirectoriesAreChargedToTheScanBudget(t *testing.T) {
	h := newHarness(t, withPath("arch/*/app.log"))
	var dirs []string
	for i := 1; i <= 5; i++ {
		dir := fmt.Sprintf("arch/d%d", i)
		dirs = append(dirs, dir)
		h.share.Mkdir(dir)
		h.share.Write(dir+"/app.log", []byte(lines(i, i)))
		h.share.FailNextPath(fake.OpListDir, dir, tooLarge(dir, 30)...)
	}
	listed := h.listings()
	tried := make(map[string]bool)
	for scan := 1; scan <= 5; scan++ {
		h.scan()
		refused := 0
		for dir, n := range listed() {
			if strings.HasPrefix(dir, "arch/d") {
				refused += n
				tried[dir] = true
			}
		}
		assert.Equal(t, 1, refused, "scan %d lists one directory that is too large", scan)
		if scan == 1 {
			assert.Contains(t, h.source.Messages.GetMessages()[0], "4 directories were not listed in the last scan")
		}
	}
	assert.Len(t, tried, len(dirs), "every directory is tried in turn")
}

// TestFileThatMovedToADirectoryTheBudgetLeftOutIsNotProvedGone: a file that
// rotated away from its path while the Agent was down, to a directory the scan's
// budget leaves for the next scan, is not reported missed and read again from
// its beginning because the directories that were listed do not hold it: the
// next scan lists the directory, and the file resumes where it was.
func TestFileThatMovedToADirectoryTheBudgetLeftOutIsNotProvedGone(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t, withPath("logs/*/app.log"))
	for _, dir := range []string{"logs/d1", "logs/d2", "logs/d3"} {
		h.share.Mkdir(dir)
	}
	h.share.Write("logs/d1/app.log", []byte(lines(1, 3)))
	h.scan()
	h.out.waitLines(t, 3)
	h.commitOffsets()

	// While the Agent is down the file is archived into d2 and d1 gets a new one.
	h.share.Append("logs/d1/app.log", []byte(lines(4, 4)))
	require.NoError(t, h.share.Rename("logs/d1/app.log", "logs/d2/app.log"))
	h.share.Write("logs/d1/app.log", []byte(lines(5, 5)))

	// The restarted scanner's first scan lists logs and d1, and leaves d2 and d3
	// for the next one.
	restarted := h.restart()
	restarted.launcher.listEntries = 4
	restarted.scan()
	require.Contains(t, restarted.scanner.cursors, 2, "d2 and d3 were left out")
	assert.Empty(t, metrics.MissedBytesSnapshot(), "the file is not proved gone: d2 was not listed")

	restarted.launcher.listEntries = 0
	restarted.scan()
	restarted.scan()
	assert.ElementsMatch(t, want(1, 5), restarted.finish(), "each line once")
	assert.Empty(t, metrics.MissedBytesSnapshot())
}
