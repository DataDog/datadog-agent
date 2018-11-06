// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
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

type mockReaderSleep struct{}

func (m *mockReaderSleep) Read(p []byte) (int, error) {
	s := strings.NewReader("Some bytes")
	time.Sleep(2 * testReadTimeout)
	n, _ := io.ReadFull(s, p)
	return n, nil
}

func (m *mockReaderSleep) Close() error {
	return nil
}

func NewTestTailer(reader io.ReadCloser) *Tailer {
	return &Tailer{
		ContainerID:        "1234567890abcdef",
		outputChan:         make(chan *message.Message, 100),
		decoder:            nil,
		source:             nil,
		cli:                nil,
		sleepDuration:      defaultSleepDuration,
		stop:               make(chan struct{}, 1),
		done:               make(chan struct{}, 1),
		erroredContainerID: make(chan string, 100),
		reader:             reader,
	}
}

func TestTailerIdentifier(t *testing.T) {
	tailer := &Tailer{ContainerID: "test"}
	assert.Equal(t, "docker:test", tailer.Identifier())
}

func TestRead(t *testing.T) {
	tailer := NewTestTailer(&mockReaderNoSleep{})
	inBuf := make([]byte, 4096)

	n, err := tailer.read(inBuf, testReadTimeout)

	assert.Nil(t, err)
	assert.Equal(t, 10, n)
}

func TestReadTimeout(t *testing.T) {
	tailer := NewTestTailer(&mockReaderSleep{})
	inBuf := make([]byte, 4096)

	n, err := tailer.read(inBuf, testReadTimeout)

	assert.NotNil(t, err)
	assert.Equal(t, 0, n)
}
