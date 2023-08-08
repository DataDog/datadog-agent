// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/tag"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"

	"github.com/docker/docker/api/types"

	"github.com/stretchr/testify/assert"
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

func NewTestDecoder() *decoder.Decoder {
	return &decoder.Decoder{
		InputChan: make(chan *decoder.Input),
	}
}

func NewTestTailer(reader io.ReadCloser, dockerClient *fakeDockerClient, cancelFunc context.CancelFunc) *Tailer {
	containerID := "1234567890abcdef"
	source := sources.NewLogSource("foo", nil)
	tailer := &Tailer{
		ContainerID:        containerID,
		outputChan:         make(chan *message.Message, 100),
		decoder:            NewTestDecoder(),
		Source:             source,
		tagProvider:        tag.NewLocalProvider([]string{}),
		dockerutil:         dockerClient,
		readTimeout:        time.Millisecond,
		sleepDuration:      time.Second,
		stop:               make(chan struct{}, 1),
		done:               make(chan struct{}, 1),
		erroredContainerID: make(chan string, 1),
		reader:             newSafeReader(),
		readerCancelFunc:   cancelFunc,
	}
	tailer.reader.setUnsafeReader(reader)

	return tailer
}

func TestTailerIdentifier(t *testing.T) {
	tailer := &Tailer{ContainerID: "test"}
	assert.Equal(t, "docker:test", tailer.Identifier())
}

func TestGetLastSince(t *testing.T) {
	tailer := &Tailer{lastSince: "2008-01-12T01:01:01.000000001Z"}
	assert.Equal(t, "2008-01-12T01:01:01.000000002Z", tailer.getLastSince())
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
				reader := NewTestReader("", fmt.Errorf("http: read on closed response body"), nil)
				dockerClient := NewTestDockerClient(reader, nil)
				tailer := NewTestTailer(reader, dockerClient, cancelFunc)
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
				reader := NewTestReader("", fmt.Errorf("use of closed network connection"), nil)
				dockerClient := NewTestDockerClient(reader, nil)
				tailer := NewTestTailer(reader, dockerClient, cancelFunc)
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
				// then the new reader return by the docker client will return close network connection to simulate stop agent
				connectionCloseReader := NewTestReader("", fmt.Errorf("use of closed network connection"), nil)
				dockerClient := NewTestDockerClient(connectionCloseReader, nil)
				tailer := NewTestTailer(initialReader, dockerClient, cancelFunc)
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
				reader := NewTestReader("", fmt.Errorf("this is a random error"), nil)
				dockerClient := NewTestDockerClient(reader, nil)
				tailer := NewTestTailer(reader, dockerClient, cancelFunc)
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

func (c *fakeDockerClient) ContainerLogs(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
	if c.counter >= len(c.entries) {
		c.counter = 0
	}
	entry := c.entries[c.counter]
	c.counter++
	return entry.reader, entry.err
}
