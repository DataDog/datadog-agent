// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package procutil

import (
	"strings"
	"unicode"
)

// Extensions in which windows defaults the exec commands
var (
	winDotExec= []string{
		".com ",".exe ",".bat ",".cmd ",".vbs ", ".vbe ",".js ",".jse ",".wsf ",".wsh ",".psc1 "}
)


// stripWindowsArgs removes the arguments of the commands if identifies on them any of the files extensions that windows defaults for exec.
// If the commands doesn't identify the extension will return those strings existing before the first space. 

func stripWindowsArgs(cmdline []string, winDotExec []string)[]string {
	
		cmdlineLength := len(cmdline)
	
		switch{
		case cmdlineLength == 1: 
	
			strCmdline := cmdline[0]
	
			validCmdLine := validWindowsCommand(strCmdline)
	
			cmdline = extensionParser(validCmdLine, winDotExec)
	
			return cmdline
	
		case cmdlineLength > 1:
			
			pathExec := cmdline [0]
	
			cmdline = []string {pathExec}
	
			return cmdline
	
			}
	
		return cmdline
	}
	
	func extensionParser (validCmdline string, winDotExec []string) []string{
	
		var i int
	
		cmdline := validCmdline
		strippedcmdline := []string{}
	
		for _, c := range winDotExec {
	
			if i = strings.Index(validCmdline, c); i != -1 {
				strippedcmdline = []string{cmdline[:i+len(c)]}
				return strippedcmdline
				} else{
				strippedcmdline = []string{strings.Split(validCmdline, " ")[0]}
				}
			}
		return strippedcmdline
	}
	
// Remove any quotation mark taken in the strings as a character.
// Example: For ""C:\Program Files\Datadog\agent.wsf" check process" output =>> "C:\Program Files\Datadog\agent.wsf check process"

	func validWindowsCommand(cmdline string, ) string {
	
		for _, c := range cmdline {
			if unicode.IsLetter(c) {
				break
			} else {
	
				validCmdline := regexp.MustCompile(`["]+`).ReplaceAllString(cmdline, "")
	
				return validCmdline
			}
		}
	
		return cmdline
	
	}	
