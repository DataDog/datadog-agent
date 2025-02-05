// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	// DefaultScrubber is the scrubber used by the package-level cleaning functions.
	//
	// It includes a set of agent-specific replacers.  It can scrub DataDog App
	// and API keys, passwords from URLs, and multi-line PEM-formatted TLS keys and
	// certificates.  It contains special handling for YAML-like content (with
	// lines of the form "key: value") and can scrub passwords, tokens, and SNMP
	// community strings in such content.
	//
	// See default.go for details of these replacers.
	DefaultScrubber = &Scrubber{}

	defaultReplacement = "********"

	// dynamicReplacers are replacers added at runtime. New Replacer can be added through configuration or by the
	// secrets package for example.
	dynamicReplacers      = []Replacer{}
	dynamicReplacersMutex = sync.Mutex{}

	// defaultVersion is the first version of the agent scrubber.
	// https://github.com/DataDog/datadog-agent/pull/9618
	defaultVersion = "7.33.0"
)

func init() {
	AddDefaultReplacers(DefaultScrubber)
}

// AddDefaultReplacers to a scrubber. This is called automatically for
// DefaultScrubber, but can be used to initialize other, custom scrubbers with
// the default replacers.
func AddDefaultReplacers(scrubber *Scrubber) {
	hintedAPIKeyReplacer := matchRegularExpression(`(api_?key=)\b[a-zA-Z0-9]+([a-zA-Z0-9]{5})\b`, false, []string{"api_key", "apikey"}, []byte(`$1***************************$2`), defaultVersion)
	hintedAPPKeyReplacer := matchRegularExpression(`(ap(?:p|plication)_?key=)\b[a-zA-Z0-9]+([a-zA-Z0-9]{5})\b`, false, []string{"app_key", "appkey", "application_key"}, []byte(`$1***********************************$2`), defaultVersion)

	// replacers are check one by one in order. We first try to scrub 64 bytes token, keeping the last 5 digit. If
	// the token has a different size we scrub it entirely.
	hintedBearerReplacer := matchRegularExpression(`\bBearer [a-fA-F0-9]{59}([a-fA-F0-9]{5})\b`, false, []string{"bearer"}, []byte(`Bearer ***********************************************************$1`), "7.38.0")
	// For this one we match any characters
	hintedBearerInvalidReplacer := matchRegularExpression(`\bBearer\s+[^*]+\b`, false, []string{"bearer"}, []byte(`Bearer `+defaultReplacement), "7.53.0") // https://github.com/DataDog/datadog-agent/pull/23448

	apiKeyReplacerYAML := matchRegularExpression(`(\-|\:|,|\[|\{)(\s+)?\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b`, true, nil, []byte(`$1$2"***************************$3"`), "7.39.0") // https://github.com/DataDog/datadog-agent/pull/12605
	apiKeyReplacer := matchRegularExpression(`\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b`, true, nil, []byte(`***************************$1`), defaultVersion)
	appKeyReplacerYAML := matchRegularExpression(`(\-|\:|,|\[|\{)(\s+)?\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b`, true, nil, []byte(`$1$2"***********************************$3"`), "7.39.0") // https://github.com/DataDog/datadog-agent/pull/12605
	appKeyReplacer := matchRegularExpression(`\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b`, true, nil, []byte(`***********************************$1`), defaultVersion)

	rcAppKeyReplacer := matchRegularExpression(`\bDDRCM_[A-Z0-9]+([A-Z0-9]{5})\b`, true, nil, []byte(`***********************************$1`), "7.42.0") // https://github.com/DataDog/datadog-agent/pull/14681

	// URI Generic Syntax
	// https://tools.ietf.org/html/rfc3986
	uriPasswordReplacer := matchRegularExpression(`([a-z][a-z0-9+-.]+://|\b)([^:\s]+):([^\s|"]+)@`, false, nil, []byte(`$1$2:********@`), "7.62.0") // https://github.com/DataDog/datadog-agent/pull/32503

	yamlPasswordReplacer := matchYAMLKeyPart(
		`(pass(word)?|pwd)`,
		[]string{"pass", "pwd"},
		[]byte(`$1 "********"`),
		defaultVersion,
	)

	// this regex has three parts:
	// * key: case-insensitive, optionally quoted (pass | password | pswd | pwd), not anchored to match on args like --mysql_password= etc.
	// * separator: (= or :) with optional opening quote we don't want to match as part of the password
	// * password string: alphanum + special chars except quotes and semicolon
	// replace the 3rd capture group (password string) with ********
	passwordReplacer := matchRegularExpression(`(\"?(?:pass(?:word)?|pswd|pwd)\"?)((?:=| = |: )\"?)([0-9A-Za-z#!$%&()*+,\-./:<=>?@[\\\]^_{|}~]+)`, false, nil, []byte(`$1$2********`), "7.57.0") // https://github.com/DataDog/datadog-agent/pull/28144

	tokenReplacer := matchYAMLKeyEnding(
		`token`,
		[]string{"token"},
		[]byte(`$1 "********"`),
		defaultVersion,
	)
	snmpReplacer := matchYAMLKey(
		`(community_string|authKey|privKey|community|authentication_key|privacy_key|Authorization|authorization)`,
		[]string{"community_string", "authkey", "privkey", "community", "authentication_key", "privacy_key", "authorization"},
		[]byte(`$1 "********"`),
		"7.53.0", // https://github.com/DataDog/datadog-agent/pull/23515
	)
	snmpMultilineReplacer := matchYAMLKeyWithListValue(
		"(community_strings)",
		"community_strings",
		[]byte(`$1 "********"`),
		"7.34.0", // https://github.com/DataDog/datadog-agent/pull/10305
	)

	/*
		Try to match as accurately as possible. RFC 7468's ABNF
		Backreferences are not available in go, so we cannot verify
		here that the BEGIN label is the same as the END label.
	*/
	certReplacer := matchRegularExpression(`-----BEGIN (?:.*)(?:\n[0-9A-Za-z=\+\/\s]*)+-----END (?:.*)-----`, true, []string{"BEGIN"}, []byte(`********`), defaultVersion)

	// The following replacers works on YAML object only

	apiKeyYaml := matchYAMLOnly(
		`api_key`,
		func(data interface{}) interface{} {
			if apiKey, ok := data.(string); ok {
				apiKey := strings.TrimSpace(apiKey)
				if apiKey == "" {
					return ""
				}
				if len(apiKey) == 32 {
					return HideKeyExceptLastFiveChars(apiKey)
				}
			}
			return defaultReplacement
		},
		"7.44.0", // https://github.com/DataDog/datadog-agent/pull/15707
	)

	appKeyYaml := matchYAMLOnly(
		`ap(?:p|plication)_?key`,
		func(data interface{}) interface{} {
			if appKey, ok := data.(string); ok {
				appKey := strings.TrimSpace(appKey)
				if appKey == "" {
					return ""
				}
				if len(appKey) == 40 {
					return HideKeyExceptLastFiveChars(appKey)
				}
			}
			return defaultReplacement
		},
		"7.44.0", // https://github.com/DataDog/datadog-agent/pull/15707
	)

	scrubber.AddReplacer(SingleLine, hintedAPIKeyReplacer)
	scrubber.AddReplacer(SingleLine, hintedAPPKeyReplacer)
	scrubber.AddReplacer(SingleLine, hintedBearerReplacer)
	scrubber.AddReplacer(SingleLine, hintedBearerInvalidReplacer)
	scrubber.AddReplacer(SingleLine, apiKeyReplacerYAML)
	scrubber.AddReplacer(SingleLine, apiKeyReplacer)
	scrubber.AddReplacer(SingleLine, appKeyReplacerYAML)
	scrubber.AddReplacer(SingleLine, appKeyReplacer)
	scrubber.AddReplacer(SingleLine, rcAppKeyReplacer)
	scrubber.AddReplacer(SingleLine, uriPasswordReplacer)
	scrubber.AddReplacer(SingleLine, yamlPasswordReplacer)
	scrubber.AddReplacer(SingleLine, passwordReplacer)
	scrubber.AddReplacer(SingleLine, tokenReplacer)
	scrubber.AddReplacer(SingleLine, snmpReplacer)

	scrubber.AddReplacer(SingleLine, apiKeyYaml)
	scrubber.AddReplacer(SingleLine, appKeyYaml)

	scrubber.AddReplacer(MultiLine, snmpMultilineReplacer)
	scrubber.AddReplacer(MultiLine, certReplacer)

	dynamicReplacersMutex.Lock()
	for _, r := range dynamicReplacers {
		scrubber.AddReplacer(SingleLine, r)
	}
	dynamicReplacersMutex.Unlock()
}

