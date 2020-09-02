// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build docker

package docker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/stretchr/testify/assert"
)

func getDummyHeader(i int) []byte {
	hdr := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	hdr[i] = 10
	return hdr
}

func TestDecoderDetectDockerHeader(t *testing.T) {
	source := config.NewLogSource("config", &config.LogsConfig{})
	d := InitializeDecoder(source, "container1")
	d.Start()

	for i := 4; i < 8; i++ {
		input := []byte("hello\n")
		input = append(input, getDummyHeader(i)...) // docker header
		input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
		d.InputChan <- decoder.NewInput(input)

		var output *decoder.Output
		output = <-d.OutputChan
		assert.Equal(t, "hello", string(output.Content))

		output = <-d.OutputChan
		assert.Equal(t, "app logs", string(output.Content))
	}
	d.Stop()
}

func TestDecoderDetectMultipleDockerHeader(t *testing.T) {
	source := config.NewLogSource("config", &config.LogsConfig{})
	d := InitializeDecoder(source, "container1")
	d.Start()

	var input []byte
	for i := 0; i < 100; i++ {
		input = append(input, getDummyHeader(4+i%4)...) // docker header
		input = append(input, []byte(fmt.Sprintf("2018-06-14T18:27:03.246999277Z app logs %d\n", i))...)
	}
	d.InputChan <- decoder.NewInput(input)

	var output *decoder.Output
	for i := 0; i < 100; i++ {
		output = <-d.OutputChan
		assert.Equal(t, fmt.Sprintf("app logs %d", i), string(output.Content))
	}

	d.Stop()
}

func TestDecoderDetectMultipleDockerHeaderOnAChunkedLine(t *testing.T) {
	source := config.NewLogSource("config", &config.LogsConfig{})
	longestChunk := strings.Repeat("A", 16384)
	d := InitializeDecoder(source, "container1")
	d.Start()

	var input []byte
	input = append(input, getDummyHeader(5)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z "+longestChunk)...)
	input = append(input, getDummyHeader(6)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z "+longestChunk)...)
	input = append(input, getDummyHeader(7)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z the end\n")...)
	input = append(input, getDummyHeader(5)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z "+longestChunk)...)
	input = append(input, getDummyHeader(6)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z "+longestChunk)...)
	input = append(input, getDummyHeader(7)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z the very end\n")...)

	d.InputChan <- decoder.NewInput(input)

	var output *decoder.Output
	output = <-d.OutputChan
	assert.Equal(t, fmt.Sprintf(longestChunk+longestChunk+"the end"), string(output.Content))
	output = <-d.OutputChan
	assert.Equal(t, fmt.Sprintf(longestChunk+longestChunk+"the very end"), string(output.Content))

	d.Stop()
}

func TestDecoderNoNewLineBeforeDockerHeader(t *testing.T) {
	source := config.NewLogSource("config", &config.LogsConfig{})
	d := InitializeDecoder(source, "container1")
	d.Start()
	for i := 4; i < 8; i++ {
		input := []byte("hello")
		input = append(input, getDummyHeader(i)...) // docker header
		input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
		d.InputChan <- decoder.NewInput(input)

		var output *decoder.Output
		output = <-d.OutputChan
		assert.Equal(t, "app logs", string(output.Content))
	}
	d.Stop()
}
