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
	defaultVersion = parseVersion("7.33.0")
)

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

		LastUpdated: defaultVersion,
	}
	hintedAPPKeyReplacer := Replacer{
		// If hinted, mask the value regardless if it doesn't match 40-char hexadecimal string
		Regex: regexp.MustCompile(`(ap(?:p|plication)_?key=)\b[a-zA-Z0-9]+([a-zA-Z0-9]{5})\b`),
		Hints: []string{"app_key", "appkey", "application_key"},
		Repl:  []byte(`$1***********************************$2`),

		LastUpdated: defaultVersion,
	}

	// replacers are check one by one in order. We first try to scrub 64 bytes token, keeping the last 5 digit. If
	// the token has a different size we scrub it entirely.
	hintedBearerReplacer := Replacer{
		Regex: regexp.MustCompile(`\bBearer [a-fA-F0-9]{59}([a-fA-F0-9]{5})\b`),
		Hints: []string{"Bearer"},
		Repl:  []byte(`Bearer ***********************************************************$1`),

		// https://github.com/DataDog/datadog-agent/pull/12338
		LastUpdated: parseVersion("7.38.0"),
	}
	// For this one we match any characters
	hintedBearerInvalidReplacer := Replacer{
		Regex: regexp.MustCompile(`\bBearer\s+[^*]+\b`),
		Hints: []string{"Bearer"},
		Repl:  []byte("Bearer " + defaultReplacement),

		// https://github.com/DataDog/datadog-agent/pull/23448
		LastUpdated: parseVersion("7.53.0"),
	}

	apiKeyReplacerYAML := Replacer{
		Regex: regexp.MustCompile(`(\-|\:|,|\[|\{)(\s+)?\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b`),
		Repl:  []byte(`$1$2"***************************$3"`),

		// https://github.com/DataDog/datadog-agent/pull/12605
		LastUpdated: parseVersion("7.39.0"),
	}
	apiKeyReplacer := Replacer{
		Regex: regexp.MustCompile(`\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b`),
		Repl:  []byte(`***************************$1`),

		LastUpdated: defaultVersion,
	}
	appKeyReplacerYAML := Replacer{
		Regex: regexp.MustCompile(`(\-|\:|,|\[|\{)(\s+)?\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b`),
		Repl:  []byte(`$1$2"***********************************$3"`),

		// https://github.com/DataDog/datadog-agent/pull/12605
		LastUpdated: parseVersion("7.39.0"),
	}
	appKeyReplacer := Replacer{
		Regex: regexp.MustCompile(`\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b`),
		Repl:  []byte(`***********************************$1`),

		LastUpdated: defaultVersion,
	}
	rcAppKeyReplacer := Replacer{
		Regex: regexp.MustCompile(`\bDDRCM_[A-Z0-9]+([A-Z0-9]{5})\b`),
		Repl:  []byte(`***********************************$1`),

		// https://github.com/DataDog/datadog-agent/pull/14681
		LastUpdated: parseVersion("7.42.0"),
	}

	// URI Generic Syntax
	// https://tools.ietf.org/html/rfc3986
	uriPasswordReplacer := Replacer{
		Regex: regexp.MustCompile(`(?i)([a-z][a-z0-9+-.]+://|\b)([^:\s]+):([^\s|"]+)@`),
		Repl:  []byte(`$1$2:********@`),

		// https://github.com/DataDog/datadog-agent/pull/32503
		LastUpdated: parseVersion("7.62.0"),
	}

	yamlPasswordReplacer := matchYAMLKeyPart(
		`(pass(word)?|pwd)`,
		[]string{"pass", "pwd"},
		[]byte(`$1 "********"`),
	)
	yamlPasswordReplacer.LastUpdated = parseVersion("7.70.2")
	passwordReplacer := Replacer{
		// this regex has three parts:
		// * key: case-insensitive, optionally quoted (pass | password | pswd | pwd), not anchored to match on args like --mysql_password= etc.
		// * separator: (= or :) with optional opening quote we don't want to match as part of the password
		// * password string: alphanum + special chars except quotes and semicolon
		Regex: regexp.MustCompile(`(?i)(\"?(?:pass(?:word)?|pswd|pwd)\"?)((?:=| = |: )\"?)([0-9A-Za-z#!$%&()*+,\-./:<=>?@[\\\]^_{|}~]+)`),
		// replace the 3rd capture group (password string) with ********
		Repl: []byte(`$1$2********`),

		// https://github.com/DataDog/datadog-agent/pull/28144
		LastUpdated: parseVersion("7.57.0"),
	}
	tokenReplacer := matchYAMLKeyEnding(
		`token`,
		[]string{"token"},
		[]byte(`$1 "********"`),
	)
	tokenReplacer.LastUpdated = parseVersion("7.70.2")

	secretReplacer := matchYAMLKey(
		`(token_secret|consumer_secret)`,
		[]string{"token_secret", "consumer_secret"},
		[]byte(`$1 "********"`),
	)
	secretReplacer.LastUpdated = parseVersion("7.70.0") // https://github.com/DataDog/datadog-agent/pull/40345

	// OAuth credentials scrubbers for continuous_ai_netsuite and similar integrations
	consumerKeyAndTokenIDReplacer := matchYAMLKey(
		`(consumer_key|token_id)`,
		[]string{"consumer_key", "token_id"},
		[]byte(`$1 "********"`),
	)
	consumerKeyAndTokenIDReplacer.LastUpdated = parseVersion("7.70.0") // https://github.com/DataDog/datadog-agent/pull/40345

	snmpReplacer := matchYAMLKey(
		`(community_string|auth[Kk]ey|priv[Kk]ey|community|authentication_key|privacy_key|Authorization|authorization)`,
		[]string{"community_string", "authKey", "authkey", "privKey", "privkey", "community", "authentication_key", "privacy_key", "Authorization", "authorization"},
		[]byte(`$1 "********"`),
	)
	snmpReplacer.LastUpdated = parseVersion("7.64.0") // https://github.com/DataDog/datadog-agent/pull/33742
	snmpMultilineReplacer := matchYAMLKeyWithListValue(
		"(community_strings)",
		"community_strings",
		[]byte(`$1 "********"`),
	)
	snmpMultilineReplacer.LastUpdated = parseVersion("7.34.0") // https://github.com/DataDog/datadog-agent/pull/10305
	certReplacer := Replacer{
		/*
		   Try to match as accurately as possible. RFC 7468's ABNF
		   Backreferences are not available in go, so we cannot verify
		   here that the BEGIN label is the same as the END label.
		*/
		Regex: regexp.MustCompile(`-----BEGIN (?:.*)-----[A-Za-z0-9=\+\/\s]*-----END (?:.*)-----`),
		Hints: []string{"BEGIN"},
		Repl:  []byte(`********`),

		LastUpdated: defaultVersion,
	}

	// The following replacers works on YAML object only

	apiKeyYaml := matchYAMLOnly(
		`api[-_]?key`,
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
	)
	apiKeyYaml.LastUpdated = parseVersion("7.44.0") // https://github.com/DataDog/datadog-agent/pull/15707

	appKeyYaml := matchYAMLOnly(
		`ap(?:p|plication)[-_]?key`,
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
	)
	appKeyYaml.LastUpdated = parseVersion("7.44.0") // https://github.com/DataDog/datadog-agent/pull/15707

	// HTTP header-style API keys with "key" suffix
	httpHeaderKeyReplacer := matchYAMLKeyPrefixSuffix(
		`x-`,
		`key`,
		[]string{"x-api-key", "x-dreamfactory-api-key", "x-functions-key", "x-lz-api-key", "x-octopus-apikey", "x-pm-partner-key", "x-rapidapi-key", "x-sungard-idp-api-key", "x-vtex-api-appkey", "x-seel-api-key", "x-goog-api-key", "x-sonar-passcode"},
		[]byte(`$1 "********"`),
	)
	httpHeaderKeyReplacer.LastUpdated = parseVersion("7.70.2")

	// HTTP header-style API keys with "token" suffix
	httpHeaderTokenReplacer := matchYAMLKeyPrefixSuffix(
		`x-`,
		`token`,
		[]string{"x-auth-token", "x-rundeck-auth-token", "x-consul-token", "x-datadog-monitor-token", "x-vault-token", "x-vtex-api-apptoken", "x-static-token"},
		[]byte(`$1 "********"`),
	)
	httpHeaderTokenReplacer.LastUpdated = parseVersion("7.70.2")

	// HTTP header-style API keys with "auth" suffix
	httpHeaderAuthReplacer := matchYAMLKeyPrefixSuffix(
		`x-`,
		`auth`,
		[]string{"x-auth", "x-stratum-auth"},
		[]byte(`$1 "********"`),
	)
	httpHeaderAuthReplacer.LastUpdated = parseVersion("7.70.2")

	// HTTP header-style API keys with "secret" suffix
	httpHeaderSecretReplacer := matchYAMLKeyPrefixSuffix(
		`x-`,
		`secret`,
		[]string{"x-api-secret", "x-ibm-client-secret", "x-chalk-client-secret"},
		[]byte(`$1 "********"`),
	)
	httpHeaderSecretReplacer.LastUpdated = parseVersion("7.70.2")

	// Exact key matches for specific API keys and auth tokens
	exactKeyReplacer := matchYAMLKey(
		`(auth-tenantid|authority|cainzapp-api-key|cms-svc-api-key|lodauth|sec-websocket-key|statuskey|cookie|private-token|kong-admin-token|accesstoken|session_token)`,
		[]string{"auth-tenantid", "authority", "cainzapp-api-key", "cms-svc-api-key", "lodauth", "sec-websocket-key", "statuskey", "cookie", "private-token", "kong-admin-token", "accesstoken", "session_token"},
		[]byte(`$1 "********"`),
	)
	exactKeyReplacer.LastUpdated = parseVersion("7.70.2")

	scrubber.AddReplacer(SingleLine, hintedAPIKeyReplacer)
	scrubber.AddReplacer(SingleLine, hintedAPPKeyReplacer)
	scrubber.AddReplacer(SingleLine, hintedBearerReplacer)
	scrubber.AddReplacer(SingleLine, hintedBearerInvalidReplacer)
	scrubber.AddReplacer(SingleLine, httpHeaderKeyReplacer)
	scrubber.AddReplacer(SingleLine, httpHeaderTokenReplacer)
	scrubber.AddReplacer(SingleLine, httpHeaderAuthReplacer)
	scrubber.AddReplacer(SingleLine, httpHeaderSecretReplacer)
	scrubber.AddReplacer(SingleLine, exactKeyReplacer)
	scrubber.AddReplacer(SingleLine, apiKeyReplacerYAML)
	scrubber.AddReplacer(SingleLine, apiKeyReplacer)
	scrubber.AddReplacer(SingleLine, appKeyReplacerYAML)
	scrubber.AddReplacer(SingleLine, appKeyReplacer)
	scrubber.AddReplacer(SingleLine, rcAppKeyReplacer)
	scrubber.AddReplacer(SingleLine, uriPasswordReplacer)
	scrubber.AddReplacer(SingleLine, yamlPasswordReplacer)
	scrubber.AddReplacer(SingleLine, passwordReplacer)
	scrubber.AddReplacer(SingleLine, tokenReplacer)
	scrubber.AddReplacer(SingleLine, consumerKeyAndTokenIDReplacer)
	scrubber.AddReplacer(SingleLine, secretReplacer)
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

