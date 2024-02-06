// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package procutil

import (
	"strings"
)

var winDotExec = []string{".com", ".exe", ".bat", ".cmd", ".vbs", ".vbe", ".js", ".jse", ".wsf", ".wsh", ".psc1", ".ps1"}

// If the command doesn't identify the extension and cannot split the exec command from the args, it will return the existing characters until the first empty space occurrence.

func (ds *DataScrubber) stripArguments(cmdline []string) []string {

	argLength := len(cmdline)

	strCmdline := (cmdline[0] + " ")

	// case 1: Process a cmdline with multiple strings and use extensionParser() to format and search the command.
	if argLength > 1 && !strings.HasPrefix(strCmdline, "\"") {
		strippedCmdline := extensionParser(strCmdline, winDotExec)

		return []string{strings.TrimSuffix(strippedCmdline, " ")}
	}

	// case 2: Uses extensionParser() to format and search the command.
	if argLength == 1 && !strings.HasPrefix(strCmdline, "\"") {
		strippedCmdline := extensionParser(strCmdline, winDotExec)

		return []string{strings.TrimSuffix(strippedCmdline, " ")}

	}

	// case 3: Uses quotesFinder() to search for any existing pair of double quotes ("") existing in the string as characters and return the content between them.

	strippedCmdline := findEmbeddedQuotes(strCmdline)

	return []string{strings.TrimSuffix(strippedCmdline, " ")}

}

// Iterate through the cmdline to identify any match with any item of winDotExec[n] and remove the characters after any occurrence.

func extensionParser(cmdline string, winDotExec []string) string {

	var i int

	var processedCmdline string

	for _, c := range winDotExec {
		if i = strings.Index(cmdline, c+" "); i != -1 {
			processedCmdline = cmdline[:i+len(c)]
			return processedCmdline
		}
	}

	if len(cmdline) >= 1 {
		processedCmdline = strings.Split(cmdline, " ")[0]
	}

	return processedCmdline
}

// This function looks for the first pair of "" embedded in the string as a character and retrieves the content on it. Example: Input="\"C:\\Program Files\\Datadog\\agent.vbe\" check process" check process" Output="C:\\Program Files\\Datadog\\agent.vbe"

func findEmbeddedQuotes(cmdline string) string {

	var strippedCmdline string

	firstQuoteRemoved := cmdline[1:]

	splittedCmdline := strings.Split(firstQuoteRemoved, "\"")

	if len(splittedCmdline) >= 1 {
		strippedCmdline = splittedCmdline[0]
	}

	return strippedCmdline
}
