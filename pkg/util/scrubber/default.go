// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// DefaultScrubber is the scrubber used by the package-level cleaning functions.
//
// It includes a set of agent-specific replacers defined in default.go.
var DefaultScrubber = &Scrubber{}

func init() {
	AddDefaultReplacers(DefaultScrubber)
}

// AddDefaultReplacers to a scrubber. This is called automatically for
// DefaultScrubber, but can be used to initialize other, custom scrubbers with
// the default replacers.
func AddDefaultReplacers(scrubber *Scrubber) {
	hintedAPIKeyReplacer := Replacer{
		// If hinted, mask the value regardless if it doesn't match 32-char hexadecimal string
		Regex: regexp.MustCompile(`(api_?key=)\b[a-zA-Z0-9]+([a-zA-Z0-9]{5})\b`),
		Hints: []string{"api_key", "apikey"},
		Repl:  []byte(`$1***************************$2`),
	}
	hintedAPPKeyReplacer := Replacer{
		// If hinted, mask the value regardless if it doesn't match 40-char hexadecimal string
		Regex: regexp.MustCompile(`(ap(?:p|plication)_?key=)\b[a-zA-Z0-9]+([a-zA-Z0-9]{5})\b`),
		Hints: []string{"app_key", "appkey", "application_key"},
		Repl:  []byte(`$1***********************************$2`),
	}
	apiKeyReplacer := Replacer{
		Regex: regexp.MustCompile(`\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b`),
		Repl:  []byte(`***************************$1`),
	}
	appKeyReplacer := Replacer{
		Regex: regexp.MustCompile(`\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b`),
		Repl:  []byte(`***********************************$1`),
	}
	// URI Generic Syntax
	// https://tools.ietf.org/html/rfc3986
	uriPasswordReplacer := Replacer{
		Regex: regexp.MustCompile(`([A-Za-z][A-Za-z0-9+-.]+\:\/\/|\b)([^\:]+)\:([^\s]+)\@`),
		Repl:  []byte(`$1$2:********@`),
	}
	passwordReplacer := Replacer{
		Regex: matchYAMLKeyPart(`(pass(word)?|pwd)`),
		Hints: []string{"pass", "pwd"},
		Repl:  []byte(`$1 ********`),
	}
	tokenReplacer := Replacer{
		Regex: matchYAMLKeyEnding(`token`),
		Hints: []string{"token"},
		Repl:  []byte(`$1 ********`),
	}
	snmpReplacer := Replacer{
		Regex: matchYAMLKey(`(community_string|authKey|privKey|community|authentication_key|privacy_key)`),
		Hints: []string{"community_string", "authKey", "privKey", "community", "authentication_key", "privacy_key"},
		Repl:  []byte(`$1 ********`),
	}
	certReplacer := Replacer{
		Regex: matchCert(),
		Hints: []string{"BEGIN"},
		Repl:  []byte(`********`),
	}
	scrubber.AddReplacer(SingleLine, hintedAPIKeyReplacer)
	scrubber.AddReplacer(SingleLine, hintedAPPKeyReplacer)
	scrubber.AddReplacer(SingleLine, apiKeyReplacer)
	scrubber.AddReplacer(SingleLine, appKeyReplacer)
	scrubber.AddReplacer(SingleLine, uriPasswordReplacer)
	scrubber.AddReplacer(SingleLine, passwordReplacer)
	scrubber.AddReplacer(SingleLine, tokenReplacer)
	scrubber.AddReplacer(SingleLine, snmpReplacer)
	scrubber.AddReplacer(MultiLine, certReplacer)
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

// ScrubURL sanitizes credentials from a message containing a URL, and returns
// a string that can be logged safely, using the default scrubber.
func ScrubURL(url string) string {
	return DefaultScrubber.ScrubURL(url)
}

// AddStrippedKeys adds to the set of YAML keys that will be recognized and have
// their values stripped.  This modifies the DefaultScrubber directly.
func AddStrippedKeys(strippedKeys []string) {
	if len(strippedKeys) > 0 {
		configReplacer := Replacer{
			Regex: matchYAMLKey(fmt.Sprintf("(%s)", strings.Join(strippedKeys, "|"))),
			Hints: strippedKeys,
			Repl:  []byte(`$1 ********`),
		}
		DefaultScrubber.AddReplacer(SingleLine, configReplacer)
	}
}

// NewWriter instantiates a Writer to the given file path with the given
// permissions, using the default scrubber.
func NewWriter(path string, perms os.FileMode) (*Writer, error) {
	return newWriterWithScrubber(path, perms, DefaultScrubber)
}
