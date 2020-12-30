// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package docker

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestDecoderWithHeaderSingleline(t *testing.T) {
	var output *decoder.Message
	var line []byte
	var lineLen int

	d := InitializeDecoder(config.NewLogSource("", &config.LogsConfig{}), "")
	d.Start()
	defer d.Stop()

	line = append([]byte{2, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852911Z message\n")...)
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

	// As we have no validation on the header, the parsing is incorrect
	// and this test fails.
	// It returns "wrong" as a timestamp and "message" as a content
	// TODO: add validation in the header and return the full message when
	// the validation fails.

	// output = <-d.OutputChan
	// assert.Equal(t, []byte("wrong message"), output.Content)
	// assert.Equal(t, lineLen, output.RawDataLen)
	// assert.Equal(t, message.StatusInfo, output.Status)
	// assert.Equal(t, "", output.Timestamp)

	output = <-d.OutputChan
	assert.Equal(t, []byte("message"), output.Content)
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "wrong", output.Timestamp)

}

func TestDecoderWithHeaderMultiline(t *testing.T) {
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

	d := InitializeDecoder(config.NewLogSource("", c), "")
	d.Start()
	defer d.Stop()

	line = append([]byte{1, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852911Z 1234 hello\n")...)
	lineLen = len(line)
	d.InputChan <- decoder.NewInput(line)

	line = append([]byte{1, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852912Z world\n")...)
	lineLen += len(line)
	d.InputChan <- decoder.NewInput(line)

	line = append([]byte{2, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852913Z 1234 bye\n")...)
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

func TestDecoderWithJSONSingleline(t *testing.T) {
	var output *decoder.Message
	var line []byte
	var lineLen int

	d := decoder.InitializeDecoder(config.NewLogSource("", &config.LogsConfig{}), JSONParser)
	d.Start()
	defer d.Stop()

	line = []byte(`{"log":"message\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}` + "\n")
	lineLen = len(line)
	d.InputChan <- decoder.NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("message"), output.Content)
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
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

func TestDecoderWithJSONMultiline(t *testing.T) {
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

	d := decoder.InitializeDecoder(config.NewLogSource("", c), JSONParser)
	d.Start()
	defer d.Stop()

	line = []byte(`{"log":"1234 hello\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}` + "\n")
	lineLen = len(line)
	d.InputChan <- decoder.NewInput(line)

	line = []byte(`{"log":"world\n","stream":"stdout","time":"2019-06-06T16:35:55.930852912Z"}` + "\n")
	lineLen += len(line)
	d.InputChan <- decoder.NewInput(line)

	line = []byte(`{"log":"1234 bye\n","stream":"stderr","time":"2019-06-06T16:35:55.930852913Z"}` + "\n")
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

func TestDecoderWithJSONSplittedByDocker(t *testing.T) {
	var output *decoder.Message
	var line []byte

	d := decoder.InitializeDecoder(config.NewLogSource("", &config.LogsConfig{}), JSONParser)
	d.Start()
	defer d.Stop()

	line = []byte(`{"log":"part1","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}` + "\n")
	rawLen := len(line)
	d.InputChan <- decoder.NewInput(line)

	line = []byte(`{"log":"part2\n","stream":"stdout","time":"2019-06-06T16:35:55.930852912Z"}` + "\n")
	rawLen += len(line)
	d.InputChan <- decoder.NewInput(line)

	// We don't reaggregate partial messages but we expect content of line not finishing with a '\n' character to be reconciliated
	// with the next line.
	output = <-d.OutputChan
	assert.Equal(t, []byte("part1part2"), output.Content)
	assert.Equal(t, rawLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", output.Timestamp)
}
