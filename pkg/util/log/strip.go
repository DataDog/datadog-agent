// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package log

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Replacer structure to store regex matching and replacement functions
type Replacer struct {
	Regex    *regexp.Regexp
	Hints    []string // If any of these hints do not exist in the line, then we know the regex wont match either
	Repl     []byte
	ReplFunc func(b []byte) []byte
}

var commentRegex = regexp.MustCompile(`^\s*#.*$`)
var blankRegex = regexp.MustCompile(`^\s*$`)
var singleLineReplacers, multiLineReplacers []Replacer

func init() {
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
		Regex: matchYAMLKey(`(community_string|authKey|privKey)`),
		Hints: []string{"community_string", "authKey", "privKey"},
		Repl:  []byte(`$1 ********`),
	}
	certReplacer := Replacer{
		Regex: matchCert(),
		Hints: []string{"BEGIN"},
		Repl:  []byte(`********`),
	}
	singleLineReplacers = []Replacer{apiKeyReplacer, appKeyReplacer, uriPasswordReplacer, passwordReplacer, tokenReplacer, snmpReplacer}
	multiLineReplacers = []Replacer{certReplacer}
}

func matchYAMLKeyPart(part string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(\s*(\w|_)*%s(\w|_)*\s*:).+`, part))
}

func matchYAMLKey(key string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(\s*%s\s*:).+`, key))
}

// matchYAMLKeyEnding returns a regexp matching a single YAML line with a key ending by the string passed as argument.
// The returned regexp catches only the key and not the value.
func matchYAMLKeyEnding(ending string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(\s*(\w|_)*%s\s*:).+`, ending))
}

func matchCert() *regexp.Regexp {
	/*
	   Try to match as accurately as possible RFC 7468's ABNF
	   Backreferences are not available in go, so we cannot verify
	   here that the BEGIN label is the same as the END label.
	*/
	return regexp.MustCompile(
		`-----BEGIN (?:.*)-----[A-Za-z0-9=\+\/\s]*-----END (?:.*)-----`,
	)
}

// AddStrippedKeys allows configuration keys cleaned up
func AddStrippedKeys(strippedKeys []string) {
	if len(strippedKeys) > 0 {
		configReplacer := Replacer{
			Regex: matchYAMLKey(fmt.Sprintf("(%s)", strings.Join(strippedKeys, "|"))),
			Hints: strippedKeys,
			Repl:  []byte(`$1 ********`),
		}
		singleLineReplacers = append(singleLineReplacers, configReplacer)
	}
}

// CredentialsCleanerFile scrubs credentials from file in path
func CredentialsCleanerFile(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	return credentialsCleaner(file)
}

// CredentialsCleanerBytes scrubs credentials from slice of bytes
func CredentialsCleanerBytes(file []byte) ([]byte, error) {
	r := bytes.NewReader(file)
	return credentialsCleaner(r)
}

func credentialsCleaner(file io.Reader) ([]byte, error) {
	var cleanedFile []byte

	scanner := bufio.NewScanner(file)

	// First, we go through the file line by line, applying any
	// single-line replacer that matches the line.
	first := true
	for scanner.Scan() {
		b := scanner.Bytes()
		if !commentRegex.Match(b) && !blankRegex.Match(b) && string(b) != "" {
			for _, repl := range singleLineReplacers {
				containsHint := false
				for _, hint := range repl.Hints {
					if strings.Contains(string(b), hint) {
						containsHint = true
						break
					}
				}
				if len(repl.Hints) == 0 || containsHint {
					if repl.ReplFunc != nil {
						b = repl.Regex.ReplaceAllFunc(b, repl.ReplFunc)
					} else {
						b = repl.Regex.ReplaceAll(b, repl.Repl)
					}
				}
			}
			if !first {
				cleanedFile = append(cleanedFile, byte('\n'))
			}

			cleanedFile = append(cleanedFile, b...)
			first = false
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Then we apply multiline replacers on the cleaned file
	for _, repl := range multiLineReplacers {
		containsHint := false
		for _, hint := range repl.Hints {
			if strings.Contains(string(cleanedFile), hint) {
				containsHint = true
				break
			}
		}
		if len(repl.Hints) == 0 || containsHint {
			if repl.ReplFunc != nil {
				cleanedFile = repl.Regex.ReplaceAllFunc(cleanedFile, repl.ReplFunc)
			} else {
				cleanedFile = repl.Regex.ReplaceAll(cleanedFile, repl.Repl)
			}
		}
	}

	return cleanedFile, nil
}
