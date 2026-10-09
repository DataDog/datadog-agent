// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && smb && !goexperiment.systemcrypto && !goexperiment.boringcrypto && !requirefips

package smb

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math"
	"strconv"
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
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client/fake"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/smb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	testHost     = "files.example.com"
	testShare    = "logs"
	testPassword = "Sup3r-S3cret-K3y=="
	testTimeout  = 5 * time.Second
	closeTimeout = time.Minute
)

func identifier(p string) string {
	return tailer.Identifier(testHost, testShare, p)
}

// collector receives everything the launcher's tailers forward.
type collector struct {
	ch    chan *message.Message
	mu    sync.Mutex
	msgs  []*message.Message
	done  chan struct{}
	syncs chan chan struct{}
}

func newCollector(t *testing.T, ch chan *message.Message) *collector {
	c := &collector{ch: ch, done: make(chan struct{}), syncs: make(chan chan struct{})}
	go func() {
		for {
			select {
			case msg := <-ch:
				c.mu.Lock()
				c.msgs = append(c.msgs, msg)
				c.mu.Unlock()
			case ack := <-c.syncs:
				close(ack)
			case <-c.done:
				return
			}
		}
	}()
	t.Cleanup(func() { close(c.done) })
	return c
}

// flush returns once every message whose send completed has been recorded.
func (c *collector) flush() {
	ack := make(chan struct{})
	c.syncs <- ack
	<-ack
}

func (c *collector) messages() []*message.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*message.Message(nil), c.msgs...)
}

func (c *collector) lines() []string {
	var lines []string
	for _, msg := range c.messages() {
		lines = append(lines, string(msg.GetContent()))
	}
	return lines
}

// waitLines waits until n messages were received and returns their content.
func (c *collector) waitLines(t *testing.T, n int) []string {
	t.Helper()
	require.Eventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.msgs) >= n
	}, testTimeout, time.Millisecond, "expected %d messages, got %q", n, c.lines())
	return c.lines()
}

type harness struct {
	t        *testing.T
	share    *fake.Share
	clock    *clock.Mock
	launcher *Launcher
	registry *auditorMock.Registry
	source   *sources.LogSource
	scanner  *scanner
	out      *collector
	ctx      context.Context
	stopped  bool
}

type harnessOption func(*config.LogsConfig, *Launcher)

func withPath(p string) harnessOption {
	return func(c *config.LogsConfig, _ *Launcher) { c.Path = p }
}

func withStartPosition(mode string) harnessOption {
	return func(c *config.LogsConfig, _ *Launcher) { c.TailingMode = mode }
}

func withExcludes(patterns ...string) harnessOption {
	return func(c *config.LogsConfig, _ *Launcher) { c.ExcludePaths = patterns }
}

func withLauncher(fn func(*Launcher)) harnessOption {
	return func(_ *config.LogsConfig, l *Launcher) { fn(l) }
}

