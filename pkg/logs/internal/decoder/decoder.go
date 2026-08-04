// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"regexp"
	"time"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	severityprovider "github.com/DataDog/datadog-agent/comp/logs/severityprovider/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
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

// multiLineCountable is implemented by any handler or aggregator that tracks multiline match
// and lines-combined counters, allowing them to be shared across multiple tailers for the same source.
type multiLineCountable interface {
	CountInfo() *status.CountInfo
	SetCountInfo(*status.CountInfo)
	LinesCombinedInfo() *status.CountInfo
	SetLinesCombinedInfo(*status.CountInfo)
}

// syncSourceInfo ensures that multiple decoders for the same source share the same status counters,
// so the status page displays a single combined count rather than per-tailer counts.
func syncSourceInfo(source *sources.ReplaceableSource, c multiLineCountable) {
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
	maxMessageSize := source.Config().GetMaxMessageSizeBytes(pkgconfigsetup.Datadog())
	inputChan := make(chan *message.Message)
	outputChan := make(chan *message.Message)
	detectedPattern := &DetectedPattern{}

	var sourceCategory []string
	if sc := source.Config().SourceCategory; sc != "" {
		sourceCategory = []string{"sourcecategory:" + sc}
	}
	baseBytes := message.TagMetadataBytes(source.Config().Tags, sourceCategory)
	tokenizerMaxInputBytes, labelerMaxBytes := resolveTokenizerAndLabelerMaxInputBytes(source.Config().AutoMultiLineOptions, source.Config().ExperimentalAdaptiveSampling, source.Config().ExperimentalNoisyLogDetection)
	tok := preprocessor.NewTokenizer(tokenizerMaxInputBytes)
	lineHandler := buildLineHandler(source, multiLinePattern, tailerInfo, outputChan, detectedPattern, tok, labelerMaxBytes, baseBytes)

	var lineParser LineParser
	if parser.SupportsPartialLine() {
		lineParser = NewMultiLineParser(lineHandler, config.AggregationTimeout(pkgconfigsetup.Datadog()), parser, maxMessageSize)
	} else {
		lineParser = NewSingleLineParser(lineHandler, parser)
	}

	framer := framer.NewFramer(lineParser.process, framing, maxMessageSize)

	return New(inputChan, outputChan, framer, lineParser, lineHandler, detectedPattern)
}

// resolveTokenizerAndLabelerMaxInputBytes computes the tokenizer and labeler byte windows.
// The labeler uses the effective auto-multiline tokenizer window (global, optionally overridden per source).
// The tokenizer can be widened beyond that when adaptive sampling or noisy log detection is enabled,
// so the sampler can observe more context without changing labeler behavior.
func resolveTokenizerAndLabelerMaxInputBytes(sourceAutoMLSettings *config.SourceAutoMultiLineOptions, sourceAdaptiveSampling *config.SourceAdaptiveSamplingOptions, sourceNoisyLogDetection *bool) (tokenizerMaxInputBytes int, labelerMaxBytes int) {
	labelerMaxBytes = pkgconfigsetup.Datadog().GetInt("logs_config.auto_multi_line.tokenizer_max_input_bytes")
	if sourceAutoMLSettings != nil && sourceAutoMLSettings.TokenizerMaxInputBytes != nil {
		labelerMaxBytes = *sourceAutoMLSettings.TokenizerMaxInputBytes
	}

	tokenizerMaxInputBytes = labelerMaxBytes
	if resolveAdaptiveSamplerEnabled(sourceAdaptiveSampling) || resolveNoisyLogDetectionEnabled(sourceNoisyLogDetection) {
		samplerMin := pkgconfigsetup.Datadog().GetInt("logs_config.experimental_adaptive_sampling.tokenizer_max_input_bytes")
		if sourceAdaptiveSampling != nil && sourceAdaptiveSampling.TokenizerMaxInputBytes != nil {
			samplerMin = *sourceAdaptiveSampling.TokenizerMaxInputBytes
		}
		if samplerMin > tokenizerMaxInputBytes {
			tokenizerMaxInputBytes = samplerMin
		}
	}

	return tokenizerMaxInputBytes, labelerMaxBytes
}

func resolveAdaptiveSamplerEnabled(sourceAdaptiveSampling *config.SourceAdaptiveSamplingOptions) bool {
	if sourceAdaptiveSampling != nil && sourceAdaptiveSampling.Enabled != nil {
		return *sourceAdaptiveSampling.Enabled
	}

	return pkgconfigsetup.Datadog().GetBool("logs_config.experimental_adaptive_sampling.enabled")
}

