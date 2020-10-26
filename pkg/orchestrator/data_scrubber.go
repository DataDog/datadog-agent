// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build orchestrator

package orchestrator

import (
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/config"
)

var (
	defaultSensitiveWords = []string{
		"password", "passwd", "mysql_pwd",
		"access_token", "auth_token",
		"api_key", "apikey", "pwd",
		"secret", "credentials", "stripetoken"}
)

// DataScrubber allows the agent to block cmdline arguments that match
// a list of predefined and custom words
type DataScrubber struct {
	Enabled                  bool
	RegexSensitivePatterns   []*regexp.Regexp
	LiteralSensitivePatterns []string
	scrubbedCmdlines         map[string][]string
}

// NewDefaultDataScrubber creates a DataScrubber with the default behavior: enabled
// and matching the default sensitive words
func NewDefaultDataScrubber() *DataScrubber {
	newDataScrubber := &DataScrubber{
		Enabled:                  true,
		LiteralSensitivePatterns: defaultSensitiveWords,
		scrubbedCmdlines:         make(map[string][]string),
	}

	return newDataScrubber
}
func (ds *DataScrubber) ContainsSensitiveWord(word string) bool {
	for _, pattern := range ds.LiteralSensitivePatterns {
		if strings.Contains(strings.ToLower(word), pattern) {
			return true
		}
	}
	return false
}

// ScrubSimpleCommand hides the argument value for any key which matches a "sensitive word" pattern.
// It returns the updated cmdline, as well as a boolean representing whether it was scrubbed.
func (ds *DataScrubber) ScrubSimpleCommand(cmdline []string) ([]string, bool) {
	changed := false
	regexChanged := false
	var wordReplacesIndexes []int

	rawCmdline := strings.Join(cmdline, " ")
	for _, pattern := range ds.RegexSensitivePatterns {
		if pattern.MatchString(rawCmdline) {
			regexChanged = true
			rawCmdline = pattern.ReplaceAllString(rawCmdline, "${key}${delimiter}********")
		}
	}
	newCmdline := strings.Split(rawCmdline, " ")
	// preprocess, without the preprocessing it is needed to strip until the whitespaces.
	for index, cmd := range newCmdline {
		for _, pattern := range ds.LiteralSensitivePatterns { // this can be optimized
			// if we found a word from the list, it means that either the current or next word should be a password we want to replace.
			if strings.Contains(strings.ToLower(cmd), pattern) { // password=1234
				if index == 0 {
					// the first index | should be the command name which shouldn't be matched
					continue
				}
				changed = true
				v := strings.IndexAny(cmd, "=:")
				if v > 1 {
					// password:1234  password=1234 ==> password=****** || password:******
					newCmdline[index] = cmd[:v+1] + "********"
					// replace from v to end of string with ********
					break
				} else {
					wordReplacesIndexes = append(wordReplacesIndexes, index+1)
					break
				}
			}
		}
	}
	for i := 0; i < len(wordReplacesIndexes); i++ {
		// we still want to make sure that we are in the index e.g. the word is at the end and actually does not mean adding a password/token.
		index := wordReplacesIndexes[i]
		if index < len(newCmdline) {
			// we only want to replace words
			for newCmdline[index] == "" {
				index++
			}
			if index < len(newCmdline) {
				newCmdline[index] = "********"
			}

		}
	}

	// if nothing changed, just return the input
	if !(changed || regexChanged) {
		return cmdline, false
	}

	return newCmdline, changed || regexChanged
}

// AddCustomSensitiveWords adds custom sensitive words on the DataScrubber object
// In the future we can add own regex expression
func (ds *DataScrubber) AddCustomSensitiveWords(words []string) {
	ds.LiteralSensitivePatterns = append(ds.LiteralSensitivePatterns, words...)
}

// AddCustomSensitiveRegex adds custom sensitive regex on the DataScrubber object
func (ds *DataScrubber) AddCustomSensitiveRegex(words []string) {
	r := config.CompileStringsToRegex(words)
	ds.RegexSensitivePatterns = append(ds.RegexSensitivePatterns, r...)
}