func newSMBSource(name string, opts ...func(*config.LogsConfig)) *sources.LogSource {
	cfg := &config.LogsConfig{
		Type:        config.SMBType,
		Path:        "app/*.log",
		Source:      "demo",
		Service:     "demo-app",
		TailingMode: "beginning",
		SMB: &config.SMBConfig{
			Host:     testHost,
			Share:    testShare,
			Username: "myacct",
			Password: testPassword,
		},
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return sources.NewLogSource(name, cfg)
}

// newHarness returns a scanner of one source on a fake share. Tests drive
// it with scan; nothing runs in the background but the tailers' decoders and
// forwarders.
func newHarness(t *testing.T, opts ...harnessOption) *harness {
	t.Helper()
	configmock.New(t)
	share := fake.New()
	share.Mkdir("app")
	clk := clock.NewMock()
	l := newTestLauncher(share, clk)
	provider := mock.NewMockProvider()
	l.pipelineProvider = provider
	registry := auditorMock.NewMockRegistry()
	l.registry = registry

	source := newSMBSource("smb-test")
	for _, opt := range opts {
		opt(source.Config, l)
	}
	require.NoError(t, source.Config.Validate())

	h := &harness{
		t:        t,
		share:    share,
		clock:    clk,
		launcher: l,
		registry: registry,
		source:   source,
		out:      newCollector(t, provider.NextPipelineChan()),
		ctx:      context.Background(),
	}
	key, c := l.acquireClient(source.Config.SMB)
	var err error
	h.scanner, err = newScanner(l, source, c, key)
	require.NoError(t, err)
	t.Cleanup(h.stop)
	return h
}

func newTestLauncher(share *fake.Share, clk clock.Clock) *Launcher {
	l := NewLauncher(closeTimeout)
	l.clock = clk
	l.dial = share.Dial
	l.forceReadEvery = 3
	return l
}

func (h *harness) scan() {
	h.t.Helper()
	h.scanner.scan(h.ctx)
}

// stop stops every tailer, which flushes them, and closes the client.
func (h *harness) stop() {
	if h.stopped {
		return
	}
	h.stopped = true
	h.scanner.stopTailers()
	h.launcher.releaseClient(h.scanner.clientKey)
	h.out.flush()
}

// finish stops the scanner and returns everything that was forwarded.
func (h *harness) finish() []string {
	h.t.Helper()
	h.stop()
	return h.out.lines()
}

func (h *harness) activeTailer(p string) *tailer.Tailer {
	h.t.Helper()
	tl := h.scanner.active[p]
	require.NotNil(h.t, tl, "no active tailer for %s", p)
	return tl
}

// messagesFor returns the content of the messages committing offsets under
// the identifier of p, and those committing none.
func (h *harness) messagesFor(p string) (committed, uncommitted []string) {
	for _, msg := range h.out.messages() {
		switch msg.Origin.Identifier {
		case identifier(p):
			committed = append(committed, string(msg.GetContent()))
		case "":
			uncommitted = append(uncommitted, string(msg.GetContent()))
		}
	}
	return committed, uncommitted
}

// trackedIDs returns the IDs of the tailers agent status lists.
func trackedIDs(l *Launcher) []string {
	var ids []string
	for _, t := range l.tailers.All() {
		ids = append(ids, t.GetID())
	}
	return ids
}

func lines(from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

func want(from, to int) []string {
	var w []string
	for i := from; i <= to; i++ {
		w = append(w, fmt.Sprintf("line %d", i))
	}
	return w
}

func TestAppend(t *testing.T) {
	h := newHarness(t)
	id := h.share.Write("app/app.log", []byte(lines(1, 2)))

	h.scan()
	assert.Equal(t, want(1, 2), h.out.waitLines(t, 2))
	assert.True(t, h.source.Status().IsSuccess())
	assert.Equal(t, []string{identifier("app/app.log")}, h.source.GetInputs())
	assert.True(t, h.registry.TailedSources[identifier("app/app.log")])

	h.share.Append("app/app.log", []byte(lines(3, 3)))
	h.scan()
	got := h.out.waitLines(t, 3)
	assert.Equal(t, want(1, 3), got)
	last := h.out.messages()[2]
	assert.Equal(t, identifier("app/app.log"), last.Origin.Identifier)
	assert.Equal(t, tailer.EncodeOffset(id, int64(len(lines(1, 3)))), last.Origin.Offset)

	assert.Equal(t, want(1, 3), h.finish())
	assert.Empty(t, h.source.GetInputs())
	assert.False(t, h.registry.TailedSources[identifier("app/app.log")])
	assert.Zero(t, h.launcher.tailers.Count())
}

func TestIdleFileCostsNoRead(t *testing.T) {
	h := newHarness(t, withLauncher(func(l *Launcher) { l.forceReadEvery = 100 }))
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	reads := h.share.Calls(fake.OpReadAt)
	for range 5 {
		h.scan()
	}
	assert.Equal(t, reads, h.share.Calls(fake.OpReadAt), "a file whose listing shows no growth is not opened")
	assert.Equal(t, 6, h.share.Calls(fake.OpListDir), "one listing per directory per scan")
}

func TestGlobAcrossDirectoriesAndExcludes(t *testing.T) {
	h := newHarness(t, withPath("*/app.log"), withExcludes("skip/*"))
	h.share.Write("a/app.log", []byte("from a\n"))
	h.share.Write("b/app.log", []byte("from b\n"))
	h.share.Write("b/other.log", []byte("not matched\n"))
	h.share.Write("skip/app.log", []byte("excluded\n"))
	h.share.Write("app.log", []byte("not in a directory\n"))

	h.scan()
	assert.ElementsMatch(t, []string{"from a", "from b"}, h.out.waitLines(t, 2))
	// The root, then a, app, b and skip: each directory listed once.
	assert.Equal(t, 5, h.share.Calls(fake.OpListDir))
	assert.ElementsMatch(t, []string{"from a", "from b"}, h.finish())
}

func TestRotationByRenameAndCreateDrainsTheOldFile(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t)
	oldID := h.share.Write("app/app.log", []byte(lines(1, 2)))
	h.scan()
	h.out.waitLines(t, 2)

	// Written just before the rotation, never seen at app/app.log.
	h.share.Append("app/app.log", []byte(lines(3, 3)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	newID := h.share.Write("app/app.log", []byte(lines(4, 4)))

	h.scan()
	assert.ElementsMatch(t, want(1, 4), h.out.waitLines(t, 4))
	require.Len(t, h.scanner.draining, 1)
	assert.Equal(t, oldID, h.scanner.draining[0].t.FileID())
	assert.Equal(t, "app/app.log.1", h.scanner.draining[0].t.ReadPath())
	assert.Equal(t, newID, h.activeTailer("app/app.log").FileID())
	drainID := fmt.Sprintf("%s (rotated, FileId %d)", identifier("app/app.log"), oldID)
	assert.ElementsMatch(t, []string{identifier("app/app.log"), drainID}, trackedIDs(h.launcher),
		"agent status lists the drain next to the path's new tailer")
	assert.Contains(t, h.scanner.draining[0].t.GetInfo().Rendered(), "Draining Since")

	h.scan()
	require.Len(t, h.scanner.draining, 1)
	h.scan()
	assert.Empty(t, h.scanner.draining, "the drain ends after two polls without new data")
	assert.Equal(t, []string{identifier("app/app.log")}, trackedIDs(h.launcher))

	committed, uncommitted := h.messagesFor("app/app.log")
	assert.Equal(t, append(want(1, 2), "line 4"), committed)
	assert.Equal(t, []string{"line 3"}, uncommitted, "the drain commits no offset")
	assert.Empty(t, metrics.MissedBytesSnapshot())
	assert.ElementsMatch(t, want(1, 4), h.finish())
}

func TestDrainFollowsAWriterStillAppendingToTheRotatedFile(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", nil)

	h.scan()
	for i := 2; i <= 4; i++ {
		// The writer kept its handle on the rotated file.
		h.share.Append("app/app.log.1", []byte(lines(i, i)))
		h.scan()
		require.Len(t, h.scanner.draining, 1, "a growing file keeps being drained")
	}
	h.scan()
	h.scan()
	assert.Empty(t, h.scanner.draining)
	assert.Equal(t, want(1, 4), h.finish())
}

func TestMultipleRotationsBetweenScans(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.out.waitLines(t, 1)

	h.share.Append("app/app.log", []byte(lines(2, 2)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", []byte("never seen\n"))
	require.NoError(t, h.share.Rename("app/app.log.1", "app/app.log.2"))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", []byte(lines(3, 3)))

	for range 3 {
		h.scan()
	}
	assert.Empty(t, h.scanner.draining)
	// The first file is found by FileId two renames later. The file that
	// lived at app/app.log only between two scans was never seen: it is
	// not read, and not reported as missed either.
	assert.ElementsMatch(t, want(1, 3), h.finish())
}

func TestCopyTruncate(t *testing.T) {
	h := newHarness(t)
	id := h.share.Write("app/app.log", []byte(lines(1, 3)))
	h.scan()
	h.out.waitLines(t, 3)

	h.share.Write("app/app.log.1", []byte(lines(1, 3)))
	require.NoError(t, h.share.Truncate("app/app.log", 0))
	h.share.Append("app/app.log", []byte(lines(4, 4)))

	h.scan()
	got := h.out.waitLines(t, 4)
	assert.Equal(t, want(1, 4), got)
	last := h.out.messages()[3]
	assert.Equal(t, tailer.EncodeOffset(id, int64(len(lines(4, 4)))), last.Origin.Offset, "the new content is read from offset 0")
	assert.Equal(t, id, h.activeTailer("app/app.log").FileID())
	assert.Equal(t, want(1, 4), h.finish())
}

func TestFileIDReuseWithShrink(t *testing.T) {
	h := newHarness(t)
	id := h.share.Write("app/app.log", []byte(lines(1, 3)))
	h.scan()
	h.out.waitLines(t, 3)

	// Deleted and recreated under the same FileId (as Samba does with inode
	// numbers), with less data than was read.
	h.share.Recreate("app/app.log", id)
	h.share.Append("app/app.log", []byte(lines(4, 4)))

	h.scan()
	assert.Equal(t, want(1, 4), h.out.waitLines(t, 4))
	assert.Equal(t, want(1, 4), h.finish())
}

func TestDeleteAndRecreateRecordsMissedBytes(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 2)))
	h.scan()
	h.out.waitLines(t, 2)

	// line 3 is listed, then the file is deleted and recreated before it is
	// read: SMB cannot open a file by FileId, so line 3 is lost.
	h.share.Append("app/app.log", []byte(lines(3, 3)))
	var once sync.Once
	h.share.SetHook(func(op fake.Op, _ string) {
		if op == fake.OpReadAt {
			once.Do(func() {
				h.share.Recreate("app/app.log", 0)
				h.share.Append("app/app.log", []byte(lines(4, 4)))
			})
		}
	})
	before := metrics.BytesMissed.Value()

	h.scan() // the read finds another FileId and forwards nothing
	h.scan() // the listing shows the new file: the old one is gone
	assert.Equal(t, []string{"line 1", "line 2", "line 4"}, h.out.waitLines(t, 3))
	assert.Empty(t, h.scanner.draining)

	missed := int64(len(lines(3, 3)))
	assert.Equal(t, missed, metrics.BytesMissed.Value()-before)
	snapshot := metrics.MissedBytesSnapshot()
	require.Len(t, snapshot, 1)
	assert.Equal(t, "demo", snapshot[0].Source)
	assert.Equal(t, "demo-app", snapshot[0].Service)
	assert.Equal(t, missed, snapshot[0].Bytes)
	assert.Equal(t, []string{"line 1", "line 2", "line 4"}, h.finish())
}

func TestPathGoneDrainsUnderTheNewName(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.share.Append("app/app.log", []byte(lines(2, 2)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1")) // nothing created yet

	h.scan()
	assert.Equal(t, want(1, 2), h.out.waitLines(t, 2))
	assert.Empty(t, h.scanner.active)
	assert.Empty(t, h.source.GetInputs())

	// The writer creates the new file late: it is read from the beginning,
	// although nothing tailed the path for a while.
	h.share.Write("app/app.log", []byte(lines(3, 3)))
	h.scan()
	assert.Equal(t, want(1, 3), h.out.waitLines(t, 3))
	assert.Equal(t, want(1, 3), h.finish())
}

func TestRenameToAnotherMatchedPathIsNotReadTwice(t *testing.T) {
	h := newHarness(t, withPath("app/*"))
	id := h.share.Write("app/a.log", []byte(lines(1, 1)))
	h.scan()
	h.share.Append("app/a.log", []byte(lines(2, 2)))
	require.NoError(t, h.share.Rename("app/a.log", "app/b.log"))

	h.scan() // a.log is gone: its file is drained at b.log, b.log waits
	assert.Nil(t, h.scanner.active["app/b.log"])
	h.scan()
	h.scan() // second poll without new data: the drain hands b.log over
	assert.Empty(t, h.scanner.draining)
	h.share.Append("app/b.log", []byte(lines(3, 3)))
	h.scan()
	assert.Equal(t, id, h.activeTailer("app/b.log").FileID())
	// line 2 comes from the drain, line 3 from b.log's tailer: their order
	// is not guaranteed.
	assert.ElementsMatch(t, want(1, 3), h.out.waitLines(t, 3))
	assert.ElementsMatch(t, want(1, 3), h.finish())
}

// TestRotationChainMatchedByThePatternIsNotReadTwice rotates twice with a
// pattern that matches the rotated names too: app.log.1 then holds a file that
// was just drained as app.log's previous file, at a path whose own previous
// file rotated away. It resumes where the drain stopped.
func TestRotationChainMatchedByThePatternIsNotReadTwice(t *testing.T) {
	h := newHarness(t, withPath("app/*"))
	first := h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()

	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	second := h.share.Write("app/app.log", []byte(lines(2, 2)))
	for range 3 {
		h.scan()
	}
	require.Empty(t, h.scanner.draining)
	assert.Equal(t, first, h.activeTailer("app/app.log.1").FileID())

	require.NoError(t, h.share.Rename("app/app.log.1", "app/app.log.2"))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", []byte(lines(3, 3)))
	for range 3 {
		h.scan()
	}
	require.Empty(t, h.scanner.draining)
	assert.Equal(t, second, h.activeTailer("app/app.log.1").FileID())
	assert.Equal(t, first, h.activeTailer("app/app.log.2").FileID())

	h.share.Append("app/app.log.1", []byte(lines(4, 4)))
	h.scan()
	assert.ElementsMatch(t, want(1, 4), h.out.waitLines(t, 4))
	assert.ElementsMatch(t, want(1, 4), h.finish(), "each line is sent once")
}

func TestFileRenamedAwayAndBackIsNotReadTwice(t *testing.T) {
	h := newHarness(t)
	id := h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.out.waitLines(t, 1)

	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.tmp"))
	h.share.Append("app/app.log.tmp", []byte(lines(2, 2)))
	h.scan() // app.log is gone: its file is drained at app.log.tmp
	require.NoError(t, h.share.Rename("app/app.log.tmp", "app/app.log"))
	h.share.Append("app/app.log", []byte(lines(3, 3)))
	for range 3 {
		h.scan() // the drain follows the file back, then hands app.log over
	}
	require.Empty(t, h.scanner.draining)

	h.share.Append("app/app.log", []byte(lines(4, 4)))
	h.scan()
	assert.Equal(t, id, h.activeTailer("app/app.log").FileID())
	assert.ElementsMatch(t, want(1, 4), h.out.waitLines(t, 4))
	assert.ElementsMatch(t, want(1, 4), h.finish(), "each line is sent once")
	committed, uncommitted := h.messagesFor("app/app.log")
	assert.Equal(t, []string{"line 1", "line 4"}, committed)
	assert.Equal(t, want(2, 3), uncommitted, "the drain commits no offset")
}

func TestTruncatedDrainHandsItsMatchedPathOverFromTheBeginning(t *testing.T) {
	h := newHarness(t, withPath("app/*"))
	h.share.Write("app/a.log", []byte(lines(1, 2)))
	h.scan()
	require.NoError(t, h.share.Rename("app/a.log", "app/b.log"))
	h.scan() // a.log is gone: its file is drained at b.log, b.log waits
	require.Len(t, h.scanner.draining, 1)

	// copytruncate of the drained file, with new content
	require.NoError(t, h.share.Truncate("app/b.log", 0))
	h.share.Append("app/b.log", []byte(lines(3, 3)))
	h.scan()
	require.Empty(t, h.scanner.draining)
	h.scan() // b.log is read again from offset 0, in a single scan
	assert.ElementsMatch(t, want(1, 3), h.out.waitLines(t, 3))
	assert.ElementsMatch(t, want(1, 3), h.finish())
}

func TestDrainTimeoutRecordsMissedBytes(t *testing.T) {
	metrics.ResetMissedBytesForTest()
	t.Cleanup(metrics.ResetMissedBytesForTest)
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.share.Append("app/app.log", []byte(lines(2, 2)))
	require.NoError(t, h.share.Rename("app/app.log", "app/app.log.1"))
	h.share.Write("app/app.log", nil)
	// The rotated file stays locked by its writer for the whole drain.
	h.share.FailNextPath(fake.OpReadAt, "app/app.log.1", fake.ErrSharing, fake.ErrSharing)

	h.scan()
	assert.True(t, h.source.Status().IsSuccess(), "a sharing violation is retried without an error status")
	h.clock.Add(closeTimeout)
	h.scan()
	assert.Empty(t, h.scanner.draining)
	snapshot := metrics.MissedBytesSnapshot()
	require.Len(t, snapshot, 1)
	assert.Equal(t, int64(len(lines(2, 2))), snapshot[0].Bytes)
	assert.Equal(t, want(1, 1), h.finish())
}

func TestIdentityChangeWithoutListingFileIDs(t *testing.T) {
	h := newHarness(t)
	h.share.SetListingFileIDs(false)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.out.waitLines(t, 1)
	h.share.Recreate("app/app.log", 0)
	h.share.Append("app/app.log", []byte(lines(2, 3)))

	// The listing cannot show the replacement, the read does: the path is
	// read again from the beginning in the same scan.
	h.scan()
	assert.Equal(t, want(1, 3), h.out.waitLines(t, 3))
	assert.Equal(t, want(1, 3), h.finish())
}

func TestStaleListingSizeForcesAPeriodicRead(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.share.SetStaleListing(true)
	h.share.Append("app/app.log", []byte(lines(2, 2)))
	reads := h.share.Calls(fake.OpReadAt)

	h.scan()
	h.scan()
	assert.Equal(t, reads, h.share.Calls(fake.OpReadAt), "the stale listing shows no growth")
	h.scan()
	assert.Equal(t, reads+1, h.share.Calls(fake.OpReadAt), "every third poll reads anyway")
	assert.Equal(t, want(1, 2), h.out.waitLines(t, 2))
	assert.Equal(t, want(1, 2), h.finish())
}

func TestTransientErrorRedialsAndResumesWithoutResending(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 2)))
	h.scan()
	h.out.waitLines(t, 2)
	require.Equal(t, 1, h.share.Calls(fake.OpDial))

	// The session served long enough for its loss to be routine (it
	// expired): the next scan dials again right away.
	h.clock.Add(30 * time.Second)
	h.share.DropSessions()
	h.share.Append("app/app.log", []byte(lines(3, 3)))
	h.scan()
	require.True(t, h.source.Status().IsError())
	assert.Contains(t, h.source.Status().GetError(), "cannot reach smb://files.example.com/logs, retrying")
	require.NotNil(t, h.scanner.active["app/app.log"], "the tailer survives the error, with its offsets")

	h.scan()
	assert.Equal(t, 2, h.share.Calls(fake.OpDial), "the session was dialed again")
	assert.True(t, h.source.Status().IsSuccess(), "the status recovers")
	assert.Equal(t, want(1, 3), h.out.waitLines(t, 3))
	assert.Equal(t, want(1, 3), h.finish(), "nothing was sent twice")
}

// TestTransientReadErrorsDoNotRedialPerFile covers a server that accepts
// sessions but fails reads (overloaded, throttling): a scan dials at most
// once, however many files it reads, and new sessions follow the backoff.
func TestTransientReadErrorsDoNotRedialPerFile(t *testing.T) {
	h := newHarness(t)
	files := []string{"app/a.log", "app/b.log", "app/c.log", "app/d.log", "app/e.log"}
	for _, f := range files {
		h.share.Write(f, []byte(f+" 1\n"))
	}
	h.scan()
	h.out.waitLines(t, len(files))
	require.Equal(t, 1, h.share.Calls(fake.OpDial))

	h.share.FailNext(fake.OpReadAt, fake.ErrOverloaded, fake.ErrOverloaded)
	for _, f := range files {
		h.share.Append(f, []byte(f+" 2\n"))
	}
	h.scan()
	assert.True(t, h.source.Status().IsError())
	assert.Equal(t, 1, h.share.Calls(fake.OpDial), "the files after the first failure do not dial")
	h.scan()
	assert.Equal(t, 1, h.share.Calls(fake.OpDial), "no dial before the backoff (1s) passed")

	h.clock.Add(time.Second)
	h.scan() // the new session fails too: the next one waits 2s
	assert.Equal(t, 2, h.share.Calls(fake.OpDial))
	h.clock.Add(time.Second)
	h.scan()
	assert.Equal(t, 2, h.share.Calls(fake.OpDial))
	h.clock.Add(time.Second)
	h.scan()
	assert.Equal(t, 3, h.share.Calls(fake.OpDial))
	assert.True(t, h.source.Status().IsSuccess(), "the server recovered")

	var want []string
	for _, f := range files {
		want = append(want, f+" 1", f+" 2")
	}
	assert.ElementsMatch(t, want, h.finish(), "every line is sent once")
}

func TestServerOutageWarnsOnce(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.share.FailNext(fake.OpDial, fake.ErrTransient, fake.ErrTransient, fake.ErrTransient)

	var statuses []string
	logs := captureLogs(t, func() {
		// Dial failures 1s, 2s and 4s apart, with scans in the backoffs.
		for _, step := range []time.Duration{0, 500 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 2 * time.Second} {
			h.clock.Add(step)
			h.scan()
			require.True(t, h.source.Status().IsError())
			statuses = append(statuses, h.source.Status().GetError())
		}
	})
	assert.Equal(t, 3, h.share.Calls(fake.OpDial))
	for _, status := range statuses[1:] {
		assert.Equal(t, statuses[0], status, "the status does not change while the server stays down")
	}
	assert.Equal(t, 1, strings.Count(logs, "[WARN] SMB source "), "one warning for the outage, not one per scan:\n%s", logs)

	h.clock.Add(4 * time.Second)
	h.scan()
	assert.True(t, h.source.Status().IsSuccess())
	assert.Equal(t, want(1, 1), h.out.waitLines(t, 1))
}

// captureLogs redirects the Agent logger to a buffer while fn runs and
// returns everything logged, at every level.
func captureLogs(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(w, log.TraceLvl)
	require.NoError(t, err)
	previous := log.Default()
	t.Cleanup(func() { log.SetupLogger(previous, "debug") })
	log.SetupLogger(logger, "trace")
	fn()
	require.NoError(t, w.Flush())
	return buf.String()
}

func TestListingErrorKeepsTailers(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.scan()
	h.share.FailNext(fake.OpListDir, fake.ErrAccessDenied)
	h.scan()
	assert.NotNil(t, h.scanner.active["app/app.log"], "a failed listing does not make files look gone")
	assert.Empty(t, h.scanner.draining)
	assert.True(t, h.source.Status().IsError())
}

func TestAuthErrorStatus(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.share.FailNext(fake.OpDial, fmt.Errorf("logon failed with password %s: %w", testPassword, fake.ErrAuth))

	h.scan()
	require.True(t, h.source.Status().IsError())
	msg := h.source.Status().GetError()
	assert.Contains(t, msg, "the server rejected the credentials or denied access")
	assert.Contains(t, msg, "Check the username, password and domain")
	assert.NotContains(t, msg, testPassword)

	// Bad credentials are retried after 30s, not on every scan.
	h.scan()
	assert.Equal(t, 1, h.share.Calls(fake.OpDial))
	h.clock.Add(30 * time.Second)
	h.scan()
	assert.Equal(t, 2, h.share.Calls(fake.OpDial))
	assert.True(t, h.source.Status().IsSuccess())
	assert.Equal(t, want(1, 1), h.out.waitLines(t, 1))
}

func TestMissingDirectoryStatus(t *testing.T) {
	h := newHarness(t, withPath("missing/*.log"))
	h.scan()
	require.True(t, h.source.Status().IsError())
	assert.Contains(t, h.source.Status().GetError(), `directory "missing" does not exist`)
}

func TestStartPosition(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want []string
	}{
		{mode: "beginning", want: want(1, 3)},
		{mode: "end", want: want(3, 3)},
		{mode: "", want: want(3, 3)}, // end is the default, as for file sources
	} {
		t.Run("mode="+tc.mode, func(t *testing.T) {
			h := newHarness(t, withStartPosition(tc.mode))
			h.share.Write("app/app.log", []byte(lines(1, 2)))
			h.scan()
			h.share.Append("app/app.log", []byte(lines(3, 3)))
			h.scan()
			assert.Equal(t, tc.want, h.finish())
		})
	}
}

