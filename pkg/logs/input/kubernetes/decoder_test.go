// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package kubernetes

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestDecoderWithSingleline(t *testing.T) {
	var output *decoder.Message
	var line []byte
	var lineLen int

	d := decoder.InitializeDecoder(config.NewLogSource("", &config.LogsConfig{}), Parser)
	d.Start()
	defer d.Stop()

	line = []byte("2019-06-06T16:35:55.930852911Z stderr F message\n")
	lineLen = len(line)
	d.InputChan <- decoder.NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("message"), output.Content)
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.Timestamp)

	line = []byte("wrong message\n")
	lineLen = len(line)
	d.InputChan <- decoder.NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("wrong message"), output.Content)
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "", output.Timestamp)
}

func TestDecoderWithMultiline(t *testing.T) {
	var output *decoder.Message
	var line []byte
	var lineLen int

	c := &config.LogsConfig{
		ProcessingRules: []*config.ProcessingRule{
			{
				Type:  config.MultiLine,
				Regex: regexp.MustCompile("1234"),
			},
		},
	}
	d := decoder.InitializeDecoder(config.NewLogSource("", c), Parser)
	d.Start()
	defer d.Stop()

	line = []byte("2019-06-06T16:35:55.930852911Z stdout F 1234 hello\n")
	lineLen = len(line)
	d.InputChan <- decoder.NewInput(line)

	line = []byte("2019-06-06T16:35:55.930852912Z stdout F world\n")
	lineLen += len(line)
	d.InputChan <- decoder.NewInput(line)

	line = []byte("2019-06-06T16:35:55.930852913Z stderr F 1234 bye\n")
	d.InputChan <- decoder.NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("1234 hello\\nworld"), output.Content)
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", output.Timestamp)

	lineLen = len(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("1234 bye"), output.Content)
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", output.Timestamp)
}
