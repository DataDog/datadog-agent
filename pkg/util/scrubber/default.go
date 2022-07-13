// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber/comments"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber/multi"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber/replacers"
)

// DefaultScrubber is the scrubber used by the package-level cleaning functions.
//
// It includes a set of agent-specific replacers.  It can scrub DataDog App
// and API keys, passwords from URLs, and multi-line PEM-formatted TLS keys and
// certificates.  It contains special handling for YAML-like content (with
// lines of the form "key: value") and can scrub passwords, tokens, and SNMP
// community strings in such content.
//
// See default.go for details of these replacers.
var DefaultScrubber Scrubber

// defaultReplacerScrubber is the replacers.Scrubber part of the
// DefaultScrubber.  It is used to add stripped keys after the DefaultScrubber
// has been built.
var defaultReplacerScrubber *replacers.Scrubber

func init() {
	defaultReplacerScrubber = replacers.NewEmptyScrubber()
	defaultReplacerScrubber.AddDefaultReplacers()
	DefaultScrubber = multi.NewScrubber([]Scrubber{
		comments.NewScrubber(),
		defaultReplacerScrubber,
	})
}

func matchYAMLKeyPart(part string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(\s*(\w|_)*%s(\w|_)*\s*:).+`, part))
}

func matchYAMLKey(key string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(\s*%s\s*:).+`, key))
}

func matchYAMLKeyEnding(ending string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(^\s*(\w|_)*%s\s*:).+`, ending))
}

func matchCert() *regexp.Regexp {
	/*
	   Try to match as accurately as possible. RFC 7468's ABNF
	   Backreferences are not available in go, so we cannot verify
	   here that the BEGIN label is the same as the END label.
	*/
	return regexp.MustCompile(
		`-----BEGIN (?:.*)-----[A-Za-z0-9=\+\/\s]*-----END (?:.*)-----`,
	)
}

// matchYAMLKeyWithListValue matches YAML keys with array values.
// caveat: doesn't work if the array contain nested arrays. Example:
//   key: [
//    [a, b, c],
//    def]
func matchYAMLKeyWithListValue(key string) *regexp.Regexp {
	/*
				Example 1:
				network_devices:
		  		  snmp_traps:
		            community_strings:
				    - 'pass1'
				    - 'pass2'

				Example 2:
				network_devices:
		  		  snmp_traps:
				    community_strings: ['pass1', 'pass2']

				Example 3:
				network_devices:
		  		  snmp_traps:
				    community_strings: [
				    'pass1',
				    'pass2']
	*/
	return regexp.MustCompile(
		fmt.Sprintf(`(\s*%s\s*:)\s*(?:\n(?:\s+-\s+.*)*|\[(?:\n?.*?)*?\])`, key),
		/*           -----------      ---------------  -------------
		             match key(s)     |                |
		                              match multiple   match anything
		                              lines starting   enclosed between `[` and `]`
		                              with `-`
		*/
	)
}

// ScrubFile scrubs credentials from the given file, using the
// default scrubber.
func ScrubFile(filePath string) ([]byte, error) {
	return DefaultScrubber.ScrubFile(filePath)
}

// ScrubBytes scrubs credentials from the given slice of bytes,
// using the default scrubber.
func ScrubBytes(file []byte) ([]byte, error) {
	return DefaultScrubber.ScrubBytes(file)
}

// ScrubString scrubs credentials from the given string, using the default scrubber.
func ScrubString(data string) (string, error) {
	res, err := DefaultScrubber.ScrubBytes([]byte(data))
	if err != nil {
		return "", err
	}
	return string(res), nil
}

// ScrubLine scrubs credentials from a single line of text, using the default
// scrubber.  It can be safely applied to URLs or to strings containing URLs.
// It should not be used on multi-line inputs.
func ScrubLine(url string) string {
	return DefaultScrubber.ScrubLine(url)
}

// AddStrippedKeys adds to the set of YAML keys that will be recognized and have
// their values stripped.  This modifies the DefaultScrubber directly.
func AddStrippedKeys(strippedKeys []string) {
	defaultReplacerScrubber.AddStrippedKeys(strippedKeys)
}