func TestFilesCreatedAfterTheSourceStartedAreReadFromTheBeginning(t *testing.T) {
	// start_position: end, the default, applies to the files that were there
	// when the source started, as for file sources.
	h := newHarness(t, withPath("*/*.log"), withStartPosition(""))
	h.share.Write("app/old.log", []byte(lines(1, 2)))
	h.scan()

	// Created after the source started: in a directory listed before, in a
	// directory created since, and in a directory that was empty.
	h.share.Mkdir("empty")
	h.scan()
	h.share.Write("app/new.log", []byte(lines(3, 4)))
	h.share.Write("other/app.log", []byte(lines(5, 5)))
	h.share.Write("empty/first.log", []byte(lines(6, 6)))
	h.share.Append("app/old.log", []byte(lines(7, 7)))
	h.scan()
	assert.ElementsMatch(t, want(3, 7), h.finish())
}

func TestStartPositionAppliesToTheFilesOfTheFirstFullListing(t *testing.T) {
	t.Run("share unreachable when the source starts", func(t *testing.T) {
		h := newHarness(t, withStartPosition("end"))
		h.share.Write("app/app.log", []byte(lines(1, 2)))
		h.share.FailNext(fake.OpDial, fake.ErrTransient)
		h.scan() // nothing is listed
		require.True(t, h.source.Status().IsError())
		h.clock.Add(time.Second) // the dial backoff
		h.scan()                 // the file may have been there before the source started
		h.share.Append("app/app.log", []byte(lines(3, 3)))
		h.scan()
		assert.Equal(t, want(3, 3), h.finish())
	})
	t.Run("file locked when the source starts", func(t *testing.T) {
		h := newHarness(t, withStartPosition("end"))
		h.share.Write("app/app.log", []byte(lines(1, 2)))
		h.share.FailNextPath(fake.OpReadAt, "app/app.log", fake.ErrSharing)
		h.scan() // listed, but it cannot be opened yet
		require.Empty(t, h.scanner.active)
		h.scan() // still a file that was there when the source started
		h.share.Append("app/app.log", []byte(lines(3, 3)))
		h.scan()
		assert.Equal(t, want(3, 3), h.finish())
	})
}

