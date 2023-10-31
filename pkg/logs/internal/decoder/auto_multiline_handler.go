// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const autoMultiLineTelemetryMetricName = "datadog.logs_agent.auto_multi_line"

type scoredPattern struct {
	score  int
	regexp *regexp.Regexp
}

// DetectedPattern is a container to safely access a detected multiline pattern
type DetectedPattern struct {
	sync.Mutex
	pattern *regexp.Regexp
}

// Set sets the pattern
func (d *DetectedPattern) Set(pattern *regexp.Regexp) {
	d.Lock()
	defer d.Unlock()
	d.pattern = pattern
}

// Get gets the pattern
func (d *DetectedPattern) Get() *regexp.Regexp {
	d.Lock()
	defer d.Unlock()
	return d.pattern
}

// AutoMultilineHandler can attempts to detect a known/commob pattern (a timestamp) in the logs
// and will switch to a MultiLine handler if one is detected and the thresholds are met.
type AutoMultilineHandler struct {
	multiLineHandler    *MultiLineHandler
	singleLineHandler   *SingleLineHandler
	outputFn            func(*message.Message)
	isRunning           bool
	linesToAssess       int
	linesTested         int
	lineLimit           int
	matchThreshold      float64
	scoredMatches       []*scoredPattern
	processFunc         func(message *message.Message)
	flushTimeout        time.Duration
	source              *sources.ReplaceableSource
	matchTimeout        time.Duration
	timeoutTimer        *clock.Timer
	detectedPattern     *DetectedPattern
	clk                 clock.Clock
	autoMultiLineStatus *status.MappedInfo
}

// NewAutoMultilineHandler returns a new AutoMultilineHandler.
func NewAutoMultilineHandler(
	outputFn func(*message.Message),
	lineLimit, linesToAssess int,
	matchThreshold float64,
	matchTimeout time.Duration,
	flushTimeout time.Duration,
	source *sources.ReplaceableSource,
	additionalPatterns []*regexp.Regexp,
	detectedPattern *DetectedPattern,
	tailerInfo *status.InfoRegistry,
) *AutoMultilineHandler {

	// Put the user patterns at the beginning of the list so we prioritize them if there is a conflicting match.
	patterns := append(additionalPatterns, formatsToTry...)

	scoredMatches := make([]*scoredPattern, len(patterns))
	for i, v := range patterns {
		scoredMatches[i] = &scoredPattern{
			score:  0,
			regexp: v,
		}
	}
	h := &AutoMultilineHandler{
		outputFn:            outputFn,
		isRunning:           true,
		lineLimit:           lineLimit,
		matchThreshold:      matchThreshold,
		scoredMatches:       scoredMatches,
		linesToAssess:       linesToAssess,
		flushTimeout:        flushTimeout,
		source:              source,
		matchTimeout:        matchTimeout,
		timeoutTimer:        nil,
		detectedPattern:     detectedPattern,
		clk:                 clock.New(),
		autoMultiLineStatus: status.NewMappedInfo("Auto Multi-line"),
	}

	h.singleLineHandler = NewSingleLineHandler(outputFn, lineLimit)
	h.processFunc = h.processAndTry
	tailerInfo.Register(h.autoMultiLineStatus)
	h.autoMultiLineStatus.SetMessage("state", "Waiting for logs")

	return h
}

func (h *AutoMultilineHandler) process(message *message.Message) {
	h.processFunc(message)
}

func (h *AutoMultilineHandler) flushChan() <-chan time.Time {
	if h.singleLineHandler != nil {
		return h.singleLineHandler.flushChan()
	}
	return h.multiLineHandler.flushChan()
}

func (h *AutoMultilineHandler) flush() {
	if h.singleLineHandler != nil {
		h.singleLineHandler.flush()
	} else {
		h.multiLineHandler.flush()
	}
}

