// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import "regexp"

// LICENSE and TOKEN only match suffixes because they can namespace safe settings.
var secretEnvVarNameRegexp = regexp.MustCompile(
	`(^|_)(ACCESS_KEY|CERTIFICATE(_CHAIN)?|CERTIFICATES?|CREDENTIALS?|JAAS|KEY|` +
		`PASS(PHRASE|WD)?|PASSWORDS?|PRIVATE|PWD|SECRET)(_|$)|` +
		`(^|_)(LICENSE|TOKENS?)$`,
)

// IsSecretEnvVarName applies the common secret-name policy used by environment readers.
func IsSecretEnvVarName(name string) bool {
	// This PostgreSQL setting selects an encryption algorithm; it does not carry
	// a password. Keep the exception exact so other password-shaped names retain
	// the common secret filtering policy.
	if name == "POSTGRESQL_PASSWORD_ENCRYPTION" {
		return false
	}
	return secretEnvVarNameRegexp.MatchString(name)
}
