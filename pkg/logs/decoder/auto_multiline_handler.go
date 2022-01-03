// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	multiLineHandler  *MultiLineHandler
	singleLineHandler *SingleLineHandler
	inputChan         chan *Message
	outputChan        chan *Message
	isRunning         bool
	linesToAssess     int
	linesTested       int
	lineLimit         int
	matchThreshold    float64
	scoredMatches     []*scoredPattern
	processsingFunc   func(message *Message)
	flushTimeout      time.Duration
	source            *config.LogSource
	timeoutTimer      *time.Timer
	detectedPattern   *DetectedPattern
}

// NewAutoMultilineHandler returns a new AutoMultilineHandler.
func NewAutoMultilineHandler(outputChan chan *Message,
	lineLimit, linesToAssess int,
	matchThreshold float64,
	matchTimeout time.Duration,
	flushTimeout time.Duration,
	source *config.LogSource,
	additionalPatterns []*regexp.Regexp,
	detectedPattern *DetectedPattern,
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
		inputChan:       make(chan *Message),
		outputChan:      outputChan,
		isRunning:       true,
		lineLimit:       lineLimit,
		matchThreshold:  matchThreshold,
		scoredMatches:   scoredMatches,
		linesToAssess:   linesToAssess,
		flushTimeout:    flushTimeout,
		source:          source,
		timeoutTimer:    time.NewTimer(matchTimeout),
		detectedPattern: detectedPattern,
	}

	h.singleLineHandler = NewSingleLineHandler(outputChan, lineLimit)
	h.processsingFunc = h.processAndTry

	return h
}

// Handle puts all new lines into a channel for later processing.
func (h *AutoMultilineHandler) Handle(input *Message) {
	h.inputChan <- input
}

// Stop stops the handler.
func (h *AutoMultilineHandler) Stop() {
	close(h.inputChan)
}

// Start starts the handler.
func (h *AutoMultilineHandler) Start() {
	go h.run()
}

// run consumes new lines and processes them.
func (h *AutoMultilineHandler) run() {
	for {
		if !h.isRunning {
			return
		}
		line, isOpen := <-h.inputChan
		if !isOpen {
			close(h.outputChan)
			return
		}
		h.processsingFunc(line)
	}
}

func (h *AutoMultilineHandler) processAndTry(message *Message) {
	// Process message before anything else
	h.singleLineHandler.process(message)

	for i, scoredPattern := range h.scoredMatches {
		match := scoredPattern.regexp.Match(message.Content)
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

	timeout := false
	select {
	case <-h.timeoutTimer.C:
		log.Debug("Multiline auto detect timed out before reaching line test threshold")
		timeout = true
		break
	default:
		break
	}

	h.linesTested++
	if h.linesTested >= h.linesToAssess || timeout {
		topMatch := h.scoredMatches[0]
		matchRatio := float64(topMatch.score) / float64(h.linesTested)

		if matchRatio >= h.matchThreshold {
			log.Debugf("Pattern %v matched %d lines with a ratio of %f", topMatch.regexp.String(), topMatch.score, matchRatio)
			h.detectedPattern.Set(topMatch.regexp)
			h.switchToMultilineHandler(topMatch.regexp)
		} else {
			log.Debug("No pattern met the line match threshold during multiline autosensing - using single line handler")
			// Stay with the single line handler and no longer attempt to detect multiline matches.
			h.processsingFunc = h.singleLineHandler.process
		}
	}
}

func (h *AutoMultilineHandler) switchToMultilineHandler(r *regexp.Regexp) {
	h.isRunning = false
	h.singleLineHandler = nil

	// Build and start a multiline-handler
	h.multiLineHandler = newMultiLineHandler(h.inputChan, h.outputChan, r, h.flushTimeout, h.lineLimit)
	h.multiLineHandler.Start()

	// At this point control is handed over to the multiline handler and the AutoMultilineHandler read loop has stopped.
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
}
