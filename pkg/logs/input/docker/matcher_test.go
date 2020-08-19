// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build docker

package docker

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/stretchr/testify/assert"
)

func TestDecoderDetectDockerHeader(t *testing.T) {
	source := config.NewLogSource("config", &config.LogsConfig{})
	d := InitializeDecoder(source, "container1")
	d.Start()

	input := []byte("hello\n")
	input = append(input, []byte{1, 0, 0, 0, 0, 10, 0, 0}...) // docker header
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
	d.InputChan <- decoder.NewInput(input)

	var output *decoder.Message
	output = <-d.OutputChan
	assert.Equal(t, "hello", string(output.Content))

	output = <-d.OutputChan
	assert.Equal(t, "app logs", string(output.Content))
	d.Stop()
}

func TestDecoderNoNewLineBeforeDockerHeader(t *testing.T) {
	source := config.NewLogSource("config", &config.LogsConfig{})
	d := InitializeDecoder(source, "container1")
	d.Start()

	input := []byte("hello")
	input = append(input, []byte{1, 0, 0, 0, 0, 10, 0, 0}...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
	d.InputChan <- decoder.NewInput(input)

	var output *decoder.Message

	// expected output content is discarded from SingleLineHandler (line #96)
	// due to docker.parser line#80 condition not-match

	//output =<- d.OutputChan
	//expected := append([]byte("hello"), []byte{1, 0, 0, 0, 0}...)
	//assert.Equal(t, expected, output.Content)

	output = <-d.OutputChan
	assert.Equal(t, "app logs", string(output.Content))
	d.Stop()
}
