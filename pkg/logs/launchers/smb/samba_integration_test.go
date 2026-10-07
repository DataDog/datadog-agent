// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && smb && !goexperiment.systemcrypto && !goexperiment.boringcrypto && !requirefips

package smb

// TestSambaIntegration runs the SMB log source against a real Samba server in
// a Docker container (testdata/samba): the real SMB client, tailers, scanner
// and launcher, with only the pipeline and the auditor mocked. Each scenario
// writes numbered lines and checks that every line reaches the pipeline
// exactly once, except the lines the scenario expects to lose, which it names
// and compares with the bytes the launcher reports missed.
//
// The writer (smbWriter) is another SMB client with its own session, as the
// applications whose logs the source reads are, rather than a process writing
// to a bind mount: Samba then enforces share modes between the writer's
// handles and the Agent's opens (the writer's renames only succeed because
// the Agent's opens allow deletes), and the sizes and FileIds the Agent sees
// do not go through Docker Desktop's file sharing, which caches attributes.
//
// It needs Docker and is skipped unless INTEGRATION is set, so the unit test
// jobs, which have no Docker, only build it. With INTEGRATION set, it fails
// when no Docker daemon running Linux containers answers. The first run
// builds the image, which pulls alpine from Docker Hub and Samba from the
// Alpine repository.
// Run it with one of:
//
//	dda inv integration-tests --only="Logs SMB" --timeout=20m
//
//	dda inv test --targets=./pkg/logs/launchers/smb \
//	  --test-args="-test.run=TestSambaIntegration -test.v" \
//	  --bazel-args="--test_env=INTEGRATION=1 --test_env=PATH --test_env=HOME --test_env=DOCKER_HOST \
//	    --test_timeout=1200 --cache_test_results=no --strategy=TestRunner=local"
//
// It takes about five minutes, plus the first build of the image.

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/smb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// itLineLen is the length of every line, newline included.
	itLineLen = 128
	// itWriteEvery paces the writer: about 150 lines per second.
	itWriteEvery = 5 * time.Millisecond
	// itCloseTimeout is the default logs_config.close_timeout.
	itCloseTimeout = time.Minute
	// itSettle is how long a scenario keeps reading once every expected line
	// arrived, so that duplicates and late reads show up: over two polls.
	itSettle = 3 * time.Second
	// itDeliverTimeout bounds the wait for the expected lines.
	itDeliverTimeout = 90 * time.Second
	// The client's timeouts, shorter than the defaults (10s and 30s) to keep
	// the outage scenarios short. Only their values change.
	itDialTimeout = 5 * time.Second
	itOpTimeout   = 5 * time.Second
	// itSource is the source of every scenario's log source; the service
	// names the scenario, so missed bytes are counted per scenario.
	itSource = "smb-it"
)

// itLinePattern matches a line of a scenario, without its newline.
var itLinePattern = regexp.MustCompile(`^([a-z0-9-]+) seq=(\d{6}) x+$`)

func TestSambaIntegration(t *testing.T) {
	requireSambaIntegration(t)
	configmock.New(t)
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	logs := captureAgentLogs(t)
	env := startSamba(t)
	report := &itReport{}
	t.Cleanup(func() {
		t.Logf("Samba integration report:\n%s", env.redact(report.String()))
		agentLogs := logs.String()
		assert.Zero(t, env.secretsIn(agentLogs), "a password reached the Agent logs")
		if t.Failed() {
			t.Logf("Agent logs (passwords redacted, last 64 KiB):\n%s", env.redact(tail(agentLogs, 64<<10)))
		}
	})

	// The scenarios share the server, and some restart, pause it or change
	// its password: they run one after the other.
	for _, sc := range []struct {
		name string
		run  func(*testing.T, *sambaEnv, *itReport)
	}{
		{"rename rotation", testRenameRotation},
		{"fixed window rotation matched by the pattern", testFixedWindowRotation},
		{"fast fixed window rotation matched by the pattern", testFastFixedWindowRotation},
		{"copytruncate", testCopyTruncate},
		{"delete and recreate", testDeleteAndRecreate},
		{"compress on rotate", testCompressOnRotate},
		{"compress on rotate without a slow read", testCompressOnRotateUnpaced},
		{"late append to the rotated file", testLateAppend},
		{"agent restart", testAgentRestart},
		{"agent crash", testAgentCrash},
		{"server restart", testServerRestart},
		{"network stall", testNetworkStall},
		{"password change", testPasswordChange},
	} {
		if !t.Run(sc.name, func(t *testing.T) { sc.run(t, env, report) }) {
			t.Logf("scenario %q failed; the following scenarios still run", sc.name)
		}
	}
}

// testRenameRotation rotates like Log4j2's RollingFileAppender: close the
// file, rename it to a dated name the pattern does not match, create a new
// one. Each rename comes right after a burst of writes, so the rotated file
// holds lines the reader has not read yet: they are read from the rotated
// file, found by FileId.
func testRenameRotation(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "rename")
	file := s.dir + "/app.log"
	s.startSource(s.newSource(env.currentPassword(), file))
	s.startFile(file)
	for r := 1; r <= 4; r++ {
		require.NoError(t, s.writeLines(file, 200, itWriteEvery, nil))
		require.NoError(t, s.writer.rename(file, fmt.Sprintf("%s/app-2026-10-06-%d.log", s.dir, r), false))
		require.NoError(t, s.writer.create(file))
	}
	require.NoError(t, s.writeLines(file, 200, itWriteEvery, nil))

	s.verify(nil, nil)
	assert.Zero(t, s.missedBytes(), "no bytes are reported missed")
	s.reportf("%d lines over 5 files and 4 rotations: each delivered once", s.writtenCount())
}