func TestPersistentOpenFailuresAreReported(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"sharing violation", fake.ErrSharing, "another program keeps it open without letting others read it (sharing violation)"},
		{"not found while listed", fake.ErrNotFound, "not found on smb://files.example.com/logs"},
	} {
		t.Run(tc.name+" of a new file", func(t *testing.T) {
			h := newHarness(t)
			h.share.Write("app/app.log", []byte(lines(1, 1)))
			h.share.FailNextPath(fake.OpReadAt, "app/app.log", tc.err, tc.err, tc.err)

			h.scan()
			assert.True(t, h.source.Status().IsSuccess(), "first taken for a rotation race")
			h.clock.Add(blockedReportAfter - time.Second)
			h.scan()
			assert.True(t, h.source.Status().IsSuccess())
			h.clock.Add(time.Second)
			h.scan()
			require.True(t, h.source.Status().IsError(), "still failing after %s", blockedReportAfter)
			msg := h.source.Status().GetError()
			assert.Contains(t, msg, tc.want)
			assert.Contains(t, msg, identifier("app/app.log")+" still cannot be opened after 30s")

			h.scan() // the file can be opened again
			assert.True(t, h.source.Status().IsSuccess())
			assert.Equal(t, want(1, 1), h.finish())
		})
		t.Run(tc.name+" of a tailed file", func(t *testing.T) {
			h := newHarness(t)
			h.share.Write("app/app.log", []byte(lines(1, 1)))
			h.scan()
			h.out.waitLines(t, 1)
			h.share.Append("app/app.log", []byte(lines(2, 2)))
			h.share.FailNextPath(fake.OpReadAt, "app/app.log", tc.err, tc.err)

			h.scan()
			assert.True(t, h.source.Status().IsSuccess())
			h.clock.Add(blockedReportAfter)
			h.scan()
			require.True(t, h.source.Status().IsError())
			assert.Contains(t, h.source.Status().GetError(), tc.want)
			h.scan()
			assert.True(t, h.source.Status().IsSuccess())
			assert.Equal(t, want(1, 2), h.finish())
		})
	}
}

