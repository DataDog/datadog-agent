// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package procutil

import (
	"regexp"
	"strings"
	"unicode"
)

func (ds *DataScrubber) stripArguments(cmdline []string) []string {

	winDotExec := []string{".com",".exe",".bat",".cmd",".vbs", ".vbe",".js",".jse",".wsf",".wsh",".psc1"}

	if len(cmdline) > 0 {

		if len(cmdline) > 1  {

			cmdline = []string{strings.Split(cmdline[0], ",")[0]}
			return cmdline

		}else{ 

			strCmdline := strings.Join(cmdline,"")

			validCmdline := validWindowsPrefix(strCmdline)

			i := extensionParser(validCmdline, winDotExec)

			strippedcmdline := validCmdline[:i+4]
		
			slicedCmdline := []string{}
		
			cmdline := append(slicedCmdline, strippedcmdline)
		
			return cmdline					
		}	
	}
	return cmdline
}


func validWindowsPrefix(cmdline string, ) string {

	for _, c := range cmdline {
		if unicode.IsLetter(c) {
			break
		} else {
			cmdline = strings.Split(cmdline, "\"")[1]
			return cmdline
		}
	}

	return cmdline

}

func extensionParser (validCmdline string, winDotExec []string) int {

	var i int

	for _, c := range winDotExec {

		if strings.Contains(validCmdline, c) {
			i := strings.Index(validCmdline, c)
			return i
		} 
		}
	return i
}