// testFixedWindowRotation rotates like Log4j2's DefaultRolloverStrategy with
// max=3 (app.log -> app.log.1 -> app.log.2 -> app.log.3, then deleted), with
// a pattern that also matches the rotated names: a file must not be read
// again under its next name. Each rollover comes right after a burst of
// writes, so the rotated file holds lines not read yet. The writer then waits
// until the source tails every file of the window again under its new name,
// from where its drain ended, as with rollovers minutes or hours apart:
// until then, where a drain ended is only kept by path, and another rollover
// loses it (see testFastFixedWindowRotation). That takes up to about 4s after
// a rollover: up to a poll interval to see it, the drain's polls until two in
// a row find nothing new, and one more scan to start the tailer that resumes
// the drained file. Samba can give the next app.log the inode, and so the
// FileId, of the app.log.3 it just deleted.
func testFixedWindowRotation(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "window")
	file := s.dir + "/app.log"
	s.startSource(s.newSource(env.currentPassword(), file+"*"))
	s.startFile(file)
	settled := s.rollFixedWindow(file, 5, true)

	s.verify(nil, nil)
	assert.Zero(t, s.missedBytes(), "no bytes are reported missed")
	s.reportf("%d lines over 5 rollovers of a 3-file window matched by %s*, each once the previous one's files were tailed again (%s after it): each delivered once",
		s.writtenCount(), file, durationRange(settled))
}

// testFastFixedWindowRotation rolls the same window over every 1.2 seconds,
// faster than a rotated file's drain, as a size-based policy does under heavy
// logging. It checks that nothing is lost and reports the duplicates: where a
// drain ended is kept by path (scanner.handoffs) and is lost when the file is
// renamed again before that path's tailer starts, so the file's next path
// reads it again from offset 0.
func testFastFixedWindowRotation(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "window-fast")
	file := s.dir + "/app.log"
	s.startSource(s.newSource(env.currentPassword(), file+"*"))
	s.startFile(file)
	s.rollFixedWindow(file, 5, false)

	_, dups := s.verify(nil, func(*itLine) bool { return true })
	s.reportf("%d lines over 5 rollovers 1.2s apart of a 3-file window matched by %s*: no loss, %d lines delivered twice (%s)%s",
		s.writtenCount(), file, len(dups), seqRanges(dups),
		knownIssue(len(dups) > 0, "a drained file renamed again before its path's tailer starts is read again from offset 0"))
}

// rollFixedWindow writes 200 lines to file and rolls the window over,
// rollovers times, then writes 200 more lines. With settle, it waits after
// each rollover until the source tails every file of the window again (see
// waitWindowTailed), and returns how long each wait took.
func (s *itScenario) rollFixedWindow(file string, rollovers int, settle bool) (settled []time.Duration) {
	s.t.Helper()
	window := []string{file}
	for range rollovers {
		require.NoError(s.t, s.writeLines(file, 200, itWriteEvery, nil))
		if len(window) == 4 {
			require.NoError(s.t, s.writer.remove(window[3]))
			window = window[:3]
		}
		for i := len(window) - 1; i >= 1; i-- {
			require.NoError(s.t, s.writer.rename(window[i], file+"."+strconv.Itoa(i+1), false))
		}
		require.NoError(s.t, s.writer.rename(file, file+".1", false))
		require.NoError(s.t, s.writer.create(file))
		window = append(window, file+"."+strconv.Itoa(len(window)))
		if settle {
			settled = append(settled, s.waitWindowTailed(window))
		}
	}
	require.NoError(s.t, s.writeLines(file, 200, itWriteEvery, nil))
	return settled
}

// waitWindowTailed waits until each of paths is read by an active tailer of
// the file the share lists under that name, and no rotated file is being
// drained anymore: the scanner saw the rollover, every drain ended, and each
// drained file is tailed again under its new name. It returns how long that
// took. The FileIds tell these tailers from those that read the same names
// before the rollover, which the scanner may not have seen yet.
func (s *itScenario) waitWindowTailed(paths []string) time.Duration {
	s.t.Helper()
	start := time.Now()
	entries, err := s.env.listDir(s.dir)
	require.NoError(s.t, err)
	listed := make(map[string]uint64, len(entries))
	for _, e := range entries {
		listed[join(s.dir, e.Name)] = e.FileID
	}
	pending := func() []string {
		var waiting []string
		tailed := map[string]uint64{} // FileId of each active tailer, by identifier
		for _, t := range s.launcher.tailers.All() {
			if t.GetID() != t.Identifier() {
				waiting = append(waiting, "drain "+t.GetID())
				continue
			}
			tailed[t.Identifier()] = t.FileID()
		}
		for _, p := range paths {
			if id := tailed[tailer.Identifier(s.env.host, sambaShare, p)]; id == 0 || id != listed[p] {
				waiting = append(waiting, fmt.Sprintf("%s (FileId %d listed, %d tailed)", p, listed[p], id))
			}
		}
		sort.Strings(waiting)
		return waiting
	}
	s.waitFor(func() bool { return len(pending()) == 0 }, itDeliverTimeout, func() string {
		return "the files of the window are not all tailed again after the rollover: " + strings.Join(pending(), ", ")
	})
	return time.Since(start)
}

// durationRange prints the smallest and the largest of ds: "3.1s-3.9s".
func durationRange(ds []time.Duration) string {
	if len(ds) == 0 {
		return "none"
	}
	lo, hi := slices.Min(ds), slices.Max(ds)
	return lo.Round(100*time.Millisecond).String() + "-" + hi.Round(100*time.Millisecond).String()
}

// knownIssue flags a reported outcome that a product change should fix.
func knownIssue(happened bool, issue string) string {
	if !happened {
		return ""
	}
	return " [KNOWN ISSUE: " + issue + "]"
}

