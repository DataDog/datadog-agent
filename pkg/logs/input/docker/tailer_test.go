// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/tag"

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
	ctx context.Context
}

// Read mocks the Docker CLI read function
func (m *mockReaderSleep) Read(p []byte) (int, error) {
	s := strings.NewReader("Some bytes")
	var n int
	var err error
	wg := sync.WaitGroup{}
	wg.Add(1)
	time.AfterFunc(2*testReadTimeout, func() {
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

func NewTestTailer(reader io.ReadCloser, cancelFunc context.CancelFunc) *Tailer {
	containerID := "1234567890abcdef"
	source := config.NewLogSource("foo", nil)
	tailer := &Tailer{
		ContainerID:        containerID,
		outputChan:         make(chan *message.Message, 100),
		decoder:            NewTestDecoder(),
		source:             source,
		tagProvider:        tag.NoopProvider,
		cli:                nil,
		sleepDuration:      defaultSleepDuration,
		stop:               make(chan struct{}, 1),
		done:               make(chan struct{}, 1),
		erroredContainerID: make(chan string, 1),
		reader:             newSafeReader(),
		cancelFunc:         cancelFunc,
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
	tailer := NewTestTailer(&mockReaderNoSleep{}, func() {})
	inBuf := make([]byte, 4096)

	n, err := tailer.read(inBuf, testReadTimeout)

	assert.Nil(t, err)
	assert.Equal(t, 10, n)
}

func TestReadTimeout(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	tailer := NewTestTailer(&mockReaderSleep{ctx: ctx}, cancelFunc)
	inBuf := make([]byte, 4096)

	n, err := tailer.read(inBuf, testReadTimeout)

	assert.NotNil(t, err)
	assert.Equal(t, 0, n)
}

func TestTailerCanStopWithNilReader(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	tailer := NewTestTailer(&mockReaderSleep{ctx: ctx}, cancelFunc)

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
		newReader reader
		wantFunc  func(tailer *Tailer) error
	}{
		{
			name: "The reader has been closed during the shut down process",
			newTailer: func() *Tailer {
				ctx, cancelFunc := context.WithCancel(context.Background())
				reader := NewTestReader(ctx, "", fmt.Errorf("http: read on closed response body"), nil)
				tailer := NewTestTailer(reader, cancelFunc)
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
				ctx, cancelFunc := context.WithCancel(context.Background())
				reader := NewTestReader(ctx, "", fmt.Errorf("use of closed network connection"), nil)
				tailer := NewTestTailer(reader, cancelFunc)
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
				ctx, cancelFunc := context.WithCancel(context.Background())
				reader := NewTestReader(ctx, "", io.EOF, nil)
				tailer := NewTestTailer(reader, cancelFunc)
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
		{
			name: "default case with random error",
			newTailer: func() *Tailer {
				ctx, cancelFunc := context.WithCancel(context.Background())
				reader := NewTestReader(ctx, "", fmt.Errorf("this is a random error"), nil)
				tailer := NewTestTailer(reader, cancelFunc)
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

func NewTestReader(ctx context.Context, data string, err, closeErr error) io.ReadCloser {
	return &testErrorReader{
		data:     data,
		err:      err,
		closeErr: closeErr,
		ctx:      ctx,
	}

}

type testErrorReader struct {
	data     string
	err      error
	closeErr error
	ctx      context.Context
}

func (tr *testErrorReader) Read(p []byte) (int, error) {
	s := strings.NewReader(tr.data)
	n, _ := io.ReadFull(s, p)
	return n, tr.err
}

func (tr *testErrorReader) Close() error {
	return tr.closeErr
}
