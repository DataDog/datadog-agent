// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package args holds args related files
package args

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ParseProcessFlags parses the process flags
func ParseProcessFlags(args []string) []string {
	flags := make([]string, 0)
	for _, arg := range args {
		if len(arg) > 1 && arg[0] == '-' {
			isFlag := true
			name := arg[1:]
			if len(name) >= 1 && name[0] == '-' {
				name = name[1:]
				isFlag = false
			}

			isOption := false
			for _, r := range name {
				isFlag = isFlag && model.IsAlphaNumeric(r)
				isOption = isOption || r == '='
			}

			if len(name) > 0 {
				if isFlag {
					for _, r := range name {
						flags = append(flags, string(r))
					}
				}
				if !isOption && len(name) > 1 {
					flags = append(flags, name)
				}
			}
		}
	}
	return flags
}

// ParseProcessOptions parses the process options
func ParseProcessOptions(args []string) []string {
	options := make([]string, 0)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if len(arg) > 1 && arg[0] == '-' {
			name := arg[1:]
			if len(name) >= 1 && name[0] == '-' {
				name = name[1:]
			}
			if len(name) > 0 && model.IsAlphaNumeric(rune(name[0])) {
				if index := strings.IndexRune(name, '='); index == -1 {
					if i < len(args)-1 && (len(args[i+1]) == 0 || args[i+1][0] != '-') {
						options = append(options, name+"="+args[i+1])
						i++
					}
				} else {
					options = append(options, name)
				}
			}
		}
	}
	return options
}
