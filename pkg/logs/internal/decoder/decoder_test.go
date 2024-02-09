// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/framer"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/dockerfile"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/dockerstream"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/encodedtext"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"

	"github.com/stretchr/testify/assert"
)

func InitializeDecoderForTest(source *sources.LogSource, parser parsers.Parser) *Decoder {
	info := status.NewInfoRegistry()
	return InitializeDecoder(sources.NewReplaceableSource(source), parser, info)
}

func TestDecoderWithDockerHeader(t *testing.T) {
	source := sources.NewLogSource("config", &config.LogsConfig{})
	d := InitializeDecoderForTest(source, noop.New())
	d.Start()

	input := []byte("hello\n")
	input = append(input, []byte{1, 0, 0, 0, 0, 10, 0, 0}...) // docker header
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
	d.InputChan <- NewInput(input)

	var output *message.Message
	output = <-d.OutputChan
	assert.Equal(t, "hello", string(output.GetContent()))
	assert.Equal(t, len("hello")+1, output.RawDataLen)

	output = <-d.OutputChan
	expected := []byte{1, 0, 0, 0, 0}
	assert.Equal(t, expected, output.GetContent())
	assert.Equal(t, 6, output.RawDataLen)

	output = <-d.OutputChan
	expected = append([]byte{0, 0}, []byte("2018-06-14T18:27:03.246999277Z app logs")...)
	assert.Equal(t, expected, output.GetContent())
	assert.Equal(t, len(expected)+1, output.RawDataLen)

	d.Stop()
}