// Yaml helpers produce replacers that work on both a yaml object (aka map[interface{}]interface{}) and on a serialized
// YAML string.

func matchYAMLKeyPart(part string, hints []string, repl []byte) Replacer {
	return Replacer{
		Regex:        regexp.MustCompile(fmt.Sprintf(`(\s*(\w|_|-)*%s(\w|_|-)*\s*:).+`, part)),
		YAMLKeyRegex: regexp.MustCompile(part),
		Hints:        hints,
		Repl:         repl,
	}
}

func matchYAMLKey(key string, hints []string, repl []byte) Replacer {
	return Replacer{
		Regex:        regexp.MustCompile(fmt.Sprintf(`(\s*%s\s*:).+`, key)),
		YAMLKeyRegex: regexp.MustCompile(fmt.Sprintf(`^%s$`, key)),
		Hints:        hints,
		Repl:         repl,
	}
}

func matchYAMLKeyEnding(ending string, hints []string, repl []byte) Replacer {
	return Replacer{
		Regex:        regexp.MustCompile(fmt.Sprintf(`(^\s*(\w|_|-)*%s\s*:).+`, ending)),
		YAMLKeyRegex: regexp.MustCompile(fmt.Sprintf(`^.*%s$`, ending)),
		Hints:        hints,
		Repl:         repl,
	}
}

