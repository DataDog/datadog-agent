// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build orchestrator

package orchestrator

import (
	"regexp"
	"strings"
)

var (
	defaultSensitiveWords = []string{
		"password", "passwd", "mysql_pwd",
		"access_token", "auth_token",
		"api_key", "apikey", "pwd",
		"secret", "credentials", "stripetoken"}
)

// DataScrubber allows the agent to blacklist cmdline arguments that match
// a list of predefined and custom words
type DataScrubber struct {
	Enabled                  bool
	StripAllArguments        bool
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
func (ds *DataScrubber) ContainsBlacklistedWord(word string) bool {
	for _, pattern := range ds.LiteralSensitivePatterns {
		if strings.Contains(strings.ToLower(word), pattern) {
			return true
		}
	}
	return false
}

// ScrubCommand hides the argument value for any key which matches a "sensitive word" pattern.
// It returns the updated cmdline, as well as a boolean representing whether it was scrubbed
// future: we can add a check to do regex matching or simple matching depending whether we have RegexSensitivePatterns
func (ds *DataScrubber) ScrubSimpleCommand(cmdline []string) ([]string, bool) {
	changed := false
	var wordReplacesIndexes []int
	// preprocess, without the preprocessing it is needed to strip until the whitespaces.
	var preprocessedCmdLines []string
	for _, cmd := range cmdline {
		preprocessedCmdLines = append(preprocessedCmdLines, strings.Split(cmd, " ")...)
	}
	for index, cmd := range preprocessedCmdLines {
		for _, pattern := range ds.LiteralSensitivePatterns { // this can be optimized
			// if we found a word from the list, it means that either the current or next word should be a password we want to replace.
			if strings.Contains(strings.ToLower(cmd), pattern) { //password=1234
				changed = true
				v := strings.IndexAny(cmd, "=:") //password 1234
				if v > 1 {
					// password:1234  password=1234
					preprocessedCmdLines[index] = cmd[:v+1] + "********"
					// replace from v to end of string with ********
					break
				} else {
					wordReplacesIndexes = append(wordReplacesIndexes, index+1)
					index = index + 1
					break
				}
			}
		}
	}

	// password 1234
	for _, index := range wordReplacesIndexes {
		// we still want to make sure that we are in the index e.g. the word is at the end and actually does not mean adding a password/token.
		if index < len(preprocessedCmdLines) {
			if preprocessedCmdLines != nil {
				// we only want to replace words
				for preprocessedCmdLines[index] == "" {
					index += 1
				}
				preprocessedCmdLines[index] = "********"
			}
		}
	}

	return preprocessedCmdLines, changed
}

//ScrubRegexCommand hides the argument value for any key which matches a "sensitive word" pattern.
//It returns the updated cmdline, as well as a boolean representing whether it was scrubbed
func (ds *DataScrubber) ScrubRegexCommand(cmdline []string) ([]string, bool) {
	newCmdline := cmdline
	rawCmdline := strings.Join(cmdline, " ")
	changed := false
	for _, pattern := range ds.RegexSensitivePatterns {
		if pattern.MatchString(rawCmdline) {
			changed = true
			rawCmdline = pattern.ReplaceAllString(rawCmdline, "${key}${delimiter}********")
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
// In the future we can add own regex expression
func (ds *DataScrubber) AddCustomSensitiveWords(words []string) {
	ds.LiteralSensitivePatterns = append(ds.LiteralSensitivePatterns, words...)
}
