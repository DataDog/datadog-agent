// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var (
	additionals = []string{"*token*"}
)

// Scrubber is a struct that holds the proc scrubber and the regex scrubber
type Scrubber struct {
	procScrubber  *procutil.DataScrubber
	regexScrubber *scrubber.Scrubber
}

// NewScrubber creates a new scrubber
func NewScrubber(customSensitiveWords []string, regexps []string) (*Scrubber, error) {
	customSensitiveWords = append(customSensitiveWords, additionals...)
	s := &Scrubber{
		procScrubber: newProcScrubber(customSensitiveWords),
	}

	if len(regexps) > 0 {
		regexScrubber, err := newRegexScrubber(regexps)
		if err != nil {
			return nil, err
		}
		s.regexScrubber = regexScrubber
	}

	return s, nil
}

// ScrubCommand scrubs the command line using the proc scrubber and the regex scrubber
func (s *Scrubber) ScrubCommand(cmdline []string) []string {
	scrubbed, _ := s.procScrubber.ScrubCommand(cmdline)

	if s.regexScrubber != nil {
		for i := range scrubbed {
			scrubbed[i] = s.regexScrubber.ScrubLine(scrubbed[i])
		}
	}

	return scrubbed
}

// ScrubLine scrubs the line using the proc scrubber and the regex scrubber
func (s *Scrubber) ScrubLine(line string) string {
	if s.regexScrubber != nil {
		return s.regexScrubber.ScrubLine(line)
	}
	return line
}

func newProcScrubber(customSensitiveWords []string) *procutil.DataScrubber {
	scrubber := procutil.NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)

	// token is not always a sensitive word so we cannot change the default sensitive patterns
	// in the case of CWS we can assume token is something we want to scrub so we add it here
	for _, additional := range additionals {
		if !slices.Contains(customSensitiveWords, additional) {
			scrubber.AddCustomSensitiveWords([]string{additional})
		}
	}

	return scrubber
}

func newRegexScrubber(regexps []string) (*scrubber.Scrubber, error) {
	s := scrubber.New()

	addReplacer := func(expr string) error {
		r, err := regexp.Compile(expr)
		if err != nil {
			return fmt.Errorf("failed to compile regex: %w", err)
		}

		s.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
			Regex: r,
			Repl:  []byte(`*****`),
		})
		return nil
	}

	for _, regex := range regexps {
		if err := addReplacer(regex); err != nil {
			return nil, err
		}
	}

	return s, nil
}