func matchYAMLKeyPrefixSuffix(prefix, suffix string, hints []string, repl []byte) Replacer {
	return Replacer{
		Regex:        regexp.MustCompile(fmt.Sprintf(`(\s*%s(\w|_|-)*%s\s*:).+`, prefix, suffix)),
		YAMLKeyRegex: regexp.MustCompile(fmt.Sprintf(`^%s.*%s$`, prefix, suffix)),
		Hints:        hints,
		Repl:         repl,
	}
}

// This only works on a YAML object not on serialized YAML data
func matchYAMLOnly(key string, cb func(interface{}) interface{}) Replacer {
	return Replacer{
		YAMLKeyRegex: regexp.MustCompile(key),
		ProcessValue: cb,
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
func matchYAMLKeyWithListValue(key string, hints string, repl []byte) Replacer {
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
		Regex: regexp.MustCompile(fmt.Sprintf(`(\s*%s\s*:)\s*(?:\n(?:\s+-\s+.*)*|\[(?:\n?.*?)*?\])`, key)),
		/*                                     -----------      ---------------  -------------
		                                       match key(s)     |                |
		                                                        match multiple   match anything
		                                                        lines starting   enclosed between `[` and `]`
		                                                        with `-`
		*/
		YAMLKeyRegex: regexp.MustCompile(key),
		Hints:        []string{hints},
		Repl:         repl,
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
		)
		// We add the new replacer to the default scrubber and to the list of dynamicReplacers so any new
		// scubber will inherit it.
		DefaultScrubber.AddReplacer(SingleLine, replacer)
		dynamicReplacersMutex.Lock()
		dynamicReplacers = append(dynamicReplacers, replacer)
		dynamicReplacersMutex.Unlock()
	}
}