func (h *AutoMultilineHandler) processAndTry(message *message.Message) {
	// Process message before anything else
	h.singleLineHandler.process(message)

	for i, scoredPattern := range h.scoredMatches {
		match := scoredPattern.regexp.Match(message.GetContent())
		if match {
			scoredPattern.score++

			// By keeping the scored matches sorted, the best match always comes first. Since we expect one timestamp to match overwhelmingly
			// it should match most often causing few re-sorts.
			if i != 0 {
				sort.Slice(h.scoredMatches, func(i, j int) bool {
					return h.scoredMatches[i].score > h.scoredMatches[j].score
				})
			}
			break
		}
	}

	if h.timeoutTimer == nil {
		h.timeoutTimer = h.clk.Timer(h.matchTimeout)
	}

	h.linesTested++

	timeout := false
	select {
	case <-h.timeoutTimer.C:
		log.Debug("Multiline auto detect timed out before reaching line test threshold")
		h.autoMultiLineStatus.SetMessage("message2", fmt.Sprintf("Timeout reached. Processed (%d of %d) logs during detection", h.linesTested, h.linesToAssess))
		timeout = true
		break
	default:
	}

	h.autoMultiLineStatus.SetMessage("state", "State: Using auto multi-line handler")
	h.autoMultiLineStatus.SetMessage("message", fmt.Sprintf("Detecting (%d of %d)", h.linesTested, h.linesToAssess))

	if h.linesTested >= h.linesToAssess || timeout {
		topMatch := h.scoredMatches[0]
		matchRatio := float64(topMatch.score) / float64(h.linesTested)

		if matchRatio >= h.matchThreshold {
			h.autoMultiLineStatus.SetMessage("state", "State: Using multi-line handler")
			h.autoMultiLineStatus.SetMessage("message", fmt.Sprintf("Pattern %v matched %d lines with a ratio of %f", topMatch.regexp.String(), topMatch.score, matchRatio))
			log.Debug(fmt.Sprintf("Pattern %v matched %d lines with a ratio of %f - using multi-line handler", topMatch.regexp.String(), topMatch.score, matchRatio))
			telemetry.GetStatsTelemetryProvider().Count(autoMultiLineTelemetryMetricName, 1, []string{"success:true"})

			h.detectedPattern.Set(topMatch.regexp)
			h.switchToMultilineHandler(topMatch.regexp)
		} else {
			h.autoMultiLineStatus.SetMessage("state", "State: Using single-line handler")
			h.autoMultiLineStatus.SetMessage("message", fmt.Sprintf("No pattern met the line match threshold: %f during multiline auto detection. Top match was %v with a match ratio of: %f", h.matchThreshold, topMatch.regexp.String(), matchRatio))
			log.Debugf(fmt.Sprintf("No pattern met the line match threshold: %f during multiline auto detection. Top match was %v with a match ratio of: %f - using single-line handler", h.matchThreshold, topMatch.regexp.String(), matchRatio))
			telemetry.GetStatsTelemetryProvider().Count(autoMultiLineTelemetryMetricName, 1, []string{"success:false"})

			// Stay with the single line handler and no longer attempt to detect multiline matches.
			h.processFunc = h.singleLineHandler.process
		}
	}
}

func (h *AutoMultilineHandler) switchToMultilineHandler(r *regexp.Regexp) {
	h.isRunning = false
	h.singleLineHandler = nil

	// Build and start a multiline-handler
	h.multiLineHandler = NewMultiLineHandler(h.outputFn, r, h.flushTimeout, h.lineLimit, true)
	h.source.RegisterInfo(h.multiLineHandler.countInfo)
	h.source.RegisterInfo(h.multiLineHandler.linesCombinedInfo)
	// stay with the multiline handler
	h.processFunc = h.multiLineHandler.process
}

// Originally referenced from https://github.com/egnyte/ax/blob/master/pkg/heuristic/timestamp.go
// All line matching rules must only match the beginning of a line, so when adding new expressions
// make sure to prepend it with `^`
var formatsToTry = []*regexp.Regexp{
	// time.RFC3339,
	regexp.MustCompile(`^\d+-\d+-\d+T\d+:\d+:\d+(\.\d+)?(Z\d*:?\d*)?`),
	// time.ANSIC,
	regexp.MustCompile(`^[A-Za-z_]+ [A-Za-z_]+ +\d+ \d+:\d+:\d+ \d+`),
	// time.UnixDate,
	regexp.MustCompile(`^[A-Za-z_]+ [A-Za-z_]+ +\d+ \d+:\d+:\d+( [A-Za-z_]+ \d+)?`),
	// time.RubyDate,
	regexp.MustCompile(`^[A-Za-z_]+ [A-Za-z_]+ \d+ \d+:\d+:\d+ [\-\+]\d+ \d+`),
	// time.RFC822,
	regexp.MustCompile(`^\d+ [A-Za-z_]+ \d+ \d+:\d+ [A-Za-z_]+`),
	// time.RFC822Z,
	regexp.MustCompile(`^\d+ [A-Za-z_]+ \d+ \d+:\d+ -\d+`),
	// time.RFC850,
	regexp.MustCompile(`^[A-Za-z_]+, \d+-[A-Za-z_]+-\d+ \d+:\d+:\d+ [A-Za-z_]+`),
	// time.RFC1123,
	regexp.MustCompile(`^[A-Za-z_]+, \d+ [A-Za-z_]+ \d+ \d+:\d+:\d+ [A-Za-z_]+`),
	// time.RFC1123Z,
	regexp.MustCompile(`^[A-Za-z_]+, \d+ [A-Za-z_]+ \d+ \d+:\d+:\d+ -\d+`),
	// time.RFC3339Nano,
	regexp.MustCompile(`^\d+-\d+-\d+[A-Za-z_]+\d+:\d+:\d+\.\d+[A-Za-z_]+\d+:\d+`),
	// 2021-07-08 05:08:19,214
	regexp.MustCompile(`^\d+-\d+-\d+ \d+:\d+:\d+(,\d+)?`),
	// Default java logging SimpleFormatter date format
	regexp.MustCompile(`^[A-Za-z_]+ \d+, \d+ \d+:\d+:\d+ (AM|PM)`),
	// 2021-01-31 - with stricter matching around the months/days
	regexp.MustCompile(`^\d{4}-(1[012]|0?[1-9])-([12][0-9]|3[01]|0?[1-9])`),
}
