// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"bytes"
	"regexp"
	"sync/atomic"
	"time"

	dd_conf "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultContentLenLimit represents the max size for a line,
// if a line is bigger than this limit, it will be truncated.
const defaultContentLenLimit = 256 * 1000

// Input represents a chunk of line.
type Input struct {
	content []byte
}

// NewInput returns a new input.
func NewInput(content []byte) *Input {
	return &Input{
		content: content,
	}
}

// DecodedInput represents a decoded line and the raw length
type DecodedInput struct {
	content    []byte
	rawDataLen int
}

// NewDecodedInput returns a new decoded input.
func NewDecodedInput(content []byte, rawDataLen int) *DecodedInput {
	return &DecodedInput{
		content:    content,
		rawDataLen: rawDataLen,
	}
}

// Message represents a structured line.
type Message struct {
	Content            []byte
	Status             string
	RawDataLen         int
	Timestamp          string
	IngestionTimestamp int64
}

// NewMessage returns a new output.
func NewMessage(content []byte, status string, rawDataLen int, timestamp string) *Message {
	return &Message{
		Content:            content,
		Status:             status,
		RawDataLen:         rawDataLen,
		Timestamp:          timestamp,
		IngestionTimestamp: time.Now().UnixNano(),
	}
}

// Decoder splits raw data into lines and passes them to a lineParser that passes them to
// a lineHandler that emits outputs
// Input->[decoder]->[parser]->[handler]->Message
type Decoder struct {
	// The number of raw lines decoded from the input before they are processed.
	// Needs to be first to ensure 64 bit alignment
	linesDecoded int64

	InputChan       chan *Input
	OutputChan      chan *Message
	matcher         EndLineMatcher
	lineBuffer      *bytes.Buffer
	lineParser      LineParser
	contentLenLimit int
	rawDataLen      int

	// The decoder holds on to an instace of DetectedPattern which is a thread safe container used to
	// pass a multiline pattern up from the line handler in order to surface it to the tailer.
	// The tailer uses this to determine if a pattern should be reused when a file rotates.
	detectedPattern *DetectedPattern
}

// InitializeDecoder returns a properly initialized Decoder
func InitializeDecoder(source *config.LogSource, parser parser.Parser) *Decoder {
	return NewDecoderWithEndLineMatcher(source, parser, &NewLineMatcher{}, nil)
}

// NewDecoderWithEndLineMatcher initialize a decoder with given endline strategy.
func NewDecoderWithEndLineMatcher(source *config.LogSource, parser parser.Parser, matcher EndLineMatcher, multiLinePattern *regexp.Regexp) *Decoder {
	inputChan := make(chan *Input)
	outputChan := make(chan *Message)
	lineLimit := defaultContentLenLimit
	var lineHandler LineHandler
	var lineParser LineParser
	detectedPattern := &DetectedPattern{}

	for _, rule := range source.Config.ProcessingRules {
		if rule.Type == config.MultiLine {
			lh := NewMultiLineHandler(outputChan, rule.Regex, config.AggregationTimeout(), lineLimit)

			// Since a single source can have multiple file tailers - each with their own decoder instance,
			// Make sure we keep track of the multiline match count info from all of the decoders so the
			// status page displays it correctly.
			if existingInfo, ok := source.GetInfo(lh.countInfo.InfoKey()).(*config.CountInfo); ok {
				// override the new decoders info to the instance we are already using
				lh.countInfo = existingInfo
			} else {
				// this is the first decoder we have seen for this source - use it's count info
				source.RegisterInfo(lh.countInfo)
			}
			lineHandler = lh
		}
	}
	if lineHandler == nil {
		if dd_conf.Datadog.GetBool("logs_config.auto_multi_line_detection") || source.Config.AutoMultiLine {
			log.Infof("Auto multi line log detection enabled")

			if multiLinePattern != nil {
				log.Info("Found a previously detected pattern - using multiline handler")

				// Save the pattern again for the next rotation
				detectedPattern.Set(multiLinePattern)

				lineHandler = NewMultiLineHandler(outputChan, multiLinePattern, config.AggregationTimeout(), lineLimit)
			} else {
				lineHandler = buildAutoMultilineHandlerFromConfig(outputChan, lineLimit, source, detectedPattern)
			}
		} else {
			lineHandler = NewSingleLineHandler(outputChan, lineLimit)
		}
	}

	if parser.SupportsPartialLine() {
		lineParser = NewMultiLineParser(config.AggregationTimeout(), parser, lineHandler, lineLimit)
	} else {
		lineParser = NewSingleLineParser(parser, lineHandler)
	}

	return New(inputChan, outputChan, lineParser, lineLimit, matcher, detectedPattern)
}

