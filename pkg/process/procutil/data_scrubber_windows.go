// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package procutil

import (
	"strings"
)

// Extensions in which windows defaults the exec commands. Additional extensions can be included. 
var (
	winDotExec= []string{
		".com",".exe",".bat",".cmd",".vbs", ".vbe",".js",".jse",".wsf",".wsh",".psc1", ".ps1"} 
)

// stripWindowsArgs removes the arguments of the commands if they identify any of the file extensions that windows defaults to.
// If the command doesn't identify the extension and cannot split the exec command from the args, it will return the existing characters until the first empty space occurrence.

func (ds *DataScrubber) stripArguments(cmdline []string)[]string {
	
	argLength := len(cmdline)
	
		strCmdline := (cmdline[0] + " ")
	
	// case 1: Uses extensionParser() to format and search the command. 
		if argLength == 1 && !strings.HasPrefix(strCmdline, "\"") {
			strippedCmdline := extensionParser(strCmdline, winDotExec)
			cmdline = cleanUp(strippedCmdline)
			return cmdline
		}
		}
	// case 2: Uses quotesFinder() to search for any existing pair of double quotes ("") existing in the string as characters and return the content between them. 
		if argLength == 1 && strings.HasPrefix(strCmdline, "\""){
			strippedCmdline := quotesFinder(strCmdline)
			cmdline = cleanUp(strippedCmdline)
			return cmdline
	
		}
	// case 3: Process a cmdline with multiple strings and use extensionParser() to format and search the command. 
		if 	argLength > 1 && !strings.HasPrefix(strCmdline, "\""){
			strippedCmdline := extensionParser(strCmdline, winDotExec)
			cmdline = cleanUp(strippedCmdline)
			return cmdline
					
		}
	
		return cmdline
	}	
	
	// remove any extra space at the end cmdline before been returned to Scrubber 
	func cleanUp(strippedCmdline string)[]string{
		cmdline := []string{strings.TrimSuffix(strippedCmdline," ")}
		return cmdline
	}

	// Iterate through the cmdline to identify any match with any item of winDotExec[n] and remove the characters after any occurrence.
	
	func extensionParser (cmdline string, winDotExec []string) string{

		var i int
	
		var processedCmdline string
	
		for _, c := range winDotExec {
			if i = strings.Index(cmdline, c+" " ); i != -1 {
				processedCmdline = cmdline[:i+len(c)]
				return processedCmdline
			}
			processedCmdline = strings.Split(cmdline, c)[0]
		}
		return processedCmdline
	}
	
	func quotesFinder(cmdline string)string{
		
		firstQuoteRemoved := cmdline[1:]
	
		strippedCmdline	:= strings.Split(firstQuoteRemoved, "\"")[0]
	
		return strippedCmdline
	}
