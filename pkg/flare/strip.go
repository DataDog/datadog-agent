// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package flare

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"regexp"
)

type replacer struct {
	regex    *regexp.Regexp
	repl     []byte
	replFunc func(b []byte) []byte
}

var apiKeyReplacer, uriPasswordReplacer, passwordReplacer, tokenReplacer, snmpReplacer replacer
var commentRegex = regexp.MustCompile(`^\s*#.*$`)
var blankRegex = regexp.MustCompile(`^\s*$`)

var replacers []replacer

func init() {
	apiKeyReplacer = replacer{
		regex: regexp.MustCompile(`(?m)^\s*api_key:( *\w+(\w{5}) ?,?)+$`),
		replFunc: func(b []byte) []byte {
			s := string(b)
			replacement := "api_key: **************************" + s[len(s)-5:]
			return []byte(replacement)
		},
	}
	uriPasswordReplacer = replacer{
		regex: regexp.MustCompile(`(.*\ [A-Za-z0-9]+)\:\/\/([A-Za-z0-9_]+)\:(.+)\@`),
		repl:  []byte(`$1://$2:********@`),
	}
	passwordReplacer = replacer{
		regex: regexp.MustCompile(`( *(\w|_)*pass(word)?:).+`),
		repl:  []byte(`$1 ********`),
	}
	tokenReplacer = replacer{
		regex: regexp.MustCompile(`( *(\w|_)*token:).+`),
		repl:  []byte(`$1 ********`),
	}
	snmpReplacer = replacer{
		regex: regexp.MustCompile(`^(\s*community_string:) *.+$`),
		repl:  []byte(`$1 ********`),
	}
	replacers = []replacer{apiKeyReplacer, uriPasswordReplacer, passwordReplacer, tokenReplacer, snmpReplacer}
}

func credentialsCleanerFile(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	return credentialsCleaner(file)
}

func credentialsCleanerBytes(file []byte) ([]byte, error) {
	r := bytes.NewReader(file)
	return credentialsCleaner(r)
}

func credentialsCleaner(file io.Reader) ([]byte, error) {
	var finalFile string

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		b := scanner.Bytes()
		if !commentRegex.Match(b) && !blankRegex.Match(b) && string(b) != "" {
			for _, repl := range replacers {
				if repl.replFunc != nil {
					b = repl.regex.ReplaceAllFunc(b, repl.replFunc)
				} else {
					b = repl.regex.ReplaceAll(b, repl.repl)
				}
			}
			finalFile += string(b) + "\n"
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return []byte(finalFile), nil
}