func TestPollIntervalIsBounded(t *testing.T) {
	configmock.New(t)
	l := newTestLauncher(fake.New(), clock.NewMock())
	for _, tc := range []struct {
		seconds float64
		want    time.Duration
	}{
		{0, defaultPollInterval},
		{-1, defaultPollInterval},
		{math.NaN(), defaultPollInterval},
		{0.5, 500 * time.Millisecond},
		// Validation refuses these, but a ticker with a zero or negative
		// interval would panic: they are bounded again.
		{1e-10, config.SMBMinPollInterval},
		{1e10, config.SMBMaxPollInterval},
		{math.Inf(1), config.SMBMaxPollInterval},
	} {
		source := newSMBSource("smb-test", func(c *config.LogsConfig) { c.SMB.PollInterval = tc.seconds })
		s, err := newScanner(l, source, nil, clientKey{})
		require.NoError(t, err)
		assert.Equal(t, tc.want, s.interval, "poll_interval %v", tc.seconds)
	}
}

func TestScannerRefusesPathsLeavingTheShare(t *testing.T) {
	configmock.New(t)
	l := newTestLauncher(fake.New(), clock.NewMock())
	for _, opt := range []func(*config.LogsConfig){
		func(c *config.LogsConfig) { c.Path = "../logs/*.log" },
		func(c *config.LogsConfig) { c.ExcludePaths = []string{"app/../x"} },
	} {
		_, err := newScanner(l, newSMBSource("smb-test", opt), nil, clientKey{})
		assert.ErrorContains(t, err, "invalid smb")
	}
}

