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
	"sync/atomic"
	"testing"
	"time"

	dockerclient "github.com/moby/moby/client"

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

// mockMobyReader simulates the cancelReadCloser wrapper used by
// moby/moby/client v0.4.0+: when the context is cancelled, the body is
// closed asynchronously and the blocked Read() returns
// "http: read on closed response body" rather than context.Canceled.
type mockMobyReader struct {
	ctx context.Context
}

func newMockMobyReader(ctx context.Context) *mockMobyReader {
	return &mockMobyReader{ctx: ctx}
}

func (m *mockMobyReader) Read(_ []byte) (int, error) {
	<-m.ctx.Done()
	return 0, errors.New("http: read on closed response body")
}

func (m *mockMobyReader) Close() error {
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

// TestReadReturnsCanceledOnTimeout verifies that read() synthesizes
// context.Canceled on a self-initiated timeout regardless of which error
// the underlying SDK surfaces.
//
// moby/moby/client v0.4.0+ wraps ContainerLogs' response body so that a
// context cancellation closes the body asynchronously and the blocked
// Read() returns "http: read on closed response body". Without this
// guarantee, readForever's isReaderClosed branch would match and the
// tailer would die silently after every read timeout (AGENT-16325).
func TestReadReturnsCanceledOnTimeout(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	reader := newMockMobyReader(ctx)

	tailer := NewTestTailer(reader, nil, cancelFunc)
	inBuf := make([]byte, 4096)

	n, err := tailer.read(inBuf, testReadTimeout)

	assert.Equal(t, 0, n)
	assert.Equal(t, context.Canceled, err)
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
				// Stop() would have set this before closing the reader.
				tailer.shutdownRequested.Store(true)
				return tailer
			},
			wantFunc: func(tailer *Tailer) error {
				if len(tailer.erroredContainerID) != 0 {
					return fmt.Errorf("tailer.erroredContainerID should be empty during shutdown, current len: %d", len(tailer.erroredContainerID))
				}
				return nil
			},
		},
		{
			name: "The reader closes unexpectedly (no shutdown requested)",
			newTailer: func() *Tailer {
				_, cancelFunc := context.WithCancel(context.Background())
				reader := NewTestReader("", errors.New("http: read on closed response body"), nil)
				tailer := NewTestTailer(reader, reader, cancelFunc)
				// shutdownRequested defaults to false; this is the AGENT-16325
				// silent-death path that must now signal the supervisor.
				return tailer
			},
			wantFunc: func(tailer *Tailer) error {
				if len(tailer.erroredContainerID) != 1 {
					return fmt.Errorf("tailer.erroredContainerID should contain 1 ID, got len=%d", len(tailer.erroredContainerID))
				}
				containerID := <-tailer.erroredContainerID
				if containerID != "1234567890abcdef" {
					return fmt.Errorf("wrong containerID: %s", containerID)
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
				tailer.shutdownRequested.Store(true)
				return tailer
			},
			wantFunc: func(tailer *Tailer) error {
				if len(tailer.erroredContainerID) != 0 {
					return fmt.Errorf("tailer.erroredContainerID should be empty during shutdown, current len: %d", len(tailer.erroredContainerID))
				}
				return nil
			},
		},
		{
			name: "The connection closes unexpectedly (no shutdown requested)",
			newTailer: func() *Tailer {
				_, cancelFunc := context.WithCancel(context.Background())
				reader := NewTestReader("", errors.New("use of closed network connection"), nil)
				tailer := NewTestTailer(reader, reader, cancelFunc)
				return tailer
			},
			wantFunc: func(tailer *Tailer) error {
				if len(tailer.erroredContainerID) != 1 {
					return fmt.Errorf("tailer.erroredContainerID should contain 1 ID, got len=%d", len(tailer.erroredContainerID))
				}
				containerID := <-tailer.erroredContainerID
				if containerID != "1234567890abcdef" {
					return fmt.Errorf("wrong containerID: %s", containerID)
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
				// Model the "agent stopping after EOF restart" path: by the time
				// the connection-close error surfaces, Stop() has set the flag.
				tailer.shutdownRequested.Store(true)
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
			tailer.readForever()
			if err := tt.wantFunc(tailer); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestReadForeverRestartsAfterTimeout verifies that a moby v0.4.0-style
// timeout cycle does not silently kill the tailer: each cycle synthesizes
// context.Canceled and feeds the reader-timeout branch, which calls
// tryRestartReader. The tailer keeps running across multiple cycles and
// no error is signaled to the supervisor.
func TestReadForeverRestartsAfterTimeout(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	initial := newMockMobyReader(ctx)
	tailer := NewTestTailer(initial, nil, cancelFunc)
	tailer.readTimeout = 5 * time.Millisecond
	tailer.sleepDuration = 1 * time.Millisecond

	var setupCount atomic.Int32
	tailer.unsafeLogReader = func(ctx context.Context, _ time.Time) (io.ReadCloser, error) {
		setupCount.Add(1)
		return newMockMobyReader(ctx), nil
	}

	done := make(chan struct{})
	go func() {
		tailer.readForever()
		close(done)
	}()

	// Wait for the loop to complete at least two restart cycles.
	assert.Eventually(t, func() bool {
		return setupCount.Load() >= 2
	}, 2*time.Second, 5*time.Millisecond, "expected unsafeLogReader to be called at least twice")

	// Confirm the goroutine is still alive — the bug under fix would have
	// caused it to return after the first cycle.
	select {
	case <-done:
		t.Fatal("readForever exited unexpectedly during restart cycles")
	default:
	}

	// Shut down cleanly.
	tailer.shutdownRequested.Store(true)
	tailer.stop <- struct{}{}
	tailer.readerCancelFunc()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readForever did not exit after stop")
	}

	assert.Equal(t, 0, len(tailer.erroredContainerID),
		"erroredContainerID should be empty during normal restart cycles")
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
