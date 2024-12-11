// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

func isRuneAlphanumeric(s string, position int) bool {
	return len(s) > position && (unicode.IsLetter(rune(s[position])) || unicode.IsNumber(rune(s[position])))
}

// parseExeStartWithSymbol deals with exe that starts with special chars like "(", "-" or "["
func parseExeStartWithSymbol(exe string) string {
	if exe == "" {
		return exe
	}
	// drop the first character
	result := exe[1:]
	// if last character is also special character, also drop it
	if result != "" && !isRuneAlphanumeric(result, len(result)-1) {
		result = result[:len(result)-1]
	}
	return result
}

func getExe(cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}

	exe := cmd[0]
	// check if all args are packed into the first argument
	if len(cmd) == 1 {
		if idx := strings.IndexRune(exe, ' '); idx != -1 {
			exe = exe[0:idx]
		}
	}

	// trim any quotes from the executable
	exe = strings.Trim(exe, "\"")

	// Extract executable from commandline args
	exe = removeFilePath(exe)
	if !isRuneAlphanumeric(exe, 0) {
		exe = parseExeStartWithSymbol(exe)
	}

	// For windows executables, trim the .exe suffix if there is one
	if runtime.GOOS == "windows" {
		exe = strings.TrimSuffix(exe, ".exe")
	}

	// Lowercase the exe so that we don't need to worry about case sensitivity
	exe = strings.ToLower(exe)

	return exe
}

// removeFilePath removes the base path from the string
// Note that it's behavior is os dependent
func removeFilePath(s string) string {
	if s != "" {
		return filepath.Base(s)
	}
	return s
}
