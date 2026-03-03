// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package scrubber implements support for cleaning sensitive information out of strings
// and files.
//
// # Compatibility
//
// This module's API is not yet stable, and may change incompatibly from version to version.
package scrubber

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/version"
)

// Replacer represents a replacement of sensitive information with a "clean" version.
type Replacer struct {
	// Regex must match the sensitive information
	Regex *regexp.Regexp
	// YAMLKeyRegex matches the key of sensitive information in a dict/map. This is used when iterating over a
	// map[string]interface{} to scrub data for all matching key before being serialized.
	YAMLKeyRegex *regexp.Regexp
	// ProcessValue is a callback to be executed when YAMLKeyRegex matches the key of a map/dict in a YAML object. The
	// value is passed to the function and replaced by the returned interface. This is useful to produce custom
	// scrubbing. Example: keeping the last 5 digit of an api key.
	ProcessValue func(data interface{}) interface{}
	// Hints, if given, are strings which must also be present in the text for the regexp to match.
	// Especially in single-line replacers, this can be used to limit the contexts where an otherwise
	// very broad Regex is actually replaced.
	Hints []string
	// Repl is the text to replace the substring matching Regex.  It can use the regexp package's
	// replacement characters ($1, etc.) (see regexp#Regexp.ReplaceAll).
	Repl []byte
	// ReplFunc, if set, is called with the matched bytes (see regexp#Regexp.ReplaceAllFunc). Only
	// one of Repl and ReplFunc should be set.
	ReplFunc func(b []byte) []byte

	// LastUpdated is the last version when the replacer was updated.
	// This is used to track when a replacer was last updated to compare with the flare version on the rapid side to decide to apply the replacer or not.
	LastUpdated *version.Version
}

func parseVersion(versionString string) *version.Version {
	v, err := version.New(versionString, "")
	if err != nil {
		panic(err)
	}
	return &v
}

// ReplacerKind modifies how a Replacer is applied
type ReplacerKind int

const (
	// SingleLine indicates to Cleaner#AddReplacer that the replacer applies to
	// single lines.
	SingleLine ReplacerKind = iota
	// MultiLine indicates to Cleaner#AddReplacer that the replacer applies to
	// entire multiline text values.
	MultiLine
)

var commentRegex = regexp.MustCompile(`^\s*#.*$`)
var blankRegex = regexp.MustCompile(`^\s*$`)

// Scrubber implements support for cleaning sensitive information out of strings
// and files.  Its intended use is to "clean" data before it is logged or
// transmitted to a remote system, so that the meaning of the data remains
// clear without disclosing any sensitive information.
//
// Scrubber works by applying a set of replacers, in order.  It first applies
// all SingleLine replacers to each non-comment, non-blank line of the input.
//
// Comments and blank lines are omitted. Comments are considered to begin with `#`.
//
// It then applies all MultiLine replacers to the entire text of the input.
type Scrubber struct {
	singleLineReplacers []Replacer
	multiLineReplacers  []Replacer

	// shouldApply is a function that can be used to conditionally apply a replacer.
	// If the function returns false, the replacer will not be applied.
	shouldApply func(repl Replacer) bool

	// preserveENC controls whether ENC[] patterns should be preserved during scrubbing.
	// When true, matches containing ENC[] patterns will not be scrubbed.
	// This is primarily used by the flare to preserve ENC value in yaml files.
	preserveENC bool
}

// New creates a new scrubber with no replacers installed.
func New() *Scrubber {
	return &Scrubber{
		singleLineReplacers: make([]Replacer, 0),
		multiLineReplacers:  make([]Replacer, 0),
	}
}

// NewWithDefaults creates a new scrubber with the default replacers installed.
func NewWithDefaults() *Scrubber {
	s := New()
	AddDefaultReplacers(s)
	return s
}

// AddReplacer adds a replacer of the given kind to the scrubber.
func (c *Scrubber) AddReplacer(kind ReplacerKind, replacer Replacer) {
	switch kind {
	case SingleLine:
		c.singleLineReplacers = append(c.singleLineReplacers, replacer)
	case MultiLine:
		c.multiLineReplacers = append(c.multiLineReplacers, replacer)
	}
}

// SetShouldApply sets a condition function to the scrubber. If the function returns false, the replacer will not be applied.
func (c *Scrubber) SetShouldApply(shouldApply func(repl Replacer) bool) {
	c.shouldApply = shouldApply
}

