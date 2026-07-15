// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

// Package container provides container-related log tailers
package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	dockerclient "github.com/moby/moby/client"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/tag"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"

	"github.com/stretchr/testify/assert"

	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
)

const testReadTimeout = 10 * time.Millisecond

type mockReaderNoSleep struct{}

func (m *mockReaderNoSleep) Read(p []byte) (int, error) {
	s := strings.NewReader("Some bytes")
	n, _ := io.ReadFull(s, p)
	return n, nil
}

func (m *mockReaderNoSleep) Close() error {
	return nil
}

type mockReaderSleep struct {
	ctx     context.Context
	timeout time.Duration
}

func newMockReaderSleep(ctx context.Context) *mockReaderSleep {
	return &mockReaderSleep{ctx: ctx, timeout: 2 * testReadTimeout}
}

// Read mocks the Docker CLI read function
func (m *mockReaderSleep) Read(p []byte) (int, error) {
	s := strings.NewReader("Some bytes")
	var n int
	var err error
	wg := sync.WaitGroup{}
	wg.Add(1)
	time.AfterFunc(m.timeout, func() {
		select {
		case <-m.ctx.Done():
			err = m.ctx.Err()
		default:
			n, err = io.ReadFull(s, p)
		}
		wg.Done()
	})
	wg.Wait()
	return n, err
}

func (m *mockReaderSleep) Close() error {
	return nil
}

func NewTestTailer(reader io.ReadCloser, unsafeReader io.ReadCloser, cancelFunc context.CancelFunc) *Tailer {
	containerID := "1234567890abcdef"
	source := sources.NewLogSource("foo", nil)
	tailer := &Tailer{
		ContainerID: containerID,
		outputChan:  make(chan *message.Message, 100),
		decoder:     decoder.NewMockDecoder(),
		unsafeLogReader: func(ctx context.Context, t time.Time) (io.ReadCloser, error) { //nolint
			return unsafeReader, nil
		},
		Source:             source,
		tagProvider:        tag.NewLocalProvider([]string{}),
		readTimeout:        time.Millisecond,
		sleepDuration:      time.Second,
		stop:               make(chan struct{}, 1),
		stopping:           atomic.NewBool(false),
		done:               make(chan struct{}, 1),
		erroredContainerID: make(chan string, 1),
		reader:             newSafeReader(),
		readerCancelFunc:   cancelFunc,
		registry:           auditorMock.NewMockRegistry(),
	}
	tailer.reader.setUnsafeReader(reader)

	return tailer
}

func TestTailerIdentifier(t *testing.T) {
	tailer := &Tailer{ContainerID: "test"}
	assert.Equal(t, "docker:test", tailer.Identifier())
}

func TestGetLastSince(t *testing.T) {
	_time := time.Date(2008, 1, 12, 1, 1, 1, 1, time.UTC)
	tailer := &Tailer{lastSince: _time.Format(config.DateFormat)}
	assert.Equal(t, _time.Add(time.Nanosecond), tailer.getLastSince())
}

// TestBuildMessageAdvancesLastSince covers the auditor-side half of
// multi-line offset tracking: the docker tailer commits whatever
// ParsingExtra.Timestamp sits on the emitted message. Combined with the
// aggregator carrying the LAST aggregated line's timestamp on combined
// messages, this guarantees the lastSince offset advances past every line
// of a multi-line group, so a reader restart resumes after the group
// instead of replaying lines 2..N as duplicates.
func TestBuildMessageAdvancesLastSince(t *testing.T) {
	const aggregatedTimestamp = "2026-05-11T10:00:00.000000003Z"

	source := sources.NewLogSource("foo", nil)
	tailer := &Tailer{
		ContainerID: "test",
		Source:      source,
		tagProvider: tag.NewLocalProvider(nil),
	}

	output := message.NewMessageWithParsingExtra(
		[]byte("line1\\nline2\\nline3"),
		message.NewOrigin(source),
		message.StatusInfo,
		0,
		message.ParsingExtra{Timestamp: aggregatedTimestamp},
	)

	built := buildMessage(tailer, output)

	assert.Equal(t, aggregatedTimestamp, tailer.lastSince,
		"buildMessage must commit the emitted message's ParsingExtra.Timestamp to lastSince so the next reader restart resumes past the multi-line group")
	assert.Equal(t, aggregatedTimestamp, built.Origin.Offset)

	// Verify the +1ns resume offset is anchored on the LAST line's timestamp.
	expected, err := time.Parse(config.DateFormat, aggregatedTimestamp)
	assert.NoError(t, err)
	assert.Equal(t, expected.Add(time.Nanosecond), tailer.getLastSince())
}

func TestRead(t *testing.T) {
	tailer := NewTestTailer(&mockReaderNoSleep{}, nil, func() {})
	inBuf := make([]byte, 4096)

	n, err := tailer.read(inBuf, testReadTimeout)

	assert.Nil(t, err)
	assert.Equal(t, 10, n)
}