func matchRegularExpression(expr string, caseSensitive bool, hints []string, repl []byte, lastUpdated string) Replacer {
	if !caseSensitive {
		expr = "(?i)" + expr
	}
	return Replacer{
		Regex:       regexp.MustCompile(expr),
		Hints:       hints,
		Repl:        repl,
		LastUpdated: parseVersion(lastUpdated),
	}
}

// Yaml helpers produce replacers that work on both a yaml object (aka map[interface{}]interface{}) and on a serialized
// YAML string.

func matchYAMLKeyPart(part string, hints []string, repl []byte, lastUpdated string) Replacer {
	return Replacer{
		Regex:        regexp.MustCompile(fmt.Sprintf(`(?i)(\s*(\w|_)*%s(\w|_)*\s*:).+`, part)),
		YAMLKeyRegex: regexp.MustCompile("(?i)" + part),
		Hints:        hints,
		Repl:         repl,
		LastUpdated:  parseVersion(lastUpdated),
	}
}

func matchYAMLKey(key string, hints []string, repl []byte, lastUpdated string) Replacer {
	return Replacer{
		Regex:        regexp.MustCompile(fmt.Sprintf(`(?i)(\s*%s\s*:).+`, key)),
		YAMLKeyRegex: regexp.MustCompile(fmt.Sprintf(`(?i)^%s$`, key)),
		Hints:        hints,
		Repl:         repl,
		LastUpdated:  parseVersion(lastUpdated),
	}
}