// testCopyTruncate rotates like logrotate's copytruncate. The writer keeps
// writing between the copy and the truncation: those lines are only in the
// truncated part of the file, so they may be lost, but nothing may be
// delivered twice. Each rotation waits for the reader to catch up first, as
// a rotation hours apart would: the lines written before the copy are then
// all readable.
//
// The reader only sees a truncation while the file is shorter than what it
// read: a file that grows past that between two polls looks like a file that
// grew. So after each truncation the writer writes fewer lines than the
// reader read, and waits until they are delivered before writing more: the
// truncation is seen whatever the poll's phase and the write speed.
func testCopyTruncate(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "copytruncate")
	file := s.dir + "/app.log"
	s.startSource(s.newSource(env.currentPassword(), file))
	s.startFile(file)
	require.NoError(t, s.writeLines(file, 200, itWriteEvery, nil))
	for r := 1; r <= 3; r++ {
		s.waitDelivered(s.lastSeq())
		require.NoError(t, s.writer.copyFile(file, fmt.Sprintf("%s.%d", file, r)))
		require.NoError(t, s.writeLines(file, 20, 0, func(l *itLine) { l.mayLose = true }))
		require.NoError(t, s.writer.truncate(file))
		require.NoError(t, s.writeLines(file, 50, itWriteEvery, nil))
		s.waitDelivered(s.lastSeq())
		require.NoError(t, s.writeLines(file, 150, itWriteEvery, nil))
	}

	lost, _ := s.verify(func(l *itLine) bool { return l.mayLose }, nil)
	s.assertMissedBytesBound(lost)
	window := s.count(func(l *itLine) bool { return l.mayLose })
	s.reportf("%d lines, 3 copytruncates: %d of the %d lines written between copy and truncate lost (%s), none duplicated; %d bytes reported missed",
		s.writtenCount(), len(lost), window, seqRanges(lost), s.missedBytes())
}

// testDeleteAndRecreate deletes the file and creates a new one while the
// reader is about to read lines it already saw in a listing: those lines are
// lost, and the launcher must report exactly their bytes as missed.
//
// The read is held by a gate in the client (see readGate) until the file is
// replaced, as a slow read would be: without it, the outcome depends on when
// the writer acts relative to the poll.
func testDeleteAndRecreate(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "recreate")
	file := s.dir + "/app.log"
	s.startSource(s.newSource(env.currentPassword(), file))
	s.startFile(file)
	require.NoError(t, s.writeLines(file, 200, itWriteEvery, nil))
	s.waitDelivered(s.lastSeq())
	oldID := env.fileID(s.dir, "app.log")

	s.gate.arm(file, s.writer.open[file].size+50*itLineLen)
	require.NoError(t, s.writeBatch(file, 50, func(l *itLine) { l.doomed = true }))
	s.gate.waitHit(t)
	require.NoError(t, s.writer.remove(file))
	require.NoError(t, s.writer.create(file))
	require.NoError(t, s.writeBatch(file, 10, nil))
	newID := env.fileID(s.dir, "app.log")
	s.gate.release()
	require.NoError(t, s.writeLines(file, 200, itWriteEvery, nil))

	lost, _ := s.verify(func(l *itLine) bool { return l.doomed }, nil)
	doomed := s.count(func(l *itLine) bool { return l.doomed })
	assert.Len(t, lost, doomed, "the lines of the deleted file were lost")
	lostBytes := int64(len(lost) * itLineLen)
	assert.Equal(t, lostBytes, s.missedBytes(), "the bytes reported missed are the bytes lost")
	s.reportf("FileId %d, recreated as FileId %d (reused: %t): %d lines (%d bytes) lost, %d bytes reported missed",
		oldID, newID, oldID == newID, len(lost), lostBytes, s.missedBytes())
}

// testCompressOnRotate renames the file, creates a new one, then compresses
// the rotated file and deletes it, while the reader is about to read lines it
// saw in a listing. Those lines are lost (they only survive in the .gz, which
// the pattern does not match), and the launcher must report exactly their
// bytes as missed.
func testCompressOnRotate(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "compress")
	file := s.dir + "/app.log"
	s.startSource(s.newSource(env.currentPassword(), file))
	s.startFile(file)
	require.NoError(t, s.writeLines(file, 200, itWriteEvery, nil))
	s.waitDelivered(s.lastSeq())

	s.gate.arm(file, s.writer.open[file].size+50*itLineLen)
	require.NoError(t, s.writeBatch(file, 50, func(l *itLine) { l.doomed = true }))
	s.gate.waitHit(t)
	require.NoError(t, s.writer.rename(file, file+".1", false))
	require.NoError(t, s.writer.create(file))
	require.NoError(t, s.writeBatch(file, 10, nil))
	require.NoError(t, s.writer.gzipFile(file+".1", file+".1.gz"))
	require.NoError(t, s.writer.remove(file+".1"))
	s.gate.release()
	require.NoError(t, s.writeLines(file, 200, itWriteEvery, nil))

	lost, _ := s.verify(func(l *itLine) bool { return l.doomed }, nil)
	doomed := s.count(func(l *itLine) bool { return l.doomed })
	assert.Len(t, lost, doomed, "the lines of the compressed file were lost")
	lostBytes := int64(len(lost) * itLineLen)
	assert.Equal(t, lostBytes, s.missedBytes(), "the bytes reported missed are the bytes lost")

	// The lost lines were written: they are in the archive.
	compressed, err := s.writer.readFile(file + ".1.gz")
	require.NoError(t, err)
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)
	archived, err := io.ReadAll(zr)
	require.NoError(t, err)
	for _, l := range lost {
		assert.Contains(t, string(archived), fmt.Sprintf(" seq=%06d ", l.seq))
	}
	s.reportf("%d lines (%d bytes) lost to the archive, %d bytes reported missed", len(lost), lostBytes, s.missedBytes())
}

