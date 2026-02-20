// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/preprocessor"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/framer"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewInput returns a new decoder input.
// A decoder input is an unstructured message of raw bytes as the content.
// Some of the tailers using this are the file tailers and socket tailers
// as these logs don't have any structure, they're just raw bytes log.
// See message.Message / message.MessageContent comment for more information.
func NewInput(content []byte) *message.Message {
	return message.NewMessage(content, nil, "", time.Now().UnixNano())
}

// Decoder translates a sequence of byte buffers (such as from a file or a
// network socket) into log messages.
//
// Decoder is structured as an actor receiving messages on `InputChan` and
// writing its output in `OutputChan`
//
// The Decoder's run() takes data from InputChan, uses a Framer to break it into frames.
// The Framer passes that data to a LineParser, which uses a Parser to parse it and
// to pass it to the LineHander.
//
// The LineHandler processes the messages it as necessary (as single lines,
// multiple lines, or auto-detecting the two), and sends the result to the
// Decoder's output channel.
type Decoder interface {
	Start()
	Stop()
	GetLineCount() int64
	GetDetectedPattern() *regexp.Regexp
	InputChan() chan *message.Message
	OutputChan() chan *message.Message
}

// decoderImpl is the default implementation of the Decoder interface
type decoderImpl struct {
	inputChan  chan *message.Message
	outputChan chan *message.Message

	framer      *framer.Framer
	lineParser  LineParser
	lineHandler LineHandler

	// The decoder holds on to an instace of DetectedPattern which is a thread safe container used to
	// pass a multiline pattern up from the line handler in order to surface it to the tailer.
	// The tailer uses this to determine if a pattern should be reused when a file rotates.
	detectedPattern *DetectedPattern
}

func (d *decoderImpl) InputChan() chan *message.Message {
	return d.inputChan
}

func (d *decoderImpl) OutputChan() chan *message.Message {
	return d.outputChan
}

