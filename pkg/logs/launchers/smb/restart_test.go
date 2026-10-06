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