// testCompressOnRotateUnpaced rotates and compresses right after bursts of
// writes, with nothing slowing the reader down. The lines of a burst the
// reader had not read when its file was compressed and deleted can be lost,
// without being reported when the reader never saw them in a listing. The
// test checks that only those are lost: the end of a compressed file, after
// the last line delivered from it (the reader reads a file in order). Every
// line written to the file that is never compressed must arrive, nothing may
// be delivered twice, and the launcher must never report more bytes missed
// than were lost.
func testCompressOnRotateUnpaced(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "compress-unpaced")
	file := s.dir + "/app.log"
	s.startSource(s.newSource(env.currentPassword(), file))
	s.startFile(file)
	type burst struct{ from, to int } // seqs of the lines of a compressed file
	var bursts []burst
	for r := 1; r <= 3; r++ {
		from := s.lastSeq() + 1
		require.NoError(t, s.writeLines(file, 200, itWriteEvery, func(l *itLine) { l.mayLose = true }))
		bursts = append(bursts, burst{from, s.lastSeq()})
		require.NoError(t, s.writer.rename(file, file+".1", false))
		require.NoError(t, s.writer.create(file))
		require.NoError(t, s.writer.gzipFile(file+".1", fmt.Sprintf("%s.%d.gz", file, r)))
		require.NoError(t, s.writer.remove(file+".1"))
	}
	require.NoError(t, s.writeLines(file, 200, itWriteEvery, nil))

	lost, _ := s.verify(func(l *itLine) bool { return l.mayLose }, nil)
	for _, b := range bursts {
		firstLost := 0
		for seq := b.from; seq <= b.to; seq++ {
			delivered := s.coll.deliveries(seq) > 0
			if !delivered && firstLost == 0 {
				firstLost = seq
			}
			if delivered && firstLost != 0 {
				assert.Failf(t, "a line was lost from the middle of a compressed file",
					"line %d was lost but line %d, later in the same file (lines %d-%d), was delivered", firstLost, seq, b.from, b.to)
				break
			}
		}
	}
	s.assertMissedBytesBound(lost)
	lostBytes := int64(len(lost) * itLineLen)
	s.reportf("%d lines, 3 rotations compressed at once: %d lines (%d bytes) lost (%s), all at the end of their file; %d bytes reported missed",
		s.writtenCount(), len(lost), lostBytes, seqRanges(lost), s.missedBytes())
}

// testLateAppend renames the file while the writer keeps its handle, then
// writes one more line through that handle 0.5s, 1.5s or 3s later. The
// rotated file is read until two polls in a row find nothing new: the 0.5s
// line must survive; the others are reported.
func testLateAppend(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "late")
	file := s.dir + "/app.log"
	s.startSource(s.newSource(env.currentPassword(), file))
	s.startFile(file)
	delays := []time.Duration{500 * time.Millisecond, 1500 * time.Millisecond, 3 * time.Second}
	for i, delay := range delays {
		require.NoError(t, s.writeLines(file, 100, itWriteEvery, nil))
		rotated := fmt.Sprintf("%s.%d", file, i+1)
		require.NoError(t, s.writer.rename(file, rotated, true))
		renamed := time.Now()
		require.NoError(t, s.writer.create(file))
		require.NoError(t, s.writeLines(file, 1, 0, nil))
		time.Sleep(time.Until(renamed.Add(delay)))
		require.NoError(t, s.writeLines(rotated, 1, 0, func(l *itLine) { l.late = delay }))
		require.NoError(t, s.writer.closeFile(rotated))
		require.NoError(t, s.writeLines(file, 50, itWriteEvery, nil))
		time.Sleep(5 * time.Second) // the rotated file's drain ends
	}

	lost, _ := s.verify(func(l *itLine) bool { return l.late > delays[0] }, nil)
	s.assertMissedBytesBound(lost)
	missing := map[time.Duration]bool{}
	for _, l := range lost {
		missing[l.late] = true
	}
	var outcomes []string
	for _, delay := range delays {
		outcome := "delivered"
		if missing[delay] {
			outcome = "lost"
		}
		outcomes = append(outcomes, fmt.Sprintf("%s after the rename: %s", delay, outcome))
	}
	s.reportf("line appended to the rotated file %s; %d bytes reported missed", strings.Join(outcomes, ", "), s.missedBytes())
}

// testAgentRestart stops the launcher in the middle of a file, while the
// writer keeps writing, and starts a new one with the same registry, as an
// Agent restart does. The auditor commits each line's offset as it is
// delivered, so the new launcher resumes exactly after the last delivered
// line.
func testAgentRestart(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "restart")
	file := s.dir + "/app.log"
	source := s.newSource(env.currentPassword(), file)
	s.startSource(source)
	s.startFile(file)
	done := s.writeInBackground(file, 400, itWriteEvery)
	s.waitWritten(200)
	s.launcher.Stop()
	stoppedAt := s.coll.deliveredCount()
	s.startLauncher(s.registry)
	require.NoError(t, done())

	_, dups := s.verify(nil, nil)
	s.reportf("stopped after %d of %d lines and restarted with the committed offsets: %d duplicates, no loss",
		stoppedAt, s.writtenCount(), len(dups))
}

// testAgentCrash restarts the launcher from a registry saved before its last
// lines were delivered, as after a crash: the lines delivered since are read
// again, and nothing else is.
func testAgentCrash(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "crash")
	file := s.dir + "/app.log"
	s.startSource(s.newSource(env.currentPassword(), file))
	s.startFile(file)
	require.NoError(t, s.writeLines(file, 200, itWriteEvery, nil))
	s.waitDelivered(s.lastSeq())
	saved := s.coll.saveRegistry()
	savedAt := s.lastSeq()
	require.NoError(t, s.writeLines(file, 100, itWriteEvery, nil))
	s.waitDelivered(s.lastSeq())
	s.launcher.Stop()
	uncommitted := s.lastSeq() - savedAt

	s.coll.useRegistry(saved)
	s.startLauncher(saved)
	require.NoError(t, s.writeLines(file, 100, itWriteEvery, nil))

	_, dups := s.verify(nil, func(l *itLine) bool { return l.seq > savedAt })
	assert.Len(t, dups, uncommitted, "the lines delivered after the registry was saved are read again")
	s.reportf("restarted from a registry %d lines old: %d duplicates (the uncommitted tail is %d lines), no loss",
		uncommitted, len(dups), uncommitted)
}

