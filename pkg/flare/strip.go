// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"

	log "github.com/cihub/seelog"
)

type replacer struct {
	regex    *regexp.Regexp
	repl     []byte
	replFunc func(b []byte) []byte
}

var apiKeyReplacer, dockerAPIKeyReplacer, uriPasswordReplacer, passwordReplacer, tokenReplacer, snmpReplacer replacer
var commentRegex = regexp.MustCompile(`^\s*#.*$`)
var blankRegex = regexp.MustCompile(`^\s*$`)

var replacers []replacer

func init() {
	apiKeyReplacer := replacer{
		regex: regexp.MustCompile(`[a-fA-F0-9]{27}([a-fA-F0-9]{5})`),
		repl:  []byte(`***************************$1`),
	}
	uriPasswordReplacer = replacer{
		regex: regexp.MustCompile(`\:\/\/([A-Za-z0-9_]+)\:(.+)\@`),
		repl:  []byte(`://$1:********@`),
	}
	passwordReplacer = replacer{
		regex: matchYAMLKeyPart(`pass(word)?`),
		repl:  []byte(`$1 ********`),
	}
	tokenReplacer = replacer{
		regex: matchYAMLKeyPart(`token`),
		repl:  []byte(`$1 ********`),
	}
	snmpReplacer = replacer{
		regex: matchYAMLKey(`(community_string|authKey|privKey)`),
		repl:  []byte(`$1 ********`),
	}
	replacers = []replacer{apiKeyReplacer, uriPasswordReplacer, passwordReplacer, tokenReplacer, snmpReplacer}
}

func matchYAMLKeyPart(part string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(\s*(\w|_)*%s(\w|_)*\s*:).+`, part))
}

func matchYAMLKey(key string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(\s*%s\s*:).+`, key))
}

func credentialsWriter(filename string, data []byte, perm os.FileMode) error {

	// Clean up creds
	cleaned, err := credentialsCleanerBytes(data)
	if err != nil {
		return err
	}

	err = ensureParentDirsExist(filename)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filename, cleaned, perm)
	if err != nil {
		return err
	}

	return nil
}

func credentialsWriterFromFile(filename string, filePath string, perm os.FileMode) error {
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			log.Warnf("the specified path: %s does not exist", filePath)
		}

		return err
	}

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	return credentialsWriter(filename, data, perm)

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