func matchYAMLKeyEnding(ending string, hints []string, repl []byte, lastUpdated string) Replacer {
	return Replacer{
		Regex:        regexp.MustCompile(fmt.Sprintf(`(?i)(^\s*(\w|_)*%s\s*:).+`, ending)),
		YAMLKeyRegex: regexp.MustCompile(fmt.Sprintf(`(?i)^.*%s$`, ending)),
		Hints:        hints,
		Repl:         repl,
		LastUpdated:  parseVersion(lastUpdated),
	}
}

// This only works on a YAML object not on serialized YAML data
func matchYAMLOnly(key string, cb func(interface{}) interface{}, lastUpdated string) Replacer {
	return Replacer{
		YAMLKeyRegex: regexp.MustCompile("(?i)" + key),
		ProcessValue: cb,
		LastUpdated:  parseVersion(lastUpdated),
	}
}

// matchYAMLKeyWithListValue matches YAML keys with array values.
// caveat: doesn't work if the array contain nested arrays.
//
// Example:
//
//	key: [
//	 [a, b, c],
//	 def]
func matchYAMLKeyWithListValue(key string, hints string, repl []byte, lastUpdated string) Replacer {
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
	return Replacer{
		Regex: regexp.MustCompile(fmt.Sprintf(`(?i)(\s*%s\s*:)\s*(?:\n(?:\s+-\s+.*)*|\[(?:\n?.*?)*?\])`, key)),
		/*                                     -----------      ---------------  -------------
		                                       match key(s)     |                |
		                                                        match multiple   match anything
		                                                        lines starting   enclosed between `[` and `]`
		                                                        with `-`
		*/
		YAMLKeyRegex: regexp.MustCompile(key),
		Hints:        []string{hints},
		Repl:         repl,
		LastUpdated:  parseVersion(lastUpdated),
	}
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

// ScrubYaml scrubs credentials from the given YAML by loading the data and scrubbing the object instead of the
// serialized string, using the default scrubber.
func ScrubYaml(data []byte) ([]byte, error) {
	return DefaultScrubber.ScrubYaml(data)
}

// ScrubYamlString scrubs credentials from the given YAML string by loading the data and scrubbing the object instead of
// the serialized string, using the default scrubber.
func ScrubYamlString(data string) (string, error) {
	res, err := DefaultScrubber.ScrubYaml([]byte(data))
	if err != nil {
		return "", err
	}
	return string(res), nil
}

// ScrubJSON scrubs credentials from the given JSON by loading the data and scrubbing the object instead of the
// serialized string, using the default scrubber.
func ScrubJSON(data []byte) ([]byte, error) {
	return DefaultScrubber.ScrubJSON(data)
}

// ScrubJSONString scrubs credentials from the given JSON string by loading the data and scrubbing the object instead of
// the serialized string, using the default scrubber.
func ScrubJSONString(data string) (string, error) {
	res, err := ScrubJSON([]byte(data))
	if err != nil {
		return "", err
	}
	return string(res), nil
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
// It does not run multi-line replacers, and should not be used on multi-line
// inputs.
func ScrubLine(url string) string {
	return DefaultScrubber.ScrubLine(url)
}

// ScrubDataObj scrubs credentials from the data interface by recursively walking over all the nodes
func ScrubDataObj(data *interface{}) {
	DefaultScrubber.ScrubDataObj(data)
}

// HideKeyExceptLastFiveChars replaces all characters in the key with "*", except
// for the last 5 characters. If the key is an unrecognized length, replace
// all of it with the default string of "*"s instead.
func HideKeyExceptLastFiveChars(key string) string {
	if len(key) != 32 && len(key) != 40 {
		return defaultReplacement
	}
	return strings.Repeat("*", len(key)-5) + key[len(key)-5:]
}

// AddStrippedKeys adds to the set of YAML keys that will be recognized and have their values stripped. This modifies
// the DefaultScrubber directly and be added to any created scrubbers.
func AddStrippedKeys(strippedKeys []string) {
	// API and APP keys are already handled by default rules
	strippedKeys = slices.Clone(strippedKeys)
	strippedKeys = slices.DeleteFunc(strippedKeys, func(s string) bool {
		return s == "api_key" || s == "app_key"
	})

	if len(strippedKeys) > 0 {
		replacer := matchYAMLKey(
			fmt.Sprintf("(%s)", strings.Join(strippedKeys, "|")),
			strippedKeys,
			[]byte(`$1 "********"`),
			version.AgentVersion,
		)
		// We add the new replacer to the default scrubber and to the list of dynamicReplacers so any new
		// scubber will inherit it.
		DefaultScrubber.AddReplacer(SingleLine, replacer)
		dynamicReplacersMutex.Lock()
		dynamicReplacers = append(dynamicReplacers, replacer)
		dynamicReplacersMutex.Unlock()
	}
}