// SetPreserveENC enables or disables ENC[] preservation during scrubbing.
// When enabled, regex matches containing ENC[] patterns will not be scrubbed in single-line replacers.
// This is primarily used by flare to preserve ENC[] values as they also implement YAML scrubbing.
// Multiline replacers always scrub regardless of this setting to prevent security leaks.
func (c *Scrubber) SetPreserveENC(preserve bool) {
	c.preserveENC = preserve
}

// ScrubFile scrubs credentials from file given by pathname
func (c *Scrubber) ScrubFile(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var sizeHint int
	stats, err := file.Stat()
	if err == nil {
		sizeHint = int(stats.Size())
	}

	return c.scrubReader(file, sizeHint)
}

// ScrubBytes scrubs credentials from slice of bytes
func (c *Scrubber) ScrubBytes(data []byte) ([]byte, error) {
	r := bytes.NewReader(data)
	return c.scrubReader(r, r.Len())
}

// ScrubLine scrubs credentials from a single line of text.  It can be safely
// applied to URLs or to strings containing URLs. It does not run multi-line
// replacers, and should not be used on multi-line inputs.
func (c *Scrubber) ScrubLine(message string) string {
	return string(c.scrub([]byte(message), c.singleLineReplacers, true))
}

// scrubReader applies the cleaning algorithm to a Reader
func (c *Scrubber) scrubReader(file io.Reader, sizeHint int) ([]byte, error) {
	var cleanedBuffer bytes.Buffer
	if sizeHint > 0 {
		cleanedBuffer.Grow(sizeHint)
	}

	scanner := bufio.NewScanner(file)
	if sizeHint+1 > bufio.MaxScanTokenSize {
		buffer := make([]byte, 0, sizeHint+1)
		scanner.Buffer(buffer, sizeHint+1)
	}

	// First, we go through the file line by line, applying any
	// single-line replacer that matches the line.
	first := true
	for scanner.Scan() {
		b := scanner.Bytes()
		if blankRegex.Match(b) {
			cleanedBuffer.WriteRune('\n')
		} else if !commentRegex.Match(b) {
			b = c.scrub(b, c.singleLineReplacers, true)
			if !first {
				cleanedBuffer.WriteRune('\n')
			}

			cleanedBuffer.Write(b)
			first = false
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Then we apply multiline replacers on the cleaned file
	// Note: ENC[] checking is disabled for multiline replacers to prevent
	// security leaks where a large multiline match containing ENC[] would
	// preserve secrets on other lines within the same match.
	cleanedFile := c.scrub(cleanedBuffer.Bytes(), c.multiLineReplacers, false)

	return cleanedFile, nil
}

// scrub applies the given replacers to the given data.
// If isSingleLineReplacer is true, replacers will skip matches that contain valid ENC[] patterns.
// This should be true for single-line replacers and false for multiline replacers when preserving ENC.
func (c *Scrubber) scrub(data []byte, replacers []Replacer, isSingleLineReplacer bool) []byte {
	for _, repl := range replacers {
		if repl.Regex == nil {
			// ignoring YAML only replacers
			continue
		}

		if c.shouldApply != nil && !c.shouldApply(repl) {
			continue
		}

		containsHint := false
		for _, hint := range repl.Hints {
			if bytes.Contains(data, []byte(hint)) {
				containsHint = true
				break
			}
		}
		if len(repl.Hints) == 0 || containsHint {
			data = repl.Regex.ReplaceAllFunc(data, func(match []byte) []byte {
				if isSingleLineReplacer && c.preserveENC && containsValidENC(match) {
					return match
				}
				if repl.ReplFunc != nil {
					return repl.ReplFunc(match)
				}
				return repl.Regex.ReplaceAll(match, repl.Repl)
			})
		}
	}
	return data
}

// IsEnc returns true is the string match the 'ENC[...]' format
// Borrowed from comp/core/secrets/utils
func IsEnc(str string) bool {
	// trimming space and tabs
	str = strings.Trim(str, " \t")
	if strings.HasPrefix(str, "ENC[") && strings.HasSuffix(str, "]") {
		return true
	}
	return false
}

var encPattern = regexp.MustCompile(`ENC\[[^\]]*\]`)

func containsValidENC(data []byte) bool {
	return encPattern.Match(data)
}
