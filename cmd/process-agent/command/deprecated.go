// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"fmt"
	"io"
	"strings"
)

func deprecatedFlagWarning(deprecated, replaceWith string) string {
	return fmt.Sprintf("WARNING: `%s` argument is deprecated and will be removed in a future version. Please use `%s` instead.\n", deprecated, replaceWith)
}

type replaceFlag struct {
	hint string
	args []string
}

type replaceFlagFunc func(arg string, flag string) replaceFlag

func replaceFlagPosix(arg string, flag string) replaceFlag {
	newFlag := "-" + flag
	return replaceFlag{
		hint: newFlag,
		args: []string{
			strings.Replace(arg, flag, newFlag, 1),
		},
	}
}

func replaceFlagExact(replaceWith string) replaceFlagFunc {
	return func(arg string, flag string) replaceFlag {
		return replaceFlag{
			hint: replaceWith,
			args: []string{strings.Replace(arg, flag, replaceWith, 1)},
		}
	}
}

func replaceFlagSubCommandPosArg(replaceWith string) replaceFlagFunc {
	return func(arg string, _ string) replaceFlag {
		parts := strings.SplitN(arg, "=", 2)
		parts[0] = replaceWith
		return replaceFlag{
			hint: replaceWith,
			args: parts,
		}
	}
}

// FixDeprecatedFlags transforms args so that they are conforming to the new CLI structure:
// - replace non-posix flags posix flags
// - replace deprecated flags with their command equivalents
// - display warnings when deprecated flags are encountered
func FixDeprecatedFlags(args []string, w io.Writer) []string {
	var (
		replaceCfgPath = replaceFlagExact("--cfgpath")
		replaceVersion = replaceFlagExact("version")
		replaceCheck   = replaceFlagSubCommandPosArg("check")
	)

	deprecatedFlags := map[string]replaceFlagFunc{
		// Global flags
		"-config":          replaceCfgPath,
		"--config":         replaceCfgPath,
		"-sysprobe-config": replaceFlagPosix,
		"-version":         replaceVersion,
		"--version":        replaceVersion,
		"-check":           replaceCheck,
		"--check":          replaceCheck,
		"-pid":             replaceFlagPosix,
		"-info":            replaceFlagPosix,
		// Windows flags
		"-start-service": replaceFlagPosix,
		"-stop-service":  replaceFlagPosix,
		"-foreground":    replaceFlagPosix,
	}

	var newArgs []string
	for _, arg := range args {
		var replaced bool

		for f, replacer := range deprecatedFlags {
			if strings.HasPrefix(arg, f) {
				replacement := replacer(arg, f)
				newArgs = append(newArgs, replacement.args...)

				fmt.Fprint(w, deprecatedFlagWarning(f, replacement.hint))
				replaced = true
				break
			}
		}

		if !replaced {
			newArgs = append(newArgs, arg)
		}
	}
	return newArgs
}