func TestRegistryOffsetRecovery(t *testing.T) {
	content := lines(1, 3)
	afterLine1 := int64(len(lines(1, 1)))
	for _, tc := range []struct {
		name   string
		mode   string
		stored func(id uint64) string
		want   []string
	}{
		{
			name:   "same file",
			mode:   "end",
			stored: func(id uint64) string { return tailer.EncodeOffset(id, afterLine1) },
			want:   want(2, 3),
		},
		{
			name:   "bare offset",
			mode:   "end",
			stored: func(uint64) string { return strconv.FormatInt(afterLine1, 10) },
			want:   want(2, 3),
		},
		{
			name:   "file replaced while the agent was down",
			mode:   "end",
			stored: func(id uint64) string { return tailer.EncodeOffset(id+1, afterLine1) },
			want:   want(1, 3),
		},
		{
			// Truncated while the agent was down: everything in the file is
			// new, whatever start_position says.
			name:   "offset past the end, start_position beginning",
			mode:   "beginning",
			stored: func(id uint64) string { return tailer.EncodeOffset(id, 1000) },
			want:   want(1, 3),
		},
		{
			name:   "offset past the end, start_position end",
			mode:   "end",
			stored: func(id uint64) string { return tailer.EncodeOffset(id, 1000) },
			want:   want(1, 3),
		},
		{
			name:   "unreadable offset, start_position end",
			mode:   "end",
			stored: func(uint64) string { return "not an offset" },
			want:   nil,
		},
		{
			name:   "unreadable offset",
			mode:   "beginning",
			stored: func(uint64) string { return "not an offset" },
			want:   want(1, 3),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, withStartPosition(tc.mode))
			// The listing reports a stale size (0): the start position
			// relies on the size reported when the file is opened.
			id := h.share.Write("app/app.log", nil)
			h.share.SetStaleListing(true)
			h.share.Write("app/app.log", []byte(content))
			h.registry.SetOffset(identifier("app/app.log"), tc.stored(id))

			h.scan()
			assert.Equal(t, tc.want, h.finish())
		})
	}
}