func TestReadTimeout(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	reader := newMockReaderSleep(ctx)
	// The reader should timout after testReadTimeout (10ms).
	reader.timeout = 2 * time.Second

	tailer := NewTestTailer(reader, nil, cancelFunc)
	inBuf := make([]byte, 4096)

	n, err := tailer.read(inBuf, testReadTimeout)

	assert.NotNil(t, err)
	assert.Equal(t, 0, n)
}

func TestTailerCanStopWithNilReader(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	tailer := NewTestTailer(newMockReaderSleep(ctx), nil, cancelFunc)

	// Simulate error in tailer.setupReader()
	tailer.reader = newSafeReader()
	tailer.done <- struct{}{}

	tailer.Stop()

	assert.True(t, true)
}

func TestTailer_readForever(t *testing.T) {
	tests := []struct {
		name      string
		newTailer func() *Tailer
		wantFunc  func(tailer *Tailer) error
	}{
		{
			name: "The reader has been closed during the shut down process",
			newTailer: func() *Tailer {
				_, cancelFunc := context.WithCancel(context.Background())
				reader := NewTestReader("", errors.New("http: read on closed response body"), nil)
				tailer := NewTestTailer(reader, reader, cancelFunc)
				// The reader-closed error here is the result of Stop() closing
				// the reader, so the tailer is already stopping.
				tailer.stopping.Store(true)
				return tailer
			},
			wantFunc: func(tailer *Tailer) error {
				if len(tailer.erroredContainerID) != 0 {
					return fmt.Errorf("tailer.erroredContainerID should be empty, current len: %d", len(tailer.erroredContainerID))
				}
				return nil
			},
		},
		{
			name: "The agent is stopping",
			newTailer: func() *Tailer {
				_, cancelFunc := context.WithCancel(context.Background())
				reader := NewTestReader("", errors.New("use of closed network connection"), nil)
				tailer := NewTestTailer(reader, reader, cancelFunc)
				tailer.stopping.Store(true)
				return tailer
			},
			wantFunc: func(tailer *Tailer) error {
				if len(tailer.erroredContainerID) != 0 {
					return fmt.Errorf("tailer.erroredContainerID should be empty, current len: %d", len(tailer.erroredContainerID))
				}
				return nil
			},
		},
		{
			name: "reader io.EOF error",
			newTailer: func() *Tailer {
				_, cancelFunc := context.WithCancel(context.Background())
				// init the fake reader with an io.EOF
				initialReader := NewTestReader("", io.EOF, nil)
				// then the new reader return by the unsafeReader client will return close network connection to simulate stop agent
				connectionCloseReader := NewTestReader("", errors.New("use of closed network connection"), nil)
				tailer := NewTestTailer(initialReader, connectionCloseReader, cancelFunc)
				tailer.sleepDuration = time.Millisecond
				// The EOF triggers a restart onto connectionCloseReader; the
				// connection-close that reader then returns represents the agent
				// stopping mid-restart, so readForever terminates without
				// flagging the container as errored.
				tailer.stopping.Store(true)
				return tailer
			},
			wantFunc: func(tailer *Tailer) error {
				if len(tailer.erroredContainerID) != 0 {
					return fmt.Errorf("tailer.erroredContainerID should be empty, current value: %d", len(tailer.erroredContainerID))
				}
				return nil
			},
		},
		{
			name: "default case with random error",
			newTailer: func() *Tailer {
				_, cancelFunc := context.WithCancel(context.Background())
				reader := NewTestReader("", errors.New("this is a random error"), nil)
				tailer := NewTestTailer(reader, reader, cancelFunc)
				return tailer
			},
			wantFunc: func(tailer *Tailer) error {
				if len(tailer.erroredContainerID) != 1 {
					return fmt.Errorf("tailer.erroredContainerID should contains one ID, current value: %d", len(tailer.erroredContainerID))
				}
				containerID := <-tailer.erroredContainerID
				if containerID != "1234567890abcdef" {
					return fmt.Errorf("wront containerID, current: %s", containerID)
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tailer := tt.newTailer()

			// Run readForever under a watchdog. Every case here is expected to
			// terminate (either by reconnecting onto a reader that then signals
			// a stop, or by erroring out), so if readForever does not return
			// promptly it has regressed into an infinite restart loop
			// (AGENT-16261). Fail fast with a clear message instead of letting
			// the whole package hang until the test binary's global timeout.
			done := make(chan struct{})
			go func() {
				tailer.readForever()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("readForever did not return; it likely spun in an infinite restart loop")
			}

			if err := tt.wantFunc(tailer); err != nil {
				t.Error(err)
			}
		})
	}
}

// cancelSurfacingReader blocks until its context is cancelled, modeling a read
// that is in flight when readForever's read timeout fires. When cancelled it
// returns onCancel, modeling how the moby client (which closes the HTTP body on
// context cancellation) surfaces a cancelled read as a closed-stream error
// instead of context.Canceled.
type cancelSurfacingReader struct {
	ctx      context.Context
	onCancel error
}

func (r *cancelSurfacingReader) Read(_ []byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.onCancel
}

func (r *cancelSurfacingReader) Close() error { return nil }

// TestTailer_readForeverReconnectsOnStreamCloseWhenNotStopping is the
// regression test for AGENT-16261. The moby client surfaces a read whose
// context was cancelled by our read timeout as either "http: read on closed
// response body" or "use of closed network connection". Before the fix,
// readForever returned on those errors, silently killing the tailer with no
// restart, no erroredContainerID signal, and no further "Stop tailing
// container" log. The fix makes it reconnect unless the tailer is stopping.
func TestTailer_readForeverReconnectsOnStreamCloseWhenNotStopping(t *testing.T) {
	for _, errStr := range []string{
		"http: read on closed response body", // isReaderClosed
		"use of closed network connection",   // isClosedConnError
	} {
		t.Run(errStr, func(t *testing.T) {
			_, cancelFunc := context.WithCancel(context.Background())
			reader := NewTestReader("", errors.New(errStr), nil)
			tailer := NewTestTailer(reader, reader, cancelFunc)
			tailer.sleepDuration = time.Millisecond

			// Count reconnects (tryRestartReader -> setupReader ->
			// unsafeLogReader). Allow the first reconnect to prove recovery,
			// then mark the tailer stopping so the next closed-stream error
			// terminates the loop instead of spinning forever.
			var restarts atomic.Int64
			tailer.unsafeLogReader = func(_ context.Context, _ time.Time) (io.ReadCloser, error) {
				restarts.Add(1)
				tailer.stopping.Store(true)
				return reader, nil
			}

			done := make(chan struct{})
			go func() {
				tailer.readForever()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("readForever did not return; the closed-stream error likely still bare-returns or spins")
			}

			assert.GreaterOrEqual(t, restarts.Load(), int64(1),
				"a closed-stream error while not stopping must trigger a reconnect (tryRestartReader), not a silent return")
			assert.Equal(t, 0, len(tailer.erroredContainerID),
				"a recoverable stream close must not be reported as a fatally errored container")
		})
	}
}

// TestTailer_readForeverReconnectsAfterReadTimeoutCancel reproduces the
// AGENT-16261 mechanism end-to-end at the read() boundary: a read that is in
// flight when the read timeout fires has its context cancelled, and the moby
// client surfaces that cancelled read as a closed-stream error rather than
// context.Canceled. readForever must reconnect, not die.
func TestTailer_readForeverReconnectsAfterReadTimeoutCancel(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	blocked := &cancelSurfacingReader{ctx: ctx, onCancel: errors.New("use of closed network connection")}
	restartReader := NewTestReader("", errors.New("use of closed network connection"), nil)
	tailer := NewTestTailer(blocked, restartReader, cancelFunc)
	tailer.sleepDuration = time.Millisecond

	var restarts atomic.Int64
	tailer.unsafeLogReader = func(_ context.Context, _ time.Time) (io.ReadCloser, error) {
		restarts.Add(1)
		tailer.stopping.Store(true)
		return restartReader, nil
	}

	done := make(chan struct{})
	go func() {
		tailer.readForever()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("readForever did not return after a read-timeout-driven stream close")
	}

	assert.GreaterOrEqual(t, restarts.Load(), int64(1),
		"a read cancelled by the read timeout that surfaces as a closed-stream error must trigger a reconnect")
	assert.Equal(t, 0, len(tailer.erroredContainerID))
}

func NewTestReader(data string, err, closeErr error) *testIOReadCloser { //nolint:revive
	entries := []testIOReaderEntry{
		{
			data: data,
			err:  err,
		},
	}
	return &testIOReadCloser{
		entries:  entries,
		closeErr: closeErr,
	}

}

type testIOReadCloser struct {
	entries []testIOReaderEntry
	counter int

	closeErr error
}

type testIOReaderEntry struct {
	data string
	err  error
}

func (tr *testIOReadCloser) AddEntry(data string, err error) {
	tr.entries = append(tr.entries, testIOReaderEntry{
		data: data,
		err:  err,
	})
}

func (tr *testIOReadCloser) Read(p []byte) (int, error) {
	if tr.counter >= len(tr.entries) {
		tr.counter = 0
	}
	entry := tr.entries[tr.counter]
	s := strings.NewReader(entry.data)
	n, _ := io.ReadFull(s, p)
	tr.counter++
	return n, entry.err
}

func (tr *testIOReadCloser) Close() error {
	return tr.closeErr
}

func NewTestDockerClient(reader io.ReadCloser, err error) *fakeDockerClient { //nolint:revive
	client := &fakeDockerClient{}
	client.AddEntry(reader, err)
	return client
}

type fakeDockerClient struct {
	entries []fakeDockerClientEntry
	counter int
}

type fakeDockerClientEntry struct {
	reader io.ReadCloser
	err    error
}

func (c *fakeDockerClient) AddEntry(testIOReader io.ReadCloser, err error) {
	c.entries = append(c.entries, fakeDockerClientEntry{
		reader: testIOReader,
		err:    err,
	})
}

func (c *fakeDockerClient) ContainerLogs(context.Context, string, dockerclient.ContainerLogsOptions) (io.ReadCloser, error) {
	if c.counter >= len(c.entries) {
		c.counter = 0
	}
	entry := c.entries[c.counter]
	c.counter++
	return entry.reader, entry.err
}