func resolveNoisyLogDetectionEnabled(sourceNoisyLogDetection *bool) bool {
	if sourceNoisyLogDetection != nil {
		return *sourceNoisyLogDetection
	}

	return pkgconfigsetup.Datadog().GetBool("logs_config.experimental_noisy_log_detection")
}

const disabledSourcesConfigKey = "logs_config.experimental_adaptive_sampling.disabled_sources"

const (
	smartSeverityProfilesEnabledConfigKey           = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.enabled"
	smartSeverityProfilesMediumPassThroughConfigKey = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.medium.pass_through"
	smartSeverityProfilesMediumRateLimitConfigKey   = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.medium.rate_limit"
	smartSeverityProfilesMediumBurstSizeConfigKey   = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.medium.burst_size"
	smartSeverityProfilesHighPassThroughConfigKey   = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.high.pass_through"
	smartSeverityProfilesHighRateLimitConfigKey     = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.high.rate_limit"
	smartSeverityProfilesHighBurstSizeConfigKey     = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.high.burst_size"
)

// resolveSmartSeverityProfiles builds the Low/Medium/High profile triple. Each field of
// Medium/High cascades independently from the level below when left unconfigured (Low ->
// Medium -> High), so no combination of partially-configured fields can leave a higher
// severity level less permissive than the one below it.
func resolveSmartSeverityProfiles(low preprocessor.SamplerProfile) [severityeventsdef.NumSeverityLevels]preprocessor.SamplerProfile {
	cfg := pkgconfigsetup.Datadog()

	profiles := [severityeventsdef.NumSeverityLevels]preprocessor.SamplerProfile{
		severityeventsdef.SeverityLow:    low,
		severityeventsdef.SeverityMedium: low,
		severityeventsdef.SeverityHigh:   low,
	}

	if cfg.IsConfigured(smartSeverityProfilesMediumRateLimitConfigKey) {
		profiles[severityeventsdef.SeverityMedium].RateLimit = cfg.GetFloat64(smartSeverityProfilesMediumRateLimitConfigKey)
	}
	if cfg.IsConfigured(smartSeverityProfilesMediumBurstSizeConfigKey) {
		profiles[severityeventsdef.SeverityMedium].BurstSize = clampBurstSize(cfg.GetFloat64(smartSeverityProfilesMediumBurstSizeConfigKey))
	}
	if cfg.IsConfigured(smartSeverityProfilesMediumPassThroughConfigKey) {
		profiles[severityeventsdef.SeverityMedium].PassThrough = cfg.GetBool(smartSeverityProfilesMediumPassThroughConfigKey)
	}

	// High starts from Medium's already-resolved profile, then applies its own
	// overrides per field.
	profiles[severityeventsdef.SeverityHigh] = profiles[severityeventsdef.SeverityMedium]
	if cfg.IsConfigured(smartSeverityProfilesHighRateLimitConfigKey) {
		profiles[severityeventsdef.SeverityHigh].RateLimit = cfg.GetFloat64(smartSeverityProfilesHighRateLimitConfigKey)
	}
	if cfg.IsConfigured(smartSeverityProfilesHighBurstSizeConfigKey) {
		profiles[severityeventsdef.SeverityHigh].BurstSize = clampBurstSize(cfg.GetFloat64(smartSeverityProfilesHighBurstSizeConfigKey))
	}
	if cfg.IsConfigured(smartSeverityProfilesHighPassThroughConfigKey) {
		profiles[severityeventsdef.SeverityHigh].PassThrough = cfg.GetBool(smartSeverityProfilesHighPassThroughConfigKey)
	}

	return profiles
}

func newDisabledSet() map[string]struct{} {
	entries := pkgconfigsetup.Datadog().GetStringSlice(disabledSourcesConfigKey)
	m := make(map[string]struct{}, len(entries))
	for _, s := range entries {
		m[s] = struct{}{}
	}
	return m
}

// buildIsSourceDisabled builds a closure that checks whether the current source
// is in the disabled_sources set. The set is built once at init; the source name
// is read per-message through ReplaceableSource to track source swaps.
// When Remote Config support is added, the set can be rebuilt via a callback
// (e.g. using atomic.Pointer for lock-free reads) without changing the caller.
func buildIsSourceDisabled(source *sources.ReplaceableSource) func() bool {
	disabledSet := newDisabledSet()
	if len(disabledSet) == 0 {
		return nil
	}
	return func() bool {
		_, disabled := disabledSet[source.Config().Source]
		return disabled
	}
}

