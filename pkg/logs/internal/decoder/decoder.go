// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"regexp"
	"time"

	dd_conf "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/framer"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
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

// Decoder translates a sequence of byte buffers (such as from a file or a
// network socket) into log messages.
//
// Decoder is structured as an actor with InputChan of type *decoder.Input and
// OutputChan of type *decoder.Message.
//
// The Decoder's run() takes data from InputChan, uses a Framer to break it into frames.
// The LineBreaker passes that data to a LineParser, which uses a Parser to convert it to
// parsers.Message, converts that to decoder.Message, and passes that to the LineHandler.
//
// The LineHandler processes the messages it as necessary (as single lines,
// multiple lines, or auto-detecting the two), and sends the result to the
// Decoder's output channel.
type Decoder struct {
	InputChan  chan *Input
	OutputChan chan *Message

	framer      *framer.Framer
	lineParser  LineParser
	lineHandler LineHandler

	// The decoder holds on to an instace of DetectedPattern which is a thread safe container used to
	// pass a multiline pattern up from the line handler in order to surface it to the tailer.
	// The tailer uses this to determine if a pattern should be reused when a file rotates.
	detectedPattern *DetectedPattern
}

// InitializeDecoder returns a properly initialized Decoder
func InitializeDecoder(source *sources.ReplaceableSource, parser parsers.Parser) *Decoder {
	return NewDecoderWithFraming(source, parser, framer.UTF8Newline, nil)
}

// Since a single source can have multiple file tailers - each with their own decoder instance:
// make sure we sync info providers from all of the decoders so the status page displays it correctly.
func syncSourceInfo(source *sources.ReplaceableSource, lh *MultiLineHandler) {
	if existingInfo, ok := source.GetInfo(lh.countInfo.InfoKey()).(*status.CountInfo); ok {
		// override the new decoders info to the instance we are already using
		lh.countInfo = existingInfo
	} else {
		// this is the first decoder we have seen for this source - use it's count info
		source.RegisterInfo(lh.countInfo)
	}
	// Same as above for linesCombinedInfo
	if existingInfo, ok := source.GetInfo(lh.linesCombinedInfo.InfoKey()).(*status.CountInfo); ok {
		lh.linesCombinedInfo = existingInfo
	} else {
		source.RegisterInfo(lh.linesCombinedInfo)
	}
}

// NewDecoderWithFraming initialize a decoder with given endline strategy.
func NewDecoderWithFraming(source *sources.ReplaceableSource, parser parsers.Parser, framing framer.Framing, multiLinePattern *regexp.Regexp) *Decoder {
	inputChan := make(chan *Input)
	outputChan := make(chan *Message)
	lineLimit := defaultContentLenLimit
	detectedPattern := &DetectedPattern{}

	outputFn := func(m *Message) { outputChan <- m }

	// construct the lineHandler
	var lineHandler LineHandler
	for _, rule := range source.Config().ProcessingRules {
		if rule.Type == config.MultiLine {
			lh := NewMultiLineHandler(outputFn, rule.Regex, config.AggregationTimeout(), lineLimit, false)
			syncSourceInfo(source, lh)
			lineHandler = lh
		}
	}
	if lineHandler == nil {
		if source.Config().AutoMultiLineEnabled() {
			log.Infof("Auto multi line log detection enabled")

			if multiLinePattern != nil {
				log.Info("Found a previously detected pattern - using multiline handler")

				// Save the pattern again for the next rotation
				detectedPattern.Set(multiLinePattern)

				lh := NewMultiLineHandler(outputFn, multiLinePattern, config.AggregationTimeout(), lineLimit, true)
				syncSourceInfo(source, lh)
				lineHandler = lh
			} else {
				lineHandler = buildAutoMultilineHandlerFromConfig(outputFn, lineLimit, source, detectedPattern)
			}
		} else {
			lineHandler = NewSingleLineHandler(outputFn, lineLimit)
		}
	}

	// construct the lineParser, wrapping the parser
	var lineParser LineParser
	if parser.SupportsPartialLine() {
		lineParser = NewMultiLineParser(lineHandler.process, config.AggregationTimeout(), parser, lineLimit)
	} else {
		lineParser = NewSingleLineParser(lineHandler.process, parser)
	}

	// construct the framer
	framer := framer.NewFramer(lineParser.process, framing, lineLimit)

	return New(inputChan, outputChan, framer, lineParser, lineHandler, detectedPattern)
}

func buildAutoMultilineHandlerFromConfig(outputFn func(*Message), lineLimit int, source *sources.ReplaceableSource, detectedPattern *DetectedPattern) *AutoMultilineHandler {
	linesToSample := source.Config().AutoMultiLineSampleSize
	if linesToSample <= 0 {
		linesToSample = dd_conf.Datadog.GetInt("logs_config.auto_multi_line_default_sample_size")
	}
	matchThreshold := source.Config().AutoMultiLineMatchThreshold
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
	return NewAutoMultilineHandler(
		outputFn,
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
func New(InputChan chan *Input, OutputChan chan *Message, framer *framer.Framer, lineParser LineParser, lineHandler LineHandler, detectedPattern *DetectedPattern) *Decoder {
	return &Decoder{
		InputChan:       InputChan,
		OutputChan:      OutputChan,
		framer:          framer,
		lineParser:      lineParser,
		lineHandler:     lineHandler,
		detectedPattern: detectedPattern,
	}
}

// Start starts the Decoder
func (d *Decoder) Start() {
	go d.run()
}

// Stop stops the Decoder
func (d *Decoder) Stop() {
	// stop the entire decoder by closing the input.  This will "bubble" through the
	// components and eventually cause run() to finish, closing OutputChan.
	close(d.InputChan)
}

func (d *Decoder) run() {
	defer func() {
		// flush any remaining output in component order, and then close the
		// output channel
		d.lineParser.flush()
		d.lineHandler.flush()
		close(d.OutputChan)
	}()
	for {
		select {
		case data, isOpen := <-d.InputChan:
			if !isOpen {
				// InputChan has been closed, no more lines are expected
				return
			}

			d.framer.Process(data.content)

		case <-d.lineParser.flushChan():
			log.Debug("Flushing line parser because the flush timeout has been reached.")
			d.lineParser.flush()

		case <-d.lineHandler.flushChan():
			log.Debug("Flushing line handler because the flush timeout has been reached.")
			d.lineHandler.flush()
		}
	}
}

// GetLineCount returns the number of decoded lines
func (d *Decoder) GetLineCount() int64 {
	// for the moment, this counts _frames_, which aren't quite the same but
	// close enough for logging purposes
	return d.framer.GetFrameCount()
}

// GetDetectedPattern returns a detected pattern (if any)
func (d *Decoder) GetDetectedPattern() *regexp.Regexp {
	if d.detectedPattern == nil {
		return nil
	}
	return d.detectedPattern.Get()
}
