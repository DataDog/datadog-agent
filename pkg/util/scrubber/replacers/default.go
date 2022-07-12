// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package replacers

import (
	"fmt"
	"regexp"
	"strings"
)

// AddDefaultReplacers to a scrubber. This is called automatically for
// DefaultScrubber, but can be used to initialize other, custom scrubbers with
// the default replacers.
func (c *Scrubber) AddDefaultReplacers() {
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
	hintedBearerReplacer := Replacer{
		Regex: regexp.MustCompile(`\bBearer [a-fA-F0-9]{59}([a-fA-F0-9]{5})\b`),
		Hints: []string{"Bearer"},
		Repl:  []byte(`Bearer ***********************************************************$1`),
	}
	apiKeyReplacerYAML := Replacer{
		Regex: regexp.MustCompile(`(\-|\:|,|\[|\{)(\s+)?\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b`),
		Repl:  []byte(`$1$2"***************************$3"`),
	}
	apiKeyReplacer := Replacer{
		Regex: regexp.MustCompile(`\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b`),
		Repl:  []byte(`***************************$1`),
	}
	appKeyReplacerYAML := Replacer{
		Regex: regexp.MustCompile(`(\-|\:|,|\[|\{)(\s+)?\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b`),
		Repl:  []byte(`$1$2"***********************************$3"`),
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
		Repl:  []byte(`$1 "********"`),
	}
	tokenReplacer := Replacer{
		Regex: matchYAMLKeyEnding(`token`),
		Hints: []string{"token"},
		Repl:  []byte(`$1 "********"`),
	}
	snmpReplacer := Replacer{
		Regex: matchYAMLKey(`(community_string|authKey|privKey|community|authentication_key|privacy_key)`),
		Hints: []string{"community_string", "authKey", "privKey", "community", "authentication_key", "privacy_key"},
		Repl:  []byte(`$1 "********"`),
	}
	snmpMultilineReplacer := Replacer{
		Regex: matchYAMLKeyWithListValue("(community_strings)"),
		Hints: []string{"community_strings"},
		Repl:  []byte(`$1 "********"`),
	}
	certReplacer := Replacer{
		Regex: matchCert(),
		Hints: []string{"BEGIN"},
		Repl:  []byte(`********`),
	}
	c.AddReplacer(SingleLine, hintedAPIKeyReplacer)
	c.AddReplacer(SingleLine, hintedAPPKeyReplacer)
	c.AddReplacer(SingleLine, hintedBearerReplacer)
	c.AddReplacer(SingleLine, apiKeyReplacerYAML)
	c.AddReplacer(SingleLine, apiKeyReplacer)
	c.AddReplacer(SingleLine, appKeyReplacerYAML)
	c.AddReplacer(SingleLine, appKeyReplacer)
	c.AddReplacer(SingleLine, uriPasswordReplacer)
	c.AddReplacer(SingleLine, passwordReplacer)
	c.AddReplacer(SingleLine, tokenReplacer)
	c.AddReplacer(SingleLine, snmpReplacer)
	c.AddReplacer(MultiLine, snmpMultilineReplacer)
	c.AddReplacer(MultiLine, certReplacer)
}

// AddStrippedKeys adds to the set of YAML keys that will be recognized and have
// their values stripped.
func (c *Scrubber) AddStrippedKeys(strippedKeys []string) {
	if len(strippedKeys) > 0 {
		configReplacer := Replacer{
			Regex: matchYAMLKey(fmt.Sprintf("(%s)", strings.Join(strippedKeys, "|"))),
			Hints: strippedKeys,
			Repl:  []byte(`$1 "********"`),
		}
		c.AddReplacer(SingleLine, configReplacer)
	}
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
