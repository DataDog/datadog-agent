// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package multi

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/stretchr/testify/require"
)

type testScrubber struct {
	suffix byte
}

func (c *testScrubber) ScrubFile(filePath string) ([]byte, error) {
	panic("not called")
}

func (c *testScrubber) ScrubBytes(data []byte) ([]byte, error) {
	return append(data, 'b', c.suffix), nil
}

func (c *testScrubber) ScrubLine(message string) string {
	return string(append([]byte(message), 'l', c.suffix))
}

func TestScrubBytes(t *testing.T) {
	addX := &testScrubber{'x'}
	addY := &testScrubber{'y'}
	c := NewScrubber([]scrubber.Scrubber{addX, addY})
	bytes, err := c.ScrubBytes([]byte("foo"))
	require.NoError(t, err)
	require.Equal(t, []byte("foobxby"), bytes)
}

func TestScrubLine(t *testing.T) {
	addX := &testScrubber{'x'}
	addY := &testScrubber{'y'}
	c := NewScrubber([]scrubber.Scrubber{addX, addY})
	message := c.ScrubLine("foo")
	require.Equal(t, "foolxly", message)
}
