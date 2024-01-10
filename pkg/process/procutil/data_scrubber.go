// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package procutil

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	defaultSensitiveWords = []string{
		"*password*", "*passwd*", "*mysql_pwd*",
		"*access_token*", "*auth_token*",
		"*api_key*", "*apikey*",
		"*secret*", "*credentials*", "stripetoken", "rails"}
)

const (
	defaultCacheMaxCycles = 25
)

type processCacheKey struct {
	pid        int32
	createTime int64
}

//nolint:revive // TODO(PROC) Fix revive linter
type DataScrubberPattern struct {
	FastCheck string
	Re        *regexp.Regexp
}

// DataScrubber allows the agent to disallow-list cmdline arguments that match
// a list of predefined and custom words
type DataScrubber struct {
	Enabled           bool
	StripAllArguments bool
	SensitivePatterns []DataScrubberPattern
	seenProcess       map[processCacheKey]struct{}
	scrubbedCmdlines  map[processCacheKey][]string
	cacheCycles       uint32 // used to control the cache age
	cacheMaxCycles    uint32 // number of cycles before resetting the cache content
}

// NewDefaultDataScrubber creates a DataScrubber with the default behavior: enabled
// and matching the default sensitive words
func NewDefaultDataScrubber() *DataScrubber {
	patterns := CompileStringsToRegex(defaultSensitiveWords)
	newDataScrubber := &DataScrubber{
		Enabled:           true,
		SensitivePatterns: patterns,
		seenProcess:       make(map[processCacheKey]struct{}),
		scrubbedCmdlines:  make(map[processCacheKey][]string),
		cacheCycles:       0,
		cacheMaxCycles:    defaultCacheMaxCycles,
	}

	return newDataScrubber
}

// CompileStringsToRegex compile each word in the slice into a regex pattern to match
// against the cmdline arguments
// The word must contain only word characters ([a-zA-z0-9_]) or wildcards *
func CompileStringsToRegex(words []string) []DataScrubberPattern {
	compiledRegexps := make([]DataScrubberPattern, 0, len(words))
	forbiddenSymbols := regexp.MustCompile("[^a-zA-Z0-9_*]")

	for _, word := range words {
		if forbiddenSymbols.MatchString(word) {
			log.Warnf("data scrubber: %s skipped. The sensitive word must "+
				"contain only alphanumeric characters, underscores or wildcards ('*')", word)
			continue
		}

		if word == "*" {
			log.Warn("data scrubber: ignoring wildcard-only ('*') sensitive word as it is not supported")
			continue
		}

		originalRunes := []rune(word)
		var enhancedWord bytes.Buffer
		valid := true
		for i, rune := range originalRunes {
			if rune == '*' {
				if i == len(originalRunes)-1 {
					enhancedWord.WriteString("[^ =:]*")
				} else if originalRunes[i+1] == '*' {
					log.Warnf("data scrubber: %s skipped. The sensitive word "+
						"must not contain two consecutives '*'", word)
					valid = false
					break
				} else {
					enhancedWord.WriteString("[^\\s=:$/]*")
				}
			} else {
				enhancedWord.WriteString(string(rune))
			}
		}

		if !valid {
			continue
		}

		pattern := "(?P<key>( +| -{1,2})(?i)" + enhancedWord.String() + ")(?P<delimiter> +|=|:)(?P<value>[^\\s]*)"
		r, err := regexp.Compile(pattern)
		if err == nil {
			compiledRegexps = append(compiledRegexps, DataScrubberPattern{
				FastCheck: wordToFastChecker(word),
				Re:        r,
			})
		} else {
			log.Warnf("data scrubber: %s skipped. It couldn't be compiled into a regex expression", word)
		}
	}

	return compiledRegexps
}

// createProcessKey returns an unique identifier for a given process
func createProcessKey(p *Process) processCacheKey {
	return processCacheKey{
		pid:        p.Pid,
		createTime: p.Stats.CreateTime,
	}
}

// ScrubProcessCommand uses a cache memory to avoid scrubbing already known
// process' cmdlines
func (ds *DataScrubber) ScrubProcessCommand(p *Process) []string {
	if ds.StripAllArguments {
		return ds.stripArguments(p.Cmdline)
	}

	if !ds.Enabled {
		return p.Cmdline
	}

	pKey := createProcessKey(p)
	if _, ok := ds.seenProcess[pKey]; !ok {
		ds.seenProcess[pKey] = struct{}{}
		if scrubbed, changed := ds.ScrubCommand(p.Cmdline); changed {
			ds.scrubbedCmdlines[pKey] = scrubbed
		}
	}

	if scrubbed, ok := ds.scrubbedCmdlines[pKey]; ok {
		return scrubbed
	}
	return p.Cmdline
}

// IncrementCacheAge increments one cycle of cache memory age. If it reaches
// cacheMaxCycles, the cache is restarted
func (ds *DataScrubber) IncrementCacheAge() {
	ds.cacheCycles++
	if ds.cacheCycles == ds.cacheMaxCycles {
		ds.seenProcess = make(map[processCacheKey]struct{})
		ds.scrubbedCmdlines = make(map[processCacheKey][]string)
		ds.cacheCycles = 0
	}
}

// ScrubCommand hides the argument value for any key which matches a "sensitive word" pattern.
// It returns the updated cmdline, as well as a boolean representing whether it was scrubbed
func (ds *DataScrubber) ScrubCommand(cmdline []string) ([]string, bool) {
	newCmdline := cmdline
	rawCmdline := strings.Join(cmdline, " ")
	lowerCaseCmdline := strings.ToLower(rawCmdline)
	changed := false
	for _, pattern := range ds.SensitivePatterns {
		// fast check with direct pattern
		if !strings.Contains(lowerCaseCmdline, pattern.FastCheck) {
			continue
		}

		if pattern.Re.MatchString(rawCmdline) {
			changed = true
			rawCmdline = pattern.Re.ReplaceAllString(rawCmdline, "${key}${delimiter}********")
		}
	}

	if changed {
		newCmdline = strings.Split(rawCmdline, " ")
	}
	return newCmdline, changed
}

// Strip away all arguments from the command line
func (ds *DataScrubber) stripArguments(cmdline []string) []string {
	// We will sometimes see the entire command line come in via the first element -- splitting guarantees removal
	// of arguments in these cases.
	if len(cmdline) > 0 {
		return []string{strings.Split(cmdline[0], " ")[0]}
	}
	return cmdline
}

// AddCustomSensitiveWords adds custom sensitive words on the DataScrubber object
func (ds *DataScrubber) AddCustomSensitiveWords(words []string) {
	newPatterns := CompileStringsToRegex(words)
	ds.SensitivePatterns = append(ds.SensitivePatterns, newPatterns...)
}

// wordToFastChecker returns a string that can be used to do a first fast lookup before doing the full
// regex search
// for example `wordToFastChecker("*aa*bbb*") = "bbb"`
// if no string is found, it returns ""
func wordToFastChecker(word string) string {
	bestLen := 0
	best := ""

	for _, sub := range strings.Split(word, "*") {
		if len(sub) > bestLen {
			bestLen = len(sub)
			best = sub
		}
	}

	return strings.ToLower(best)
}