func buildAutoMultilineHandlerFromConfig(outputChan chan *Message, lineLimit int, source *config.LogSource, detectedPattern *DetectedPattern) *AutoMultilineHandler {
	linesToSample := source.Config.AutoMultiLineSampleSize
	if linesToSample <= 0 {
		linesToSample = dd_conf.Datadog.GetInt("logs_config.auto_multi_line_default_sample_size")
	}
	matchThreshold := source.Config.AutoMultiLineMatchThreshold
	if matchThreshold == 0 {
		matchThreshold = dd_conf.Datadog.GetFloat64("logs_config.auto_multi_line_default_match_threshold")
	}
	additionalPatterns := dd_conf.Datadog.GetStringSlice("logs_config.auto_multi_line_extra_patterns")
	additionalPatternsCompiled := []*regexp.Regexp{}

	for _, p := range additionalPatterns {
		compiled, err := regexp.Compile("^" + p)
		if err != nil {
			log.Warn("logs_config.auto_multi_line_extra_patterns containing value: ", p, " is not a valid regular expression")
			continue
		}
		additionalPatternsCompiled = append(additionalPatternsCompiled, compiled)
	}

	matchTimeout := time.Second * dd_conf.Datadog.GetDuration("logs_config.auto_multi_line_default_match_timeout")
	return NewAutoMultilineHandler(outputChan,
		lineLimit,
		linesToSample,
		matchThreshold,
		matchTimeout,
		config.AggregationTimeout(),
		source,
		additionalPatternsCompiled,
		detectedPattern)
}

// New returns an initialized Decoder
func New(InputChan chan *Input, OutputChan chan *Message, lineParser LineParser, contentLenLimit int, matcher EndLineMatcher, detectedPattern *DetectedPattern) *Decoder {
	var lineBuffer bytes.Buffer
	return &Decoder{
		InputChan:       InputChan,
		OutputChan:      OutputChan,
		lineBuffer:      &lineBuffer,
		lineParser:      lineParser,
		contentLenLimit: contentLenLimit,
		matcher:         matcher,
		detectedPattern: detectedPattern,
	}
}

// Start starts the Decoder
func (d *Decoder) Start() {
	d.lineParser.Start()
	go d.run()
}

// Stop stops the Decoder
func (d *Decoder) Stop() {
	close(d.InputChan)
}

// GetLineCount returns the number of decoded lines
func (d *Decoder) GetLineCount() int64 {
	return atomic.LoadInt64(&d.linesDecoded)
}

// GetDetectedPattern returns a detected pattern (if any)
func (d *Decoder) GetDetectedPattern() *regexp.Regexp {
	if d.detectedPattern == nil {
		return nil
	}
	return d.detectedPattern.Get()
}

// run lets the Decoder handle data coming from InputChan
func (d *Decoder) run() {
	for data := range d.InputChan {
		d.decodeIncomingData(data.content)
	}
	// finish to stop decoder
	d.lineParser.Stop()
}

// decodeIncomingData splits raw data based on '\n', creates and processes new lines
func (d *Decoder) decodeIncomingData(inBuf []byte) {
	i, j := 0, 0
	n := len(inBuf)
	maxj := d.contentLenLimit - d.lineBuffer.Len()

	for ; j < n; j++ {
		if j == maxj {
			// send line because it is too long
			d.lineBuffer.Write(inBuf[i:j])
			d.rawDataLen += (j - i)
			d.sendLine()
			i = j
			maxj = i + d.contentLenLimit
		} else if d.matcher.Match(d.lineBuffer.Bytes(), inBuf, i, j) {
			d.lineBuffer.Write(inBuf[i:j])
			d.rawDataLen += (j - i)
			d.rawDataLen++ // account for the matching byte
			d.sendLine()
			i = j + 1 // skip the last bytes of the matched sequence
			maxj = i + d.contentLenLimit
		}
	}
	d.lineBuffer.Write(inBuf[i:j])
	d.rawDataLen += (j - i)
}

// sendLine copies content from lineBuffer which is passed to lineHandler
func (d *Decoder) sendLine() {
	// Account for longer-than-1-byte line separator
	content := make([]byte, d.lineBuffer.Len()-(d.matcher.SeparatorLen()-1))
	copy(content, d.lineBuffer.Bytes())
	d.lineBuffer.Reset()
	d.lineParser.Handle(NewDecodedInput(content, d.rawDataLen))
	d.rawDataLen = 0
	atomic.AddInt64(&d.linesDecoded, 1)
}
