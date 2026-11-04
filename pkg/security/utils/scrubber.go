// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var (
	additionals = []string{"*token*", "*jwt*"}
	// procutil sensitive words can't contain '-', so headers like `DD-API-KEY: <key>` need their own pattern
	keyValuePattern = regexp.MustCompile(`(?i)((?:api|app|application)[-_]?key\s*[:=]\s*)[^\s&;,"']+`)
)

func scrubKeyValues(s string) string {
	return keyValuePattern.ReplaceAllString(s, "${1}********")
}

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
	args := make([]string, len(cmdline))
	// before the procutil patterns, as the line is re-split on spaces once they change anything
	for i, arg := range cmdline {
		args[i] = scrubKeyValues(arg)
	}
	// procutil patterns only match a key preceded by a space, and callers pass argv without argv[0]
	if scrubbed, changed := maskSensitiveValues(s.procScrubber.SensitivePatterns, " "+strings.Join(args, " ")); changed {
		args = strings.Split(scrubbed[1:], " ")
	}

	if s.regexScrubber != nil {
		for i := range args {
			args[i] = s.regexScrubber.ScrubLine(args[i])
		}
	}

	return args
}

// sensitiveMatch locates a key token and the value it masks within the scrubbed line
type sensitiveMatch struct {
	key, valueStart, valueEnd int
}

// findSensitiveMatches matches every pattern against the original line, unlike procutil which
// rewrites the line pattern by pattern: in `secrets.py --token abc`, masking `--token` as the
// value of `secrets.py` would hide the `--token` key and leak `abc`
func findSensitiveMatches(patterns []procutil.DataScrubberPattern, line string) []sensitiveMatch {
	lower := strings.ToLower(line)
	var matches []sensitiveMatch
	for _, p := range patterns {
		if !strings.Contains(lower, p.FastCheck) {
			continue
		}
		k, v := 2*p.Re.SubexpIndex("key"), 2*p.Re.SubexpIndex("value")
		for _, m := range p.Re.FindAllStringSubmatchIndex(line, -1) {
			key := line[m[k]:m[k+1]]
			matches = append(matches, sensitiveMatch{m[k] + len(key) - len(strings.TrimLeft(key, " ")), m[v], m[v+1]})
		}
	}
	slices.SortFunc(matches, func(a, b sensitiveMatch) int { return cmp.Compare(a.key, b.key) })
	return matches
}

// sensitiveValueSpans drops keys already masked as a value unless they are flags, so
// `--password secret -al` keeps `-al` while `secrets.py --token abc` still masks `abc`
func sensitiveValueSpans(line string, matches []sensitiveMatch) [][2]int {
	var spans [][2]int
	for _, m := range matches {
		masked := slices.ContainsFunc(spans, func(s [2]int) bool { return s[0] <= m.key && m.key < s[1] })
		if masked && line[m.key] != '-' {
			continue
		}
		spans = append(spans, [2]int{m.valueStart, m.valueEnd})
	}
	slices.SortFunc(spans, func(a, b [2]int) int { return cmp.Compare(a[0], b[0]) })
	return spans
}

func maskSensitiveValues(patterns []procutil.DataScrubberPattern, line string) (string, bool) {
	spans := sensitiveValueSpans(line, findSensitiveMatches(patterns, line))
	if len(spans) == 0 {
		return line, false
	}

	var sb strings.Builder
	last := 0
	for i := 0; i < len(spans); {
		start, end := spans[i][0], spans[i][1]
		for i++; i < len(spans) && spans[i][0] <= end; i++ {
			end = max(end, spans[i][1])
		}
		sb.WriteString(line[last:start])
		sb.WriteString("********")
		last = end
	}
	sb.WriteString(line[last:])
	return sb.String(), true
}

// ScrubLine scrubs the line using the proc scrubber and the regex scrubber
func (s *Scrubber) ScrubLine(line string) string {
	line = scrubKeyValues(line)
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