// testServerRestart restarts the Samba container in the middle of a file.
// The client reconnects and the tailer resumes at its offset.
func testServerRestart(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "server-restart")
	file := s.dir + "/app.log"
	source := s.newSource(env.currentPassword(), file)
	s.startSource(source)
	s.startFile(file)
	done := s.writeInBackground(file, 600, itWriteEvery)
	s.waitWritten(200)
	idBefore := env.fileID(s.dir, "app.log")
	dialsBefore := s.dials.Load()
	restartedAt := time.Now()
	env.restart()
	restartTook := time.Since(restartedAt)
	require.NoError(t, done())

	s.verify(nil, nil)
	assert.Greater(t, s.dials.Load(), dialsBefore, "the client reconnected")
	s.waitFor(func() bool { return source.Status().IsSuccess() }, 30*time.Second, func() string { return "status: " + source.Status().GetError() })
	idAfter := env.fileID(s.dir, "app.log")
	assert.Equal(t, idBefore, idAfter, "the FileId survives a server restart")
	s.reportf("server restarted in %s mid-file (FileId %d before, %d after; %d dials): %d lines each delivered once",
		restartTook.Round(100*time.Millisecond), idBefore, idAfter, s.dials.Load(), s.writtenCount())
}

// testNetworkStall freezes the server longer than the client's operation
// timeout, as a network partition would: the source reports an error, then
// recovers without losing or duplicating a line.
func testNetworkStall(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "stall")
	file := s.dir + "/app.log"
	source := s.newSource(env.currentPassword(), file)
	s.startSource(source)
	s.startFile(file)
	require.NoError(t, s.writeLines(file, 200, itWriteEvery, nil))
	s.waitDelivered(s.lastSeq())
	// Written just before the stall: the reader may not have read them yet.
	require.NoError(t, s.writeBatch(file, 20, nil))

	env.pause()
	pausedAt := time.Now()
	paused := true
	defer func() {
		if paused {
			env.unpause()
		}
	}()
	s.waitFor(source.Status().IsError, itOpTimeout+20*time.Second, func() string { return "no error status during the stall" })
	errorAfter := time.Since(pausedAt)
	stallStatus := env.redact(source.Status().GetError())
	assert.Contains(t, stallStatus, "cannot reach smb://")
	assert.Zero(t, env.secretsIn(source.Status().GetError()))
	time.Sleep(time.Until(pausedAt.Add(itOpTimeout + 5*time.Second)))
	env.unpause()
	paused = false
	unpausedAt := time.Now()

	require.NoError(t, s.writeLines(file, 200, itWriteEvery, nil))
	s.waitFor(source.Status().IsSuccess, itDeliverTimeout, func() string { return "status: " + source.Status().GetError() })
	recoveredAfter := time.Since(unpausedAt)
	s.verify(nil, nil)
	s.reportf("server frozen %s (operation timeout %s): error status after %s (%q), recovered %s after the thaw; %d lines each delivered once",
		unpausedAt.Sub(pausedAt).Round(100*time.Millisecond), itOpTimeout, errorAfter.Round(100*time.Millisecond),
		stallStatus, recoveredAfter.Round(100*time.Millisecond), s.writtenCount())
}

// testPasswordChange changes the account's password on the server while the
// source tails a file. The source reports an authentication error, without
// the password, and recovers once the server accepts its password again, or
// once its configuration is updated with the new password (the launcher then
// replaces the source, as after a secret refresh).
func testPasswordChange(t *testing.T, env *sambaEnv, report *itReport) {
	s := newITScenario(t, env, report, "password")
	file := s.dir + "/app.log"
	original := env.currentPassword()
	entry := func(c *config.LogsConfig) {
		c.IntegrationSource = "file:/etc/datadog-agent/conf.d/smb_it.d/conf.yaml"
		c.IntegrationSourceIndex = 0
	}
	source := s.newSource(original, file, entry)
	s.startSource(source)
	s.startFile(file)
	require.NoError(t, s.writeLines(file, 100, itWriteEvery, nil))
	s.waitDelivered(s.lastSeq())

	var statuses []string
	authError := func(source *sources.LogSource) {
		t.Helper()
		s.waitFor(func() bool {
			return source.Status().IsError() && strings.Contains(source.Status().GetError(), "rejected the credentials")
		}, time.Minute, func() string { return "status: " + source.Status().GetError() })
		statuses = append(statuses, source.Status().GetError())
	}

	// 1. The server's password changes; the Agent keeps the old one.
	env.setPassword(newPassword(t))
	env.restart() // drops the established sessions
	require.NoError(t, s.writeLines(file, 50, itWriteEvery, nil))
	authError(source)
	errorStatus := env.redact(statuses[0])

	// 2. The server accepts the old password again: the source recovers once
	// its authentication backoff (30s) is over.
	env.setPassword(original)
	restoredAt := time.Now()
	s.waitFor(source.Status().IsSuccess, time.Minute, func() string { return "status: " + source.Status().GetError() })
	recoveredAfter := time.Since(restoredAt)
	s.waitDelivered(s.lastSeq())

	// 3. The server's password changes again, and so does the source's
	// configuration.
	rotated := newPassword(t)
	env.setPassword(rotated)
	env.restart()
	require.NoError(t, s.writeLines(file, 50, itWriteEvery, nil))
	authError(source)
	refreshed := s.newSource(rotated, file, entry)
	updatedAt := time.Now()
	s.sources.AddSource(refreshed)
	s.waitFor(refreshed.Status().IsSuccess, 30*time.Second, func() string { return "status: " + refreshed.Status().GetError() })
	updatedRecovery := time.Since(updatedAt)
	assert.True(t, source.IsHiddenFromStatus(), "agent status no longer lists the replaced source")
	require.NoError(t, s.writeLines(file, 50, itWriteEvery, nil))

	s.verify(nil, nil)
	for _, src := range []*sources.LogSource{source, refreshed} {
		publicJSON, err := src.PublicJSON()
		require.NoError(t, err)
		statuses = append(statuses, src.Status().GetError(), src.Dump(true), string(publicJSON))
	}
	for _, status := range statuses {
		assert.Zero(t, env.secretsIn(status), "a password reached the source status: %s", env.redact(status))
	}
	s.reportf("auth error status %q; recovered %s after the server took the old password back, %s after the source got the new one; %d lines each delivered once",
		errorStatus, recoveredAfter.Round(100*time.Millisecond), updatedRecovery.Round(100*time.Millisecond), s.writtenCount())
}

