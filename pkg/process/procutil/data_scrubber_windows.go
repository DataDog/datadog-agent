// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package procutil

var (
	defaultSensitiveWords = []string{
		"*password*", "*passwd*", "*mysql_pwd*",
		"*access_token*", "*auth_token*",
		"*api_key*", "*apikey*",
		"*secret*", "*credentials*", "stripetoken",
		// windows arguments
		"/p", "/rp",
	}

	// note the `/` at the beginning of the regex.  it's not an escape or a typo
	// it's for handling parameters like `/p` and `/rp` in windows
	forbiddenSymbolsRegex = "[^/a-zA-Z0-9_*]"
)
