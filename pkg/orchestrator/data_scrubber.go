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
	Enabled bool
	// RegexSensitivePatterns are custom regex patterns which are currently not exposed externally
	RegexSensitivePatterns []*regexp.Regexp
	// LiteralSensitivePatterns are custom words which use to match against
	LiteralSensitivePatterns []string
	scrubbedCmdLines         map[string][]string
}

// NewDefaultDataScrubber creates a DataScrubber with the default behavior: enabled
// and matching the default sensitive words
func NewDefaultDataScrubber() *DataScrubber {
	newDataScrubber := &DataScrubber{
		Enabled:                  true,
		LiteralSensitivePatterns: defaultSensitiveWords,
		scrubbedCmdLines:         make(map[string][]string),
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

	if len(cmdline) == 0 {
		return cmdline, false
	}

	// in case we have custom regexes we have to join them and perform regex find and replace
	rawCmdline := strings.Join(cmdline, " ")
	for _, pattern := range ds.RegexSensitivePatterns {
		if pattern.MatchString(rawCmdline) {
			regexChanged = true
			rawCmdline = pattern.ReplaceAllString(rawCmdline, "${key}${delimiter}********")
		}
	}

	newCmdline := strings.Split(rawCmdline, " ")

	// preprocess, without the preprocessing we would need to strip until whitespaces.
	// the first index can be skipped because it should be the program name.
	for index := 1; index < len(newCmdline); index++ {
		cmd := newCmdline[index]
		for _, pattern := range ds.LiteralSensitivePatterns {
			// if we found a word from the list,
			// it means either:
			// - the current after a delimiter should be a password we want to replace e.g. "agent --secret=<replace-me>" (1)
			// - the next word should be replaced.  e.g. "agent --secret <replace-me>" (1)
			// - the word is part of a special list of words, contains multiple supportedEnds and can be ignored e.g. "agent > /secret/secret" should not match (3)
			matchIndex := strings.Index(strings.ToLower(cmd), pattern)
			// password<delimiter>1234 || password || (ignore<delimiter>me<delimiter>password e.g ignore:me:password ignore/me/password)
			// agent --password======test
			// agent > /password/secret ==> agent > /password/secret
			// agent --password > /password/secret ==> agent > --password ******** /password/secret

			if matchIndex >= 0 {
				before := cmd[:matchIndex] // /etc/vaultd/ from /etc/vaultd/secret/haproxy-crt.pem
				// skip paths /etc/vaultd/secrets/haproxy-crt.pem -> we don't want to match if one of the below chars are in before
				if strings.IndexAny(before, "/:=$") >= 0 {
					break
				}

				changed = true
				v := strings.IndexAny(cmd, "=:")
				if v >= 0 {
					// password:1234  password=1234 ==> password=****** || password:******
					// password::::====1234 ==> password:******
					newCmdline[index] = cmd[:v+1] + "********"
					// replace from v to end of string with ********
					break
				} else {
					// password 1234 password ******
					nextReplacementIndex := index + 1
					if nextReplacementIndex < len(newCmdline) {
						wordReplacesIndexes = append(wordReplacesIndexes, nextReplacementIndex)
						index++
					}
					break
				}
			}
		}
	}

	for _, index := range wordReplacesIndexes {
		// we only want to replace words, hence we jump to the next index and try to scrub that instead
		for index < len(newCmdline) && newCmdline[index] == "" {
			index++
		}
		if index < len(newCmdline) {
			newCmdline[index] = "********"
		}
	}

	// if nothing changed, just return the input
	if !(changed || regexChanged) {
		return cmdline, false
	}

	return newCmdline, changed || regexChanged
}

// AddCustomSensitiveWords adds custom sensitive words on the DataScrubber object
func (ds *DataScrubber) AddCustomSensitiveWords(words []string) {
	ds.LiteralSensitivePatterns = append(ds.LiteralSensitivePatterns, words...)
}

// AddCustomSensitiveRegex adds custom sensitive regex on the DataScrubber object
func (ds *DataScrubber) AddCustomSensitiveRegex(words []string) {
	r := config.CompileStringsToRegex(words)
	ds.RegexSensitivePatterns = append(ds.RegexSensitivePatterns, r...)
}
