// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import "regexp"

var secretEnvVarNameRegexps = []*regexp.Regexp{
	regexp.MustCompile(`(^|_)ACCESS_KEY(_|$)`),
	regexp.MustCompile(`(^|_)CERTIFICATE(_CHAIN)?(_|$)`),
	regexp.MustCompile(`(^|_)CERTIFICATES?(_|$)`),
	regexp.MustCompile(`(^|_)CREDENTIALS?(_|$)`),
	regexp.MustCompile(`(^|_)JAAS(_|$)`),
	regexp.MustCompile(`(^|_)KEY(_|$)`),
	// License values end in LICENSE; LICENSE can also namespace safe settings.
	regexp.MustCompile(`(^|_)LICENSE$`),
	regexp.MustCompile(`(^|_)PASS(PHRASE|WD)?(_|$)`),
	regexp.MustCompile(`(^|_)PASSWORDS?(_|$)`),
	regexp.MustCompile(`(^|_)PRIVATE(_|$)`),
	regexp.MustCompile(`(^|_)PWD(_|$)`),
	regexp.MustCompile(`(^|_)SECRET(_|$)`),
	// Token-valued variables end in TOKEN; names such as TOKEN_ENDPOINT_URL do not.
	regexp.MustCompile(`(^|_)TOKENS?$`),
}

// IsSecretEnvVarName returns whether name contains a common secret-bearing token.
func IsSecretEnvVarName(name string) bool {
	for _, secretNameRegexp := range secretEnvVarNameRegexps {
		if secretNameRegexp.MatchString(name) {
			return true
		}
	}
	return false
}
