// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package procutil

import (
	"strings"
)

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

var winDotExec = []string{".com", ".exe", ".bat", ".cmd", ".vbs", ".vbe", ".js", ".jse", ".wsf", ".wsh", ".psc1", ".ps1"}

// stripArguments identifies windows extension and extracts command. Otherwise, returns the first element of the cmdline before first space.
func (ds *DataScrubber) stripArguments(cmdline []string) []string {
	strCmdline := (cmdline[0] + " ")

	// Case 1: OS has already completed splitting as there is one token per element.
	if len(cmdline) > 1 {
		return []string{strings.TrimSuffix(cmdline[0], " ")}
	}

	// Case 2: One string for cmdline, use extensionParser() to find first token.
	if len(cmdline) == 1 && !strings.HasPrefix(strCmdline, "\"") {
		strippedCmdline := extensionParser(strCmdline, winDotExec)

		return []string{strings.TrimSuffix(strippedCmdline, " ")}
	}

	// Case 2b: One string for cmdline and first token wrapped in quotes, use findEmbeddedQuotes() to find content between quotes.
	strippedCmdline := findEmbeddedQuotes(strCmdline)
	return []string{strings.TrimSuffix(strippedCmdline, " ")}
}

// extensionParser returns cmdline with characters after any occurance of substrings in winDotExec removed.
func extensionParser(cmdline string, winDotExec []string) string {
	var processedCmdline string
	for _, c := range winDotExec {
		// Add space to searched extension to ensure we are matching last extension (possible to have multiple periods in one filename).
		searchStr := c + " "

		if i := strings.Index(cmdline, searchStr); i != -1 {
			processedCmdline = cmdline[:i+len(c)]
			return processedCmdline
		}
	}

	if len(cmdline) > 0 {
		processedCmdline = strings.SplitN(cmdline, " ", 2)[0]
	}

	return processedCmdline
}

// findEmbeddedQuotes returns the content between the first pair of double quotes in cmdline.
// If there is no pair of double quotes found, function returns original cmdline.
// Example: Input="\"C:\\Program Files\\Datadog\\agent.vbe\" check process" check process" Output="C:\\Program Files\\Datadog\\agent.vbe"
func findEmbeddedQuotes(cmdline string) string {
	if len(cmdline) == 0 {
		return cmdline
	}

	strippedCmdline := strings.SplitN(cmdline, "\"", 3)
	if len(strippedCmdline) < 3 {
		return cmdline
	}

	return strippedCmdline[1]
}