// --- scenario harness -----------------------------------------------------

// itLine is a line a scenario wrote.
type itLine struct {
	seq     int
	file    string
	mayLose bool          // written between a copytruncate's copy and its truncation, or to a file compressed and deleted at once
	doomed  bool          // destroyed while the reader's read was held (readGate)
	late    time.Duration // written to the rotated file this long after the rename
}

// itScenario is one scenario: a directory of the share, a log source tailing
// it through a real launcher, and a writer.
type itScenario struct {
	t        *testing.T
	env      *sambaEnv
	report   *itReport
	name     string
	dir      string
	started  time.Time
	provider pipeline.Provider
	registry *auditorMock.Registry
	sources  *sources.LogSources
	coll     *itCollector
	writer   *smbWriter
	launcher *Launcher
	gate     readGate
	dials    atomic.Int64

	mu      sync.Mutex
	written []*itLine // by seq-1
}

func newITScenario(t *testing.T, env *sambaEnv, report *itReport, name string) *itScenario {
	t.Helper()
	provider := mock.NewMockProvider()
	registry := auditorMock.NewMockRegistry()
	s := &itScenario{
		t:        t,
		env:      env,
		report:   report,
		name:     name,
		dir:      name,
		started:  time.Now(),
		provider: provider,
		registry: registry,
		sources:  sources.NewLogSources(),
		writer:   newSMBWriter(env),
	}
	s.coll = newITCollector(name, provider.NextPipelineChan(), registry)
	t.Cleanup(s.coll.stop)
	t.Cleanup(s.writer.close)
	require.NoError(t, s.writer.mkdirAll(s.dir))
	return s
}

// newSource returns a source tailing pattern with password.
func (s *itScenario) newSource(password, pattern string, opts ...func(*config.LogsConfig)) *sources.LogSource {
	cfg := &config.LogsConfig{
		Type:        config.SMBType,
		Path:        pattern,
		Source:      itSource,
		Service:     s.service(),
		TailingMode: "beginning",
		SMB: &config.SMBConfig{
			Host:     s.env.host,
			Share:    sambaShare,
			Username: sambaUser,
			Password: password,
			Port:     s.env.port,
		},
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return sources.NewLogSource("smb-it-"+s.name, cfg)
}

func (s *itScenario) service() string {
	return itSource + "-" + s.name
}

// startSource starts a launcher and adds source to it.
func (s *itScenario) startSource(source *sources.LogSource) {
	s.startLauncher(s.registry)
	s.sources.AddSource(source)
	s.waitFor(func() bool { return !source.Status().IsPending() }, 30*time.Second, func() string { return "the source never scanned" })
	require.True(s.t, source.Status().IsSuccess(), "source status: %s", s.env.redact(source.Status().GetError()))
}

// startLauncher starts a launcher reading the scenario's sources with
// registry, which the collector commits offsets to.
func (s *itScenario) startLauncher(registry *auditorMock.Registry) {
	l := NewLauncher(itCloseTimeout)
	l.dial = s.dial
	l.Start(s.sources, s.provider, registry, tailers.NewTailerTracker())
	s.launcher = l
	s.t.Cleanup(l.Stop)
}

// dial is the launcher's dial function: the real client, with the scenario's
// timeouts, counted, and with reads that the gate can hold.
func (s *itScenario) dial(ctx context.Context, cfg client.Config) (client.Client, error) {
	s.dials.Add(1)
	cfg.DialTimeout, cfg.OpTimeout = itDialTimeout, itOpTimeout
	c, err := client.Dial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &gatedClient{Client: c, gate: &s.gate}, nil
}

func (s *itScenario) line(seq int) []byte {
	head := fmt.Sprintf("%s seq=%06d ", s.name, seq)
	return []byte(head + strings.Repeat("x", itLineLen-len(head)-1) + "\n")
}

func (s *itScenario) next(file string, mark func(*itLine)) *itLine {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := &itLine{seq: len(s.written) + 1, file: file}
	if mark != nil {
		mark(l)
	}
	s.written = append(s.written, l)
	return l
}

// writeLines writes n lines to file, one SMB WRITE per line, every interval.
func (s *itScenario) writeLines(file string, n int, every time.Duration, mark func(*itLine)) error {
	for i := range n {
		if i > 0 && every > 0 {
			time.Sleep(every)
		}
		l := s.next(file, mark)
		if err := s.writer.write(file, s.line(l.seq)); err != nil {
			return err
		}
	}
	return nil
}

// writeBatch writes n lines to file in a single SMB WRITE.
func (s *itScenario) writeBatch(file string, n int, mark func(*itLine)) error {
	var batch []byte
	for range n {
		batch = append(batch, s.line(s.next(file, mark).seq)...)
	}
	return s.writer.write(file, batch)
}

// startFile creates file, writes a first line and waits until it is
// delivered: from then on the source tails the file, so its rotation cannot
// go unnoticed (a file created and rotated between two polls is never seen).
func (s *itScenario) startFile(file string) {
	s.t.Helper()
	require.NoError(s.t, s.writer.create(file))
	require.NoError(s.t, s.writeLines(file, 1, 0, nil))
	s.waitDelivered(s.lastSeq())
}

// writeInBackground writes n lines to file from another goroutine and
// returns a function that waits for it.
func (s *itScenario) writeInBackground(file string, n int, every time.Duration) func() error {
	errc := make(chan error, 1)
	go func() { errc <- s.writeLines(file, n, every, nil) }()
	return func() error {
		select {
		case err := <-errc:
			return err
		case <-time.After(writerRetryFor + time.Minute):
			return errors.New("the writer is stuck")
		}
	}
}

func (s *itScenario) lastSeq() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.written)
}

