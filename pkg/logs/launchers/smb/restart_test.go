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

// rotateWithLockedDrain rotates app/app.log right after "line 3" was appended
// to it, while the rotated file is locked: the scan that sees the rotation
// knows line 3 is there but cannot read it, so the drain still holds it.
// It returns the FileIds of the old and new files.
func rotateWithLockedDrain(t *testing.T, h *harness) (oldID, newID uint64) {
	t.Helper()
	oldID = h.share.Write("app/app.log", []byte(lines(1, 2)))
	h.scan()
	h.out.waitLines(t, 2)
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
	fileID, offset, ok := tailer.DecodeOffset(h.registry.GetOffset(identifier("app/app.log")))
	require.True(t, ok)
	assert.Equal(t, newID, fileID)
	assert.Equal(t, int64(len(lines(4, 4))), offset)
	assert.Len(t, h.registry.StoredOffsets, 1, "nothing is committed under another identifier")

	restarted.share.Append("app/app.log", []byte(lines(5, 5)))
	restarted.scan()
	assert.Equal(t, []string{"line 1", "line 2", "line 4", "line 5"}, restarted.finish(),
		"the new file resumes where it stopped, and the rotated file, which the pattern does not match, is not read again")

	// KNOWN GAP: line 3 is lost with the drain, which nothing resumes after a
	// restart (the registry is keyed by path), and the loss is not reported:
	// stopTailers stops drains without RecordMissedBytes. Update this
	// assertion when it reports UnreadBytes as missed.
	assert.Empty(t, metrics.MissedBytesSnapshot(), "the drain's unread bytes are not reported missed")
}

// TestRestartMidDrainReadsARotatedFileMatchedByThePatternAgain covers a
// pattern that matches the rotated name too. The rotated file's path gets its
// own tailer (and registry offsets) only when the drain ends and hands it
// over; a restart before that finds a matched file with no stored offset.
func TestRestartMidDrainReadsARotatedFileMatchedByThePatternAgain(t *testing.T) {
	h := newHarness(t, withPath("app/*"))
	rotateWithLockedDrain(t, h)
	require.Nil(t, h.scanner.active["app/app.log.1"], "app.log.1 waits for the drain")

	restarted := h.restart()
	assert.Empty(t, h.registry.GetOffset(identifier("app/app.log.1")), "nothing was committed for the rotated file's path")
	restarted.scan()
	got := restarted.finish()

	// KNOWN GAP: with start_position beginning, the restarted launcher reads
	// app.log.1 from offset 0, so lines 1 and 2, already sent from app.log,
	// are sent again (with start_position end it would skip line 3 instead).
	// A drain whose file sits at a matched path could commit its offsets
	// under that path's identifier, so a restart resumes there. Update this
	// assertion when it does.
	assert.ElementsMatch(t, []string{"line 1", "line 2", "line 4", "line 1", "line 2", "line 3"}, got)
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
	got := st.out.lines()
	assert.Subset(t, got, []string{"line 1", "line 2", "line 4", "line 5"})
	assert.Len(t, got, len(dedupe(got)), "nothing is sent twice across the replacement")
	// KNOWN GAP: Launcher.replace hands the active tailers' offsets to the new
	// scanner (resumeFrom), but not the drains: line 3 is lost, and not
	// reported missed. Update these assertions when the drains are handed
	// over too.
	assert.NotContains(t, got, "line 3")
	assert.Empty(t, metrics.MissedBytesSnapshot())
}

func dedupe(lines []string) map[string]bool {
	set := make(map[string]bool, len(lines))
	for _, l := range lines {
		set[l] = true
	}
	return set
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