func TestDecoderWithDockerHeaderSingleline(t *testing.T) {
	var output *message.Message
	var line []byte
	var lineLen int

	d := InitializeDecoderForTest(sources.NewLogSource("", &config.LogsConfig{}), dockerstream.New("abc123"))
	d.Start()
	defer d.Stop()

	line = append([]byte{2, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852911Z message\n")...)
	lineLen = len(line)
	d.InputChan <- NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.ParsingExtra.Timestamp)

	line = []byte("wrong message\n")
	lineLen = len(line)
	d.InputChan <- NewInput(line)

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
	assert.Equal(t, []byte("message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "wrong", output.ParsingExtra.Timestamp)

}

func TestDecoderWithDockerHeaderMultiline(t *testing.T) {
	var output *message.Message
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

	d := InitializeDecoderForTest(sources.NewLogSource("", c), dockerstream.New("abc123"))
	d.Start()
	defer d.Stop()

	line = append([]byte{1, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852911Z 1234 hello\n")...)
	lineLen = len(line)
	d.InputChan <- NewInput(line)

	line = append([]byte{1, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852912Z world\n")...)
	lineLen += len(line)
	d.InputChan <- NewInput(line)

	line = append([]byte{2, 0, 0, 0, 0, 0, 0, 0}, []byte("2019-06-06T16:35:55.930852913Z 1234 bye\n")...)
	d.InputChan <- NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("1234 hello\\nworld"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", output.ParsingExtra.Timestamp)

	lineLen = len(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("1234 bye"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", output.ParsingExtra.Timestamp)
}

func TestDecoderWithDockerJSONSingleline(t *testing.T) {
	var output *message.Message
	var line []byte
	var lineLen int

	d := InitializeDecoderForTest(sources.NewLogSource("", &config.LogsConfig{}), dockerfile.New())
	d.Start()
	defer d.Stop()

	line = []byte(`{"log":"message\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}` + "\n")
	lineLen = len(line)
	d.InputChan <- NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.ParsingExtra.Timestamp)

	line = []byte("wrong message\n")
	lineLen = len(line)
	d.InputChan <- NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("wrong message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "", output.ParsingExtra.Timestamp)
}

func TestDecoderWithDockerJSONMultiline(t *testing.T) {
	var output *message.Message
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

	d := InitializeDecoderForTest(sources.NewLogSource("", c), dockerfile.New())
	d.Start()
	defer d.Stop()

	line = []byte(`{"log":"1234 hello\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}` + "\n")
	lineLen = len(line)
	d.InputChan <- NewInput(line)

	line = []byte(`{"log":"world\n","stream":"stdout","time":"2019-06-06T16:35:55.930852912Z"}` + "\n")
	lineLen += len(line)
	d.InputChan <- NewInput(line)

	line = []byte(`{"log":"1234 bye\n","stream":"stderr","time":"2019-06-06T16:35:55.930852913Z"}` + "\n")
	d.InputChan <- NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("1234 hello\\nworld"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", output.ParsingExtra.Timestamp)

	lineLen = len(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("1234 bye"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", output.ParsingExtra.Timestamp)
}

func TestDecoderWithDockerJSONSplittedByDocker(t *testing.T) {
	var output *message.Message
	var line []byte

	d := InitializeDecoderForTest(sources.NewLogSource("", &config.LogsConfig{}), dockerfile.New())
	d.Start()
	defer d.Stop()

	line = []byte(`{"log":"part1","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}` + "\n")
	rawLen := len(line)
	d.InputChan <- NewInput(line)

	line = []byte(`{"log":"part2\n","stream":"stdout","time":"2019-06-06T16:35:55.930852912Z"}` + "\n")
	rawLen += len(line)
	d.InputChan <- NewInput(line)

	// We don't reaggregate partial messages but we expect content of line not finishing with a '\n' character to be reconciliated
	// with the next line.
	output = <-d.OutputChan
	assert.Equal(t, []byte("part1part2"), output.GetContent())
	assert.Equal(t, rawLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", output.ParsingExtra.Timestamp)
}

func TestDecoderWithDecodingParser(t *testing.T) {
	source := sources.NewLogSource("config", &config.LogsConfig{})

	info := status.NewInfoRegistry()
	d := NewDecoderWithFraming(sources.NewReplaceableSource(source), encodedtext.New(encodedtext.UTF16LE), framer.UTF16LENewline, nil, info)
	d.Start()

	input := []byte{'h', 0x0, 'e', 0x0, 'l', 0x0, 'l', 0x0, 'o', 0x0, '\n', 0x0}
	d.InputChan <- NewInput(input)

	var output *message.Message
	output = <-d.OutputChan
	assert.Equal(t, "hello", string(output.GetContent()))
	assert.Equal(t, len(input), output.RawDataLen)

	// Test with BOM
	input = []byte{0xFF, 0xFE, 'h', 0x0, 'e', 0x0, 'l', 0x0, 'l', 0x0, 'o', 0x0, '\n', 0x0}
	d.InputChan <- NewInput(input)

	output = <-d.OutputChan
	assert.Equal(t, "hello", string(output.GetContent()))
	assert.Equal(t, len(input), output.RawDataLen)

	d.Stop()
}

func TestDecoderWithSinglelineKubernetes(t *testing.T) {
	var output *message.Message
	var line []byte
	var lineLen int

	d := InitializeDecoderForTest(sources.NewLogSource("", &config.LogsConfig{}), kubernetes.New())
	d.Start()
	defer d.Stop()

	line = []byte("2019-06-06T16:35:55.930852911Z stderr F message\n")
	lineLen = len(line)
	d.InputChan <- NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", output.ParsingExtra.Timestamp)

	line = []byte("wrong message\n")
	lineLen = len(line)
	d.InputChan <- NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("wrong message"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "", output.ParsingExtra.Timestamp)
}

func TestDecoderWithMultilineKubernetes(t *testing.T) {
	var output *message.Message
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
	d := InitializeDecoderForTest(sources.NewLogSource("", c), kubernetes.New())
	d.Start()
	defer d.Stop()

	line = []byte("2019-06-06T16:35:55.930852911Z stdout F 1234 hello\n")
	lineLen = len(line)
	d.InputChan <- NewInput(line)

	line = []byte("2019-06-06T16:35:55.930852912Z stdout F world\n")
	lineLen += len(line)
	d.InputChan <- NewInput(line)

	line = []byte("2019-06-06T16:35:55.930852913Z stderr F 1234 bye\n")
	d.InputChan <- NewInput(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("1234 hello\\nworld"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusInfo, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", output.ParsingExtra.Timestamp)

	lineLen = len(line)

	output = <-d.OutputChan
	assert.Equal(t, []byte("1234 bye"), output.GetContent())
	assert.Equal(t, lineLen, output.RawDataLen)
	assert.Equal(t, message.StatusError, output.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", output.ParsingExtra.Timestamp)
}