func TestPasswordNeverExposed(t *testing.T) {
	h := newHarness(t)
	h.share.Write("app/app.log", []byte(lines(1, 1)))
	h.share.FailNext(fake.OpDial, fmt.Errorf("server said %q: %w", testPassword, fake.ErrTransient))

	h.scan()
	require.True(t, h.source.Status().IsError())
	failedStatus := h.source.Status().GetError()
	h.clock.Add(time.Second) // the dial backoff
	h.scan()
	h.out.waitLines(t, 1)

	exposed := []string{
		failedStatus,
		h.source.Dump(true),
		h.source.Config.Dump(true),
		fmt.Sprintf("%v %+v %#v", h.source.Config.SMB, h.source.Config.SMB, h.source.Config.SMB),
		strings.Join(h.source.GetInputs(), " "),
	}
	publicJSON, err := h.source.PublicJSON()
	require.NoError(t, err)
	exposed = append(exposed, string(publicJSON))
	for _, rendered := range h.activeTailer("app/app.log").GetInfo().Rendered() {
		exposed = append(exposed, rendered...)
	}
	for _, msg := range h.out.messages() {
		exposed = append(exposed, msg.Origin.Identifier, msg.Origin.Offset, strings.Join(msg.Origin.Tags(), ","))
	}
	exposed = append(exposed, h.scanner.statusError(&scanErrors{err: fmt.Errorf("echo %s", testPassword)}).Error())
	for _, s := range exposed {
		assert.NotContains(t, s, testPassword)
	}
}