func (s *itScenario) writtenCount() int { return s.lastSeq() }

func (s *itScenario) lines() []*itLine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*itLine(nil), s.written...)
}

func (s *itScenario) count(pred func(*itLine) bool) int {
	n := 0
	for _, l := range s.lines() {
		if pred(l) {
			n++
		}
	}
	return n
}

// waitFor waits until cond holds. msg explains a failure; it is redacted.
func (s *itScenario) waitFor(cond func() bool, timeout time.Duration, msg func() string) {
	s.t.Helper()
	for deadline := time.Now().Add(timeout); !cond(); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			require.FailNow(s.t, "timed out after "+timeout.String(), s.env.redact(msg()))
		}
	}
}

// waitWritten waits until the writer wrote n lines.
func (s *itScenario) waitWritten(n int) {
	s.t.Helper()
	s.waitFor(func() bool { return s.lastSeq() >= n }, itDeliverTimeout, func() string {
		return fmt.Sprintf("the writer wrote %d of %d lines", s.lastSeq(), n)
	})
}

// waitDelivered waits until the line seq was delivered.
func (s *itScenario) waitDelivered(seq int) {
	s.t.Helper()
	s.waitFor(func() bool { return s.coll.deliveries(seq) > 0 }, itDeliverTimeout, func() string {
		return fmt.Sprintf("line %d was not delivered; %d deliveries so far", seq, s.coll.deliveredCount())
	})
}

// verify waits until every line that must arrive did, keeps reading for
// itSettle, stops the launcher, and checks that every line was delivered
// exactly once, except those mayLose and mayDuplicate accept (nil accepts
// none). It returns the lines lost and the lines delivered more than once.
func (s *itScenario) verify(mayLose, mayDuplicate func(*itLine) bool) (lost, duplicated []*itLine) {
	s.t.Helper()
	if mayLose == nil {
		mayLose = func(*itLine) bool { return false }
	}
	if mayDuplicate == nil {
		mayDuplicate = func(*itLine) bool { return false }
	}
	written := s.lines()
	s.waitFor(func() bool {
		for _, l := range written {
			if !mayLose(l) && s.coll.deliveries(l.seq) == 0 {
				return false
			}
		}
		return true
	}, itDeliverTimeout, func() string {
		var missing []*itLine
		for _, l := range written {
			if !mayLose(l) && s.coll.deliveries(l.seq) == 0 {
				missing = append(missing, l)
			}
		}
		return "lines never delivered: " + seqRanges(missing)
	})
	time.Sleep(itSettle)
	s.launcher.Stop()

	counts, unexpected, malformed := s.coll.result()
	var missing, extra []*itLine
	for _, l := range written {
		switch n := counts[l.seq]; {
		case n == 0 && mayLose(l):
			lost = append(lost, l)
		case n == 0:
			missing = append(missing, l)
		case n > 1 && mayDuplicate(l):
			duplicated = append(duplicated, l)
		case n > 1:
			extra = append(extra, l)
		}
		delete(counts, l.seq)
	}
	for seq := range counts {
		unexpected = append(unexpected, fmt.Sprintf("seq=%06d", seq))
	}
	sort.Strings(unexpected)
	assert.Empty(s.t, seqRanges(missing), "lines lost")
	assert.Empty(s.t, seqRanges(extra), "lines delivered more than once")
	assert.Empty(s.t, unexpected, "lines delivered that were never written")
	assert.Empty(s.t, malformed, "malformed lines delivered (read from a wrong offset?)")
	s.t.Logf("%s: %d lines written, %d lost, %d duplicated in %s", s.name, len(written), len(lost), len(duplicated), time.Since(s.started).Round(time.Second))
	return lost, duplicated
}

// missedBytes returns the bytes the launcher reported missed for the
// scenario's source.
func (s *itScenario) missedBytes() int64 {
	var total int64
	for _, m := range metrics.MissedBytesSnapshot() {
		if m.Source == itSource && m.Service == s.service() {
			total += m.Bytes
		}
	}
	return total
}

// assertMissedBytesBound checks the bytes reported missed by a scenario that
// may lose lines, lost being the lines it lost: they are a lower bound of
// what was lost, as the launcher only reports the bytes it knew were there,
// so they are whole lines and never more than the bytes lost.
func (s *itScenario) assertMissedBytesBound(lost []*itLine) {
	s.t.Helper()
	missed := s.missedBytes()
	assert.LessOrEqual(s.t, missed, int64(len(lost)*itLineLen), "no more bytes are reported missed than were lost")
	assert.Zero(s.t, missed%itLineLen, "the bytes reported missed (%d) are whole lines", missed)
}

func (s *itScenario) reportf(format string, args ...any) {
	s.report.add(s.name + ": " + fmt.Sprintf(format, args...))
}

// itCollector receives what the tailers send to the pipeline, and commits
// each line's offset to the registry as the auditor does once the line is
// delivered.
type itCollector struct {
	name string
	done chan struct{}
	once sync.Once

	mu         sync.Mutex
	registry   *auditorMock.Registry
	counts     map[int]int
	total      int
	unexpected []string
	malformed  []string
}

func newITCollector(name string, ch chan *message.Message, registry *auditorMock.Registry) *itCollector {
	c := &itCollector{name: name, done: make(chan struct{}), registry: registry, counts: make(map[int]int)}
	go func() {
		for {
			select {
			case msg := <-ch:
				c.record(msg)
			case <-c.done:
				return
			}
		}
	}()
	return c
}

