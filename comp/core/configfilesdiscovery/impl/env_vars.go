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
	regexp.MustCompile(`(^|_)LICENSE(_|$)`),
	regexp.MustCompile(`(^|_)PASS(PHRASE|WD)?(_|$)`),
	regexp.MustCompile(`(^|_)PASSWORDS?(_|$)`),
	regexp.MustCompile(`(^|_)PRIVATE(_|$)`),
	regexp.MustCompile(`(^|_)SECRET(_|$)`),
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