type samplerMode int

const (
	samplerDisabled samplerMode = iota
	samplerAdaptiveSampling
	samplerNoisyLogDetection
)

func resolveSamplerMode(sourceAdaptiveSampling *config.SourceAdaptiveSamplingOptions, sourceNoisyLogDetection *bool) samplerMode {
	if resolveAdaptiveSamplerEnabled(sourceAdaptiveSampling) {
		return samplerAdaptiveSampling
	}
	if resolveNoisyLogDetectionEnabled(sourceNoisyLogDetection) {
		return samplerNoisyLogDetection
	}
	return samplerDisabled
}

func resolveAdaptiveSamplerConfig(sourceAdaptiveSampling *config.SourceAdaptiveSamplingOptions, tok *preprocessor.Tokenizer) preprocessor.AdaptiveSamplerConfig {
	includeFilters, includeConfigured := resolveGlobalAdaptiveSamplerFilters("logs_config.experimental_adaptive_sampling.include", tok)
	excludeFilters, _ := resolveGlobalAdaptiveSamplerFilters("logs_config.experimental_adaptive_sampling.exclude", tok)

	c := preprocessor.AdaptiveSamplerConfig{
		MaxPatterns:          pkgconfigsetup.Datadog().GetInt("logs_config.experimental_adaptive_sampling.max_patterns"),
		RateLimit:            pkgconfigsetup.Datadog().GetFloat64("logs_config.experimental_adaptive_sampling.rate_limit"),
		BurstSize:            pkgconfigsetup.Datadog().GetFloat64("logs_config.experimental_adaptive_sampling.burst_size"),
		MatchThreshold:       pkgconfigsetup.Datadog().GetFloat64("logs_config.experimental_adaptive_sampling.match_threshold"),
		ProtectImportantLogs: pkgconfigsetup.Datadog().GetBool("logs_config.experimental_adaptive_sampling.protect_important_logs"),
		TagPatternHash:       pkgconfigsetup.Datadog().GetBool("logs_config.experimental_adaptive_sampling.tag_pattern_hash"),
		Include:              includeFilters,
		IncludeConfigured:    includeConfigured,
		Exclude:              excludeFilters,
	}

	if sourceAdaptiveSampling != nil {
		if sourceAdaptiveSampling.MaxPatterns != nil {
			c.MaxPatterns = *sourceAdaptiveSampling.MaxPatterns
		}
		if sourceAdaptiveSampling.RateLimit != nil {
			c.RateLimit = *sourceAdaptiveSampling.RateLimit
		}
		if sourceAdaptiveSampling.BurstSize != nil {
			c.BurstSize = *sourceAdaptiveSampling.BurstSize
		}
		if sourceAdaptiveSampling.MatchThreshold != nil {
			c.MatchThreshold = *sourceAdaptiveSampling.MatchThreshold
		}
		if sourceAdaptiveSampling.ProtectImportantLogs != nil {
			c.ProtectImportantLogs = *sourceAdaptiveSampling.ProtectImportantLogs
		}
		if sourceAdaptiveSampling.TagPatternHash != nil {
			c.TagPatternHash = *sourceAdaptiveSampling.TagPatternHash
		}
		if sourceAdaptiveSampling.Include != nil {
			c.Include = resolveAdaptiveSamplerFilters(sourceAdaptiveSampling.Include, tok)
			c.IncludeConfigured = true
		}
		if sourceAdaptiveSampling.Exclude != nil {
			c.Exclude = resolveAdaptiveSamplerFilters(sourceAdaptiveSampling.Exclude, tok)
		}
	}

	c = validateAdaptiveSamplerConfig(c)

	c.SmartSeverityProfilesEnabled = pkgconfigsetup.Datadog().GetBool(smartSeverityProfilesEnabledConfigKey)
	if c.SmartSeverityProfilesEnabled {
		c.Profiles = resolveSmartSeverityProfiles(preprocessor.SamplerProfile{RateLimit: c.RateLimit, BurstSize: c.BurstSize})
		c.SeverityProvider = severityprovider.Current
	}

	return c
}

func resolveNoisyLogDetectionConfig(sourceAdaptiveSampling *config.SourceAdaptiveSamplingOptions, tok *preprocessor.Tokenizer) preprocessor.AdaptiveSamplerConfig {
	c := resolveAdaptiveSamplerConfig(sourceAdaptiveSampling, tok)
	c.DetectionOnly = true
	return c
}