func (c *itCollector) stop() { c.once.Do(func() { close(c.done) }) }

func (c *itCollector) record(msg *message.Message) {
	content := string(msg.GetContent())
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total++
	m := itLinePattern.FindStringSubmatch(content)
	switch {
	case m == nil || len(content) != itLineLen-1:
		c.malformed = append(c.malformed, fmt.Sprintf("%.80q (%d bytes)", content, len(content)))
	case m[1] != c.name:
		c.unexpected = append(c.unexpected, content[:min(len(content), 40)])
	default:
		seq, _ := strconv.Atoi(m[2])
		c.counts[seq]++
	}
	if msg.Origin != nil && msg.Origin.Identifier != "" {
		c.registry.SetOffset(msg.Origin.Identifier, msg.Origin.Offset)
	}
}

func (c *itCollector) deliveries(seq int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[seq]
}

func (c *itCollector) deliveredCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

// saveRegistry returns a copy of the registry as it is now.
func (c *itCollector) saveRegistry() *auditorMock.Registry {
	c.mu.Lock()
	defer c.mu.Unlock()
	saved := auditorMock.NewMockRegistry()
	c.registry.Lock()
	for id, offset := range c.registry.StoredOffsets {
		saved.StoredOffsets[id] = offset
	}
	c.registry.Unlock()
	return saved
}

// useRegistry makes the collector commit offsets to registry.
func (c *itCollector) useRegistry(registry *auditorMock.Registry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.registry = registry
}

func (c *itCollector) result() (counts map[int]int, unexpected, malformed []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	counts = make(map[int]int, len(c.counts))
	for seq, n := range c.counts {
		counts[seq] = n
	}
	return counts, append([]string(nil), c.unexpected...), append([]string(nil), c.malformed...)
}

// readGate holds one read of the log source's client, so a scenario can act
// at a precise point: after a listing showed the file with at least minSize
// bytes, the next read of the file waits until the scenario releases it.
type readGate struct {
	mu      sync.Mutex
	path    string // "" when not armed
	minSize int64
	listed  bool
	hit     chan struct{}
	open    chan struct{}
}

// arm makes the gate hold the next read of p that follows a listing showing
// at least minSize bytes.
func (g *readGate) arm(p string, minSize int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.path, g.minSize, g.listed = p, minSize, false
	g.hit, g.open = make(chan struct{}), make(chan struct{})
}

// waitHit waits until a read is held.
func (g *readGate) waitHit(t *testing.T) {
	t.Helper()
	g.mu.Lock()
	hit := g.hit
	g.mu.Unlock()
	select {
	case <-hit:
	case <-time.After(30 * time.Second):
		require.FailNow(t, "the reader never read the file after listing its new size")
	}
}

// release lets the held read go on.
func (g *readGate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.open != nil {
		close(g.open)
		g.open = nil
	}
}

func (g *readGate) listedDir(dir string, entries []client.Entry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.path == "" || g.listed {
		return
	}
	for _, e := range entries {
		if join(dir, e.Name) == g.path && e.Size >= g.minSize {
			g.listed = true
		}
	}
}

func (g *readGate) beforeRead(ctx context.Context, p string, maxLen int) {
	g.mu.Lock()
	if g.path == "" || !g.listed || p != g.path || maxLen == 0 {
		g.mu.Unlock()
		return
	}
	g.path = "" // a single read
	hit, open := g.hit, g.open
	g.mu.Unlock()
	close(hit)
	select {
	case <-open:
	case <-ctx.Done():
	}
}

// gatedClient is the real client with reads the gate can hold.
type gatedClient struct {
	client.Client
	gate *readGate
}

func (c *gatedClient) ListDir(ctx context.Context, dir string) ([]client.Entry, error) {
	entries, err := c.Client.ListDir(ctx, dir)
	if err == nil {
		c.gate.listedDir(dir, entries)
	}
	return entries, err
}

func (c *gatedClient) ReadAt(ctx context.Context, p string, off int64, maxLen int) (client.ReadResult, error) {
	c.gate.beforeRead(ctx, p, maxLen)
	return c.Client.ReadAt(ctx, p, off, maxLen)
}

// Abort keeps client.Abort dropping the real session without a logoff.
func (c *gatedClient) Abort() error { return client.Abort(c.Client) }

// itReport collects what the scenarios measured, printed at the end.
type itReport struct {
	mu    sync.Mutex
	lines []string
}

func (r *itReport) add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
}

func (r *itReport) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return "  " + strings.Join(r.lines, "\n  ")
}

// seqRanges prints the sequence numbers of lines as ranges: "12-15,40".
func seqRanges(lines []*itLine) string {
	if len(lines) == 0 {
		return ""
	}
	seqs := make([]int, 0, len(lines))
	for _, l := range lines {
		seqs = append(seqs, l.seq)
	}
	sort.Ints(seqs)
	var parts []string
	for i := 0; i < len(seqs); {
		j := i
		for j+1 < len(seqs) && seqs[j+1] == seqs[j]+1 {
			j++
		}
		if i == j {
			parts = append(parts, strconv.Itoa(seqs[i]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", seqs[i], seqs[j]))
		}
		i = j + 1
	}
	return strings.Join(parts, ",")
}

// syncBuffer is a bytes.Buffer safe for concurrent writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureAgentLogs sends the Agent's logs to a buffer for the rest of the
// test, so it can check that no password was logged.
func captureAgentLogs(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	logger, err := log.LoggerFromWriterWithMinLevelAndDateFuncLineMsgFormat(buf, log.DebugLvl)
	require.NoError(t, err)
	previous := log.Default()
	log.SetupLogger(logger, "debug")
	t.Cleanup(func() { log.SetupLogger(previous, "debug") })
	return buf
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
