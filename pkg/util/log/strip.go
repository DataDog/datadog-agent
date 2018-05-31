// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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

//Replacer structure to store regex matching and replacement functions
type Replacer struct {
	Regex    *regexp.Regexp
	Hints    []string // Hints to speed up regex matching -- if any of these strings exist, then it's possible that this regex might match
	Repl     []byte
	ReplFunc func(b []byte) []byte
}

var apiKeyReplacer, dockerAPIKeyReplacer, uriPasswordReplacer, passwordReplacer, tokenReplacer, snmpReplacer Replacer
var commentRegex = regexp.MustCompile(`^\s*#.*$`)
var blankRegex = regexp.MustCompile(`^\s*$`)

var replacers []Replacer

func init() {
	apiKeyReplacer := Replacer{
		Regex: regexp.MustCompile(`[a-fA-F0-9]{27}([a-fA-F0-9]{5})`),
		Repl:  []byte(`***************************$1`),
	}
	uriPasswordReplacer = Replacer{
		Regex: regexp.MustCompile(`\:\/\/([A-Za-z0-9_]+)\:(.+)\@`),
		Repl:  []byte(`://$1:********@`),
	}
	passwordReplacer = Replacer{
		Regex: matchYAMLKeyPart(`pass(word)?`),
		Hints: []string{"pass"},
		Repl:  []byte(`$1 ********`),
	}
	tokenReplacer = Replacer{
		Regex: matchYAMLKeyPart(`token`),
		Hints: []string{"token"},
		Repl:  []byte(`$1 ********`),
	}
	snmpReplacer = Replacer{
		Regex: matchYAMLKey(`(community_string|authKey|privKey)`),
		Hints: []string{"community_string", "authKey", "privKey"},
		Repl:  []byte(`$1 ********`),
	}
	replacers = []Replacer{apiKeyReplacer, uriPasswordReplacer, passwordReplacer, tokenReplacer, snmpReplacer}
}

func matchYAMLKeyPart(part string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(\s*(\w|_)*%s(\w|_)*\s*:).+`, part))
}

func matchYAMLKey(key string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(\s*%s\s*:).+`, key))
}

//CredentialsCleanerFile scrubs credentials from file in path
func CredentialsCleanerFile(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	return credentialsCleaner(file)
}

//CredentialsCleanerBytes scrubs credentials from slice of bytes
func CredentialsCleanerBytes(file []byte) ([]byte, error) {
	r := bytes.NewReader(file)
	return credentialsCleaner(r)
}

func credentialsCleaner(file io.Reader) ([]byte, error) {
	var finalFile string

	scanner := bufio.NewScanner(file)

	first := true
	for scanner.Scan() {
		b := scanner.Bytes()
		if !commentRegex.Match(b) && !blankRegex.Match(b) && string(b) != "" {
			for _, repl := range replacers {
				shouldTryRepl := len(repl.Hints) == 0
				if len(repl.Hints) > 0 { // Set this to false to make sure at least one hint exists in the log line
					shouldTryRepl = false
					for _, hint := range repl.Hints {
						if strings.Contains(string(b), hint) {
							shouldTryRepl = true
						}
					}
				}
				if shouldTryRepl {
					if repl.ReplFunc != nil {
						b = repl.Regex.ReplaceAllFunc(b, repl.ReplFunc)
					} else {
						b = repl.Regex.ReplaceAll(b, repl.Repl)
					}
				}
			}
			if !first {
				finalFile += "\n"
			}

			finalFile += string(b)
			first = false
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return []byte(finalFile), nil
}