func resolveGlobalAdaptiveSamplerFilters(key string, tok *preprocessor.Tokenizer) ([]preprocessor.AdaptiveSamplerFilter, bool) {
	cfg := pkgconfigsetup.Datadog()
	if !cfg.IsConfigured(key) {
		return nil, false
	}

	var rules []*config.AdaptiveSamplingRule
	if err := structure.UnmarshalKey(cfg, key, &rules, structure.EnableStringUnmarshal); err != nil {
		log.Warnf("Failed to unmarshal adaptive sampler filters from %s, skipping: %v", key, err)
		return nil, true
	}

	return resolveAdaptiveSamplerFilters(rules, tok), true
}

func resolveAdaptiveSamplerFilters(rules []*config.AdaptiveSamplingRule, tok *preprocessor.Tokenizer) []preprocessor.AdaptiveSamplerFilter {
	if len(rules) == 0 {
		return nil
	}
	if tok == nil {
		tok = preprocessor.NewTokenizer(0)
	}

	filters := make([]preprocessor.AdaptiveSamplerFilter, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}

		filter := preprocessor.AdaptiveSamplerFilter{}
		if rule.Regex != "" {
			compiled, err := regexp.Compile(rule.Regex)
			if err != nil {
				log.Warnf("Invalid adaptive sampler filter regex %q, skipping rule: %v", rule.Regex, err)
				continue
			}
			filter.Regex = compiled
		}
		if rule.Sample != "" {
			filter.SampleTokens, _ = tok.Tokenize([]byte(rule.Sample))
		}
		if filter.Regex == nil && len(filter.SampleTokens) == 0 {
			log.Warn("Adaptive sampler filter rule is empty, skipping")
			continue
		}

		filters = append(filters, filter)
	}
	return filters
}

