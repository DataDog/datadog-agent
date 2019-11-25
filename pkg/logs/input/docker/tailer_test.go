// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
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

func NewTestTailer(reader io.ReadCloser, cancelFunc context.CancelFunc) *Tailer {
	tailer := &Tailer{
		ContainerID:   "1234567890abcdef",
		outputChan:    make(chan *message.Message, 100),
		decoder:       nil,
		source:        config.NewLogSource("foo", nil),
		tagProvider:   tag.NoopProvider,
		cli:           nil,
		sleepDuration: defaultSleepDuration,
		stop:          make(chan struct{}, 1),
		done:          make(chan struct{}, 1),
		reader:        newSafeReader(),
		cancelFunc:    cancelFunc,
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
