// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
	"github.com/stretchr/testify/assert"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestDecoderLineExceedMax(t *testing.T) {
	source := config.NewLogSource("config", &config.LogsConfig{})
	de := NewDecoder(source)
	de.Start()
	input := strings.Repeat("a ", 2048)
	de.InputChan <- decoder.NewInput(append([]byte("2018-09-20T11:54:11.753589172Z stdout F "), []byte(input)...))
	for i := 0; i < 255; i++ {
		de.InputChan <- decoder.NewInput([]byte(strings.Repeat("a ", 2048)))
	}
	de.InputChan <- decoder.NewInput([]byte("end\n"))
	expectedRawDataLen := 1048579 // len("a ") * 2048 * 256 + len("end")
	for i := 0; i < 3; i++ {
		output := <-de.OutputChan
		assert.Equal(t, "2018-09-20T11:54:11.753589172Z", output.Timestamp)
		assert.Equal(t, "info", output.Status)
		assert.Equal(t, 256*1024, output.RawDataLen)
		expectedRawDataLen -= output.RawDataLen
	}
	output := <-de.OutputChan
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
	assert.Equal(t, 256*1024-40, output.RawDataLen)
	expectedRawDataLen -= output.RawDataLen

	output = <-de.OutputChan
	assert.Equal(t, "...TRUNCATED...a a a a a a a a a a a a a a a a a a a a end", string(output.Content))
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
	assert.Equal(t, expectedRawDataLen, output.RawDataLen)
	assert.Equal(t, 43, output.RawDataLen)
	de.Stop()
}

func TestDecoderWithSingleLineHandler(t *testing.T) {
	source := config.NewLogSource("config", &config.LogsConfig{})
	de := NewDecoder(source)
	de.Start()
	de.InputChan <- decoder.NewInput([]byte("2018-09-20T11:54:11.753589172Z stdout P 1.first line\n continue...\n2018-09-20T11:54:11.753589182Z stderr F 2.second line\n"))

	output := <-de.OutputChan
	assert.Equal(t, "1.first line", string(output.Content))
	assert.Equal(t, len("1.first line"), output.RawDataLen)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)

	output = <-de.OutputChan
	assert.Equal(t, " continue...", string(output.Content))
	assert.Equal(t, len(" continue..."), output.RawDataLen)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)

	output = <-de.OutputChan
	assert.Equal(t, "2.second line", string(output.Content))
	assert.Equal(t, len("2.second line"), output.RawDataLen)
	assert.Equal(t, "2018-09-20T11:54:11.753589182Z", output.Timestamp)
	assert.Equal(t, "error", output.Status)
	de.Stop()
}

func TestDecoderWithMultiLineHandler(t *testing.T) {
	source := config.NewLogSource(
		"config",
		&config.LogsConfig{
			ProcessingRules: []*config.ProcessingRule{
				{
					Type:  config.MultiLine,
					Regex: regexp.MustCompile("[0-9]+\\."),
				},
			},
		})
	de := NewDecoder(source)
	de.Start()
	de.InputChan <- decoder.NewInput([]byte("2018-09-20T11:54:11.753589172Z stdout P 1.first line\n continue...\n2018-09-20T11:54:11.753589182Z stderr F 2.second line\n"))

	output := <-de.OutputChan
	assert.Equal(t, "1.first line\\n continue...", string(output.Content))
	assert.Equal(t, len("1.first line\\n continue..."), output.RawDataLen)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)

	output = <-de.OutputChan
	assert.Equal(t, "2.second line", string(output.Content))
	assert.Equal(t, len("2.second line"), output.RawDataLen)
	assert.Equal(t, "2018-09-20T11:54:11.753589182Z", output.Timestamp)
	assert.Equal(t, "error", output.Status)
	de.Stop()
}

func TestLineGenerator(t *testing.T) {
	inputChan := make(chan *decoder.Input)
	outputChan := make(chan *decoder.Output)
	truncator := decoder.NewLineTruncator(outputChan, 60)
	flushTimeout := 10 * time.Millisecond
	handler := decoder.NewMultiHandler(regexp.MustCompile("[0-9]+\\."), *truncator)
	scheduler := decoder.NewLineHandlerScheduler(make(chan *decoder.RichLine), flushTimeout, handler)
	generator := decoder.NewLineGenerator(600, inputChan, &decoder.NewLineMatcher{}, &Convertor{}, *scheduler)

	generator.Start()
	inputChan <- decoder.NewInput([]byte("2018-09-20T11:54:11.753589172Z stdout P 1.first lin"))
	inputChan <- decoder.NewInput([]byte("e\n2018-09-20T11:54:11.753589173Z stdout P end of m"))
	inputChan <- decoder.NewInput([]byte("ulti line\n2018-09-20T11:54:11.753589174Z stderr F "))
	inputChan <- decoder.NewInput([]byte("2.second line\n"))

	output := <-outputChan
	assert.Equal(t, "1.first line\\nend of multi line", string(output.Content))
	assert.Equal(t, len("1.first line\\nend of multi line"), output.RawDataLen)
	assert.Equal(t, "2018-09-20T11:54:11.753589173Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)

	output = <-outputChan
	assert.Equal(t, "2.second line", string(output.Content))
	assert.Equal(t, len("2.second line"), output.RawDataLen)
	assert.Equal(t, "2018-09-20T11:54:11.753589174Z", output.Timestamp)
	assert.Equal(t, "error", output.Status)

	inputChan <- decoder.NewInput([]byte("2018-09-20T11:54:11.753589172Z stdout P 2018-09-20T11:54:11.753589173Z stdout P msg\n"))
	output = <-outputChan
	assert.Equal(t, "2018-09-20T11:54:11.753589173Z stdout P msg", string(output.Content))
	assert.Equal(t, len("2018-09-20T11:54:11.753589173Z stdout P msg"), output.RawDataLen)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)

	inputChan <- decoder.NewInput([]byte("2018-09-20T11:54:11.753589171Z stdout P \n2018-09-20T11:54:11.753589174Z stdout P msg\n"))
	output = <-outputChan
	assert.Equal(t, "msg", string(output.Content))
	assert.Equal(t, len("msg"), output.RawDataLen)
	assert.Equal(t, "2018-09-20T11:54:11.753589174Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)

	inputChan <- decoder.NewInput([]byte("2018-09-20T11:54:11.753589171Z stdout H \n2018-09-20T11:54:11.753589175Z stdout P 1.msg\n"))
	output = <-outputChan
	assert.Equal(t, "2018-09-20T11:54:11.753589171Z stdout H ", string(output.Content))
	assert.Equal(t, len("2018-09-20T11:54:11.753589171Z stdout H "), output.RawDataLen)
	assert.Equal(t, "2018-09-20T11:54:11.753589174Z", output.Timestamp) // take previous prefix
	assert.Equal(t, "info", output.Status)
	output = <-outputChan
	assert.Equal(t, "1.msg", string(output.Content))
	assert.Equal(t, len("1.msg"), output.RawDataLen)
	assert.Equal(t, "2018-09-20T11:54:11.753589175Z", output.Timestamp)
	assert.Equal(t, "info", output.Status)
}