func buildLineHandler(source *sources.ReplaceableSource, multiLinePattern *regexp.Regexp, tailerInfo *status.InfoRegistry, outputChan chan *message.Message, detectedPattern *DetectedPattern, tok *preprocessor.Tokenizer, labelerMaxBytes int, baseBytesEstimate int) LineHandler {
	maxContentSize := config.MaxMessageSizeBytes(pkgconfigsetup.Datadog())
	flushTimeout := config.AggregationTimeout(pkgconfigsetup.Datadog())

	var sampler preprocessor.Sampler
	sourceConfig := source.Config()
	switch resolveSamplerMode(sourceConfig.ExperimentalAdaptiveSampling, sourceConfig.ExperimentalNoisyLogDetection) {
	case samplerAdaptiveSampling:
		cfg := resolveAdaptiveSamplerConfig(sourceConfig.ExperimentalAdaptiveSampling, tok)
		cfg.IsSourceDisabled = buildIsSourceDisabled(source)
		sampler = preprocessor.NewAdaptiveSampler(cfg, source.UnderlyingSource().Name, baseBytesEstimate)
	case samplerNoisyLogDetection:
		cfg := resolveNoisyLogDetectionConfig(sourceConfig.ExperimentalAdaptiveSampling, tok)
		cfg.IsSourceDisabled = buildIsSourceDisabled(source)
		sampler = preprocessor.NewAdaptiveSampler(cfg, source.UnderlyingSource().Name, baseBytesEstimate)
	default:
		sampler = preprocessor.NewNoopSampler()
	}

	// directOutputFn is used by legacy handlers that bypass the Preprocessor.
	directOutputFn := func(msg *message.Message) { outputChan <- msg }

	// User-configured multiline regex — each line is matched against the regex to detect group
	// boundaries; completed groups are emitted as a single combined message.
	var lineHandler LineHandler
	for _, rule := range source.Config().ProcessingRules {
		if rule.Type == config.MultiLine {
			regexAggregator := preprocessor.NewRegexAggregator(rule.Regex, maxContentSize, false, tailerInfo, "multi_line")
			syncSourceInfo(source, regexAggregator)
			lineHandler = newPreprocessorHandler(regexAggregator, tok, preprocessor.NewNoopLabeler(), sampler, outputChan, preprocessor.NewNoopJSONAggregator(), preprocessor.NewNoopStackTraceAggregator(), flushTimeout, labelerMaxBytes)
		}
	}

	if lineHandler != nil {
		return lineHandler
	}

	// Priority order when no user-configured regex rule was set:
	// 1. Legacy auto multiline (bypasses Preprocessor; outputs directly to outputChan)
	// 2. Auto multiline with aggregation — combines detected groups into one message
	// 3. Auto multiline detection-only — tags group starts without combining, this is the default
	// 4. Pass-through — tokenizes and samples every line individually
	if source.Config().LegacyAutoMultiLineEnabled(pkgconfigsetup.Datadog()) {
		return getLegacyAutoMultilineHandler(directOutputFn, multiLinePattern, maxContentSize, source, detectedPattern, tailerInfo)
	} else if source.Config().AutoMultiLineEnabled(pkgconfigsetup.Datadog()) {
		labeler := buildAutoMultilineLabeler(source.Config().AutoMultiLineOptions, source.Config().AutoMultiLineSamples, tailerInfo)
		combiningAggregator := preprocessor.NewCombiningAggregator(maxContentSize,
			pkgconfigsetup.Datadog().GetBool("logs_config.tag_truncated_logs"),
			pkgconfigsetup.Datadog().GetBool("logs_config.tag_multi_line_logs"),
			tailerInfo)
		enableJSON := pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.enable_json_aggregation")
		if source.Config().AutoMultiLineOptions != nil && source.Config().AutoMultiLineOptions.EnableJSONAggregation != nil {
			enableJSON = *source.Config().AutoMultiLineOptions.EnableJSONAggregation
		}
		var jsonAgg preprocessor.JSONAggregator = preprocessor.NewNoopJSONAggregator()
		if enableJSON {
			jsonAgg = preprocessor.NewJSONAggregator(pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line.tag_aggregated_json"), maxContentSize)
		}
		stackTraceParsers := resolveStackTraceParsers(source)
		stackTraceAgg := preprocessor.NewStackTraceAggregatorFromNames(stackTraceParsers, maxContentSize,
			pkgconfigsetup.Datadog().GetBool("logs_config.tag_multi_line_logs"))
		return newPreprocessorHandler(combiningAggregator, tok, labeler, sampler, outputChan, jsonAgg, stackTraceAgg, flushTimeout, labelerMaxBytes)
	} else if pkgconfigsetup.Datadog().GetBool("logs_config.auto_multi_line_detection_tagging") {
		labeler := buildAutoMultilineLabeler(source.Config().AutoMultiLineOptions, source.Config().AutoMultiLineSamples, tailerInfo)
		cfg := pkgconfigsetup.Datadog()
		_, isDefaultPath := source.Config().AutoMultiLineStatus(cfg)
		// JSON aggregation is disabled in detection mode — we don't want to combine JSON
		// while only tagging everything else.
		detectingAggregator := preprocessor.NewDetectingAggregator(tailerInfo, maxContentSize, pkgconfigsetup.Datadog().GetBool("logs_config.tag_truncated_logs"), isDefaultPath)
		return newPreprocessorHandler(detectingAggregator, tok, labeler, sampler, outputChan, preprocessor.NewNoopJSONAggregator(), preprocessor.NewNoopStackTraceAggregator(), flushTimeout, labelerMaxBytes)
	}
	return newPreprocessorHandler(preprocessor.NewPassThroughAggregator(maxContentSize), tok, preprocessor.NewNoopLabeler(), sampler, outputChan, preprocessor.NewNoopJSONAggregator(), preprocessor.NewNoopStackTraceAggregator(), flushTimeout, 0)
}

// resolveStackTraceParsers returns the list of enabled stack trace parser
// names for the given source, respecting per-source overrides.
func resolveStackTraceParsers(source *sources.ReplaceableSource) []string {
	opts := source.Config().AutoMultiLineOptions
	if opts != nil && opts.StackTraceParsers != nil {
		return *opts.StackTraceParsers
	}
	return pkgconfigsetup.Datadog().GetStringSlice("logs_config.auto_multi_line.stack_trace_parsers")
}

func validateAdaptiveSamplerConfig(c preprocessor.AdaptiveSamplerConfig) preprocessor.AdaptiveSamplerConfig {
	if c.MaxPatterns <= 0 {
		c.MaxPatterns = 1
	}

	c.BurstSize = clampBurstSize(c.BurstSize)

	return c
}

// clampBurstSize floors burstSize at 1, avoiding negative starting credits.
func clampBurstSize(burstSize float64) float64 {
	if burstSize <= 0 {
		return 1
	}
	return burstSize
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
		// Flush any remaining output in component order, and then close the
		// output channel. The framer flush gives the FrameMatcher a chance to
		// emit buffered data that was waiting for a delimiter that never
		// arrived (e.g. non-transparent syslog without a trailing LF).
		d.framer.Flush()
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
