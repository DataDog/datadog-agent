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

var executableExtensions = []string{".com", ".exe", ".bat", ".cmd", ".vbs", ".vbe", ".js", ".jse", ".wsf", ".wsh", ".psc1", ".ps1"}

// stripArguments identifies windows extension and extracts command. Otherwise, returns the first element of the cmdline before first space.
// If cmdline is empty, stripArguments will return an empty string.
func (ds *DataScrubber) stripArguments(cmdline []string) []string {
	if len(cmdline) < 1 {
		return cmdline
	}

	strCmdline := cmdline[0]

	// Case 1: OS has already completed splitting as there is one token per element, we return first token.
	if len(cmdline) > 1 {
		return []string{strCmdline}
	}

	// Case 2: One string for cmdline, use extensionParser() to find first token.
	if !strings.HasPrefix(strCmdline, "\"") {
		strippedCmdline := extensionParser(strCmdline, executableExtensions)

		if strippedCmdline != "" {
			return []string{strippedCmdline}
		}

		// If no extension is found, return first token of cmdline.
		return []string{strings.SplitN(strCmdline, " ", 2)[0]}
	}

	// Case 2b: One string for cmdline and first token wrapped in quotes, use findEmbeddedQuotes() to find content between quotes.
	strippedCmdline := findEmbeddedQuotes(strCmdline)
	return []string{strippedCmdline}
}

// extensionParser returns substring of cmdline up to the first extension (inclusive).
// If no extension is found, returns empty string.
// Example: Input="C:\\Program Files\\Datadog\\agent.vbe check process"  Output="C:\\Program Files\\Datadog\\agent.vbe"
func extensionParser(cmdline string, executableExtensions []string) string {
	for _, c := range executableExtensions {
		// If extension is found before a word break (space or end of line).
		if i := strings.Index(cmdline, c); i != -1 && (i+len(c) == len(cmdline) || cmdline[i+len(c)] == ' ') {
			processedCmdline := cmdline[:i+len(c)]
			return processedCmdline
		}
	}
	return ""
}

// findEmbeddedQuotes returns the content between the first pair of double quotes in cmdline.
// If there is no pair of double quotes found, function returns original cmdline.
// Example: Input="\"C:\\Program Files\\Datadog\\agent.vbe\" check process" Output="C:\\Program Files\\Datadog\\agent.vbe"
func findEmbeddedQuotes(cmdline string) string {
	strippedCmdline := strings.SplitN(cmdline, "\"", 3)
	if len(strippedCmdline) < 3 {
		return cmdline
	}

	return strippedCmdline[1]
}