// InitializeDecoder returns a properly initialized Decoder
func InitializeDecoder(source *sources.ReplaceableSource, parser parsers.Parser, tailerInfo *status.InfoRegistry) Decoder {
	return NewDecoderWithFraming(source, parser, framer.UTF8Newline, nil, tailerInfo)
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

// syncSourceInfoForRegexCombiner syncs the RegexCombiner's counters with the source's info
// registry so that multiple tailers for the same source share the same count displays.
func syncSourceInfoForRegexCombiner(source *sources.ReplaceableSource, c *preprocessor.RegexCombiner) {
	if existingInfo, ok := source.GetInfo(c.CountInfo().InfoKey()).(*status.CountInfo); ok {
		c.SetCountInfo(existingInfo)
	} else {
		source.RegisterInfo(c.CountInfo())
	}
	if existingInfo, ok := source.GetInfo(c.LinesCombinedInfo().InfoKey()).(*status.CountInfo); ok {
		c.SetLinesCombinedInfo(existingInfo)
	} else {
		source.RegisterInfo(c.LinesCombinedInfo())
	}
}

// NewNoopDecoder initializes a decoder with all dependent components in passthrough mode.
func NewNoopDecoder() Decoder {
	inputChan := make(chan *message.Message)
	outputChan := make(chan *message.Message)
	detectedPattern := &DetectedPattern{}
	maxMessageSize := config.MaxMessageSizeBytes(pkgconfigsetup.Datadog())

	lineHandler := NewNoopLineHandler(outputChan)
	lineParser := NewSingleLineParser(lineHandler, noop.New())
	framer := framer.NewFramer(lineParser.process, framer.NoFraming, maxMessageSize)

	return New(inputChan, outputChan, framer, lineParser, lineHandler, detectedPattern)
}

// NewDecoderWithFraming initialize a decoder with given endline strategy.
func NewDecoderWithFraming(source *sources.ReplaceableSource, parser parsers.Parser, framing framer.Framing, multiLinePattern *regexp.Regexp, tailerInfo *status.InfoRegistry) Decoder {
	maxMessageSize := config.MaxMessageSizeBytes(pkgconfigsetup.Datadog())
	inputChan := make(chan *message.Message)
	outputChan := make(chan *message.Message)
	detectedPattern := &DetectedPattern{}

	// Sampler is the final step of the Pipeline — emits completed messages to outputChan.
	sampler := preprocessor.NewNoopSampler(outputChan)

	// TODO: AGNTLOG-553 Respect source-specific tokenizer settings
	//       (source.Config().AutoMultiLineOptions.TokenizerMaxInputBytes) to
	//       avoid breaking change for sources with custom tokenizer config
	tokenizerMaxInputBytes := pkgconfigsetup.Datadog().GetInt("logs_config.auto_multi_line.tokenizer_max_input_bytes")
	tok := preprocessor.NewTokenizer(tokenizerMaxInputBytes)
	lineHandler := buildLineHandler(source, multiLinePattern, tailerInfo, outputChan, detectedPattern, tok, sampler)

	var lineParser LineParser
	if parser.SupportsPartialLine() {
		lineParser = NewMultiLineParser(lineHandler, config.AggregationTimeout(pkgconfigsetup.Datadog()), parser, maxMessageSize)
	} else {
		lineParser = NewSingleLineParser(lineHandler, parser)
	}

	framer := framer.NewFramer(lineParser.process, framing, maxMessageSize)

	return New(inputChan, outputChan, framer, lineParser, lineHandler, detectedPattern)
}

func buildLineHandler(source *sources.ReplaceableSource, multiLinePattern *regexp.Regexp, tailerInfo *status.InfoRegistry, outputChan chan *message.Message, detectedPattern *DetectedPattern, tok *preprocessor.Tokenizer, sampler preprocessor.Sampler) LineHandler {
	maxContentSize := config.MaxMessageSizeBytes(pkgconfigsetup.Datadog())
	flushTimeout := config.AggregationTimeout(pkgconfigsetup.Datadog())

	// directOutputFn is used by legacy handlers that bypass the Pipeline.
	directOutputFn := func(msg *message.Message) { outputChan <- msg }

	// User-configured multiline regex — each line is matched against the regex to detect group
	// boundaries; completed groups are emitted as a single combined message.
	var lineHandler LineHandler
	for _, rule := range source.Config().ProcessingRules {
		if rule.Type == config.MultiLine {
			regexCombiner := preprocessor.NewRegexCombiner(rule.Regex, maxContentSize, false, tailerInfo, "multi_line")
			syncSourceInfoForRegexCombiner(source, regexCombiner)
			lineHandler = newPipelineHandler(regexCombiner, tok, sampler, nil, false, flushTimeout)
		}
	}

	if lineHandler != nil {
		return lineHandler
	}

	// Priority order when no user-configured regex rule was set:
	// 1. Legacy auto multiline (bypasses Pipeline; outputs directly to outputChan)
	// 2. Auto multiline with aggregation — combines detected groups into one message
	// 3. Auto multiline detection-only — tags group starts without combining, this is the default
	// 4. Pass-through — tokenizes and samples every line individually
	if source.Config().LegacyAutoMultiLineEnabled(pkgconfigsetup.Datadog()) {
		return getLegacyAutoMultilineHandler(directOutputFn, multiLinePattern, maxContentSize, source, detectedPattern, tailerInfo)
	} else if source.Config().AutoMultiLineEnabled(pkgconfigsetup.Datadog()) {
		labeler := buildAutoMultilineLabeler(source.Config().AutoMultiLineOptions, source.Config().AutoMultiLineSamples, tailerInfo)
		aggregatorFactory := func(outputFn func(*message.Message)) preprocessor.Aggregator {
			return preprocessor.NewCombiningAggregator(outputFn, maxContentSize,
				pkgconfigsetup.Datadog().GetBool("logs_config.tag_truncated_logs"),
				pkgconfigsetup.Datadog().GetBool("logs_config.tag_multi_line_logs"),
				tailerInfo)
		}
		combiner := preprocessor.NewAutoMultilineCombiner(labeler, aggregatorFactory)
		enableJSON := pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.enable_json_aggregation")
		if source.Config().AutoMultiLineOptions != nil && source.Config().AutoMultiLineOptions.EnableJSONAggregation != nil {
			enableJSON = *source.Config().AutoMultiLineOptions.EnableJSONAggregation
		}
		jsonAgg := preprocessor.NewJSONAggregator(pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.tag_aggregated_json"), maxContentSize)
		return newPipelineHandler(combiner, tok, sampler, jsonAgg, enableJSON, flushTimeout)
	} else if pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line_detection_tagging") {
		labeler := buildAutoMultilineLabeler(source.Config().AutoMultiLineOptions, source.Config().AutoMultiLineSamples, tailerInfo)
		aggregatorFactory := func(outputFn func(*message.Message)) preprocessor.Aggregator {
			// JSON aggregation is disabled in detection mode — we don't want to combine JSON
			// while only tagging everything else.
			return preprocessor.NewDetectingAggregator(outputFn, tailerInfo)
		}
		combiner := preprocessor.NewAutoMultilineCombiner(labeler, aggregatorFactory)
		return newPipelineHandler(combiner, tok, sampler, nil, false, flushTimeout)
	}
	return newPipelineHandler(preprocessor.NewPassThroughCombiner(maxContentSize), tok, sampler, nil, false, flushTimeout)
}

func getLegacyAutoMultilineHandler(outputFn func(*message.Message), multiLinePattern *regexp.Regexp, maxContentSize int, source *sources.ReplaceableSource, detectedPattern *DetectedPattern, tailerInfo *status.InfoRegistry) LineHandler {

	if multiLinePattern != nil {
		log.Info("Found a previously detected pattern - using multiline handler")

		// Save the pattern again for the next rotation
		detectedPattern.Set(multiLinePattern)

		lh := NewMultiLineHandler(outputFn, multiLinePattern, config.AggregationTimeout(pkgconfigsetup.Datadog()), maxContentSize, true, tailerInfo, "legacy_auto_multi_line")
		syncSourceInfo(source, lh)
		return lh
	}
	return buildLegacyAutoMultilineHandlerFromConfig(outputFn, maxContentSize, source, detectedPattern, tailerInfo)
}

func buildLegacyAutoMultilineHandlerFromConfig(outputFn func(*message.Message), maxContentSize int, source *sources.ReplaceableSource, detectedPattern *DetectedPattern, tailerInfo *status.InfoRegistry) *LegacyAutoMultilineHandler {
	linesToSample := source.Config().AutoMultiLineSampleSize
	if linesToSample <= 0 {
		linesToSample = pkgconfigsetup.Datadog().GetInt("logs_config.auto_multi_line_default_sample_size")
	}
	matchThreshold := source.Config().AutoMultiLineMatchThreshold
	if matchThreshold == 0 {
		matchThreshold = pkgconfigsetup.Datadog().GetFloat64("logs_config.auto_multi_line_default_match_threshold")
	}
	additionalPatterns := pkgconfigsetup.Datadog().GetStringSlice("logs_config.auto_multi_line_extra_patterns")
	additionalPatternsCompiled := []*regexp.Regexp{}

	for _, p := range additionalPatterns {
		compiled, err := regexp.Compile("^" + p)
		if err != nil {
			log.Warn("logs_config.auto_multi_line_extra_patterns containing value: ", p, " is not a valid regular expression")
			continue
		}
		additionalPatternsCompiled = append(additionalPatternsCompiled, compiled)
	}

	matchTimeout := time.Second * pkgconfigsetup.Datadog().GetDuration("logs_config.auto_multi_line_default_match_timeout")
	return NewLegacyAutoMultilineHandler(
		outputFn,
		maxContentSize,
		linesToSample,
		matchThreshold,
		matchTimeout,
		config.AggregationTimeout(pkgconfigsetup.Datadog()),
		source,
		additionalPatternsCompiled,
		detectedPattern,
		tailerInfo,
	)
}

// New returns an initialized Decoder
func New(InputChan chan *message.Message, OutputChan chan *message.Message, framer *framer.Framer, lineParser LineParser, lineHandler LineHandler, detectedPattern *DetectedPattern) Decoder {
	return &decoderImpl{
		inputChan:       InputChan,
		outputChan:      OutputChan,
		framer:          framer,
		lineParser:      lineParser,
		lineHandler:     lineHandler,
		detectedPattern: detectedPattern,
	}
}

// Start starts the Decoder
func (d *decoderImpl) Start() {
	go d.run()
}

// Stop stops the Decoder
func (d *decoderImpl) Stop() {
	// stop the entire decoder by closing the input.  This will "bubble" through the
	// components and eventually cause run() to finish, closing OutputChan.
	close(d.InputChan())
}

func (d *decoderImpl) run() {
	defer func() {
		// flush any remaining output in component order, and then close the
		// output channel
		d.lineParser.flush()
		d.lineHandler.flush()
		close(d.outputChan)
	}()
	for {
		select {
		case msg, isOpen := <-d.InputChan():
			if !isOpen {
				// InputChan has been closed, no more lines are expected
				return
			}

			d.framer.Process(msg)

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
func (d *decoderImpl) GetLineCount() int64 {
	// for the moment, this counts _frames_, which aren't quite the same but
	// close enough for logging purposes
	return d.framer.GetFrameCount()
}

// GetDetectedPattern returns a detected pattern (if any)
func (d *decoderImpl) GetDetectedPattern() *regexp.Regexp {
	if d.detectedPattern == nil {
		return nil
	}
	return d.detectedPattern.Get()
}
