// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cli provides helper functions around command line interface
package cli

import (
	"fmt"
	"io"
	"strings"
)

func deprecatedFlagWarning(deprecated, replaceWith string) string {
	return fmt.Sprintf("WARNING: `%s` argument is deprecated and will be removed in a future version. Please use `%s` instead.\n", deprecated, replaceWith)
}

// ReplaceFlag structure that will hold `Args` after replacing a flags and a `Hint` of
// the flag that was replaced.
type ReplaceFlag struct {
	Hint string
	Args []string
}

// ReplaceFlagFunc type for a function that will produce a ReplaceFlag for the provided
// `arg` and `flag`.
type ReplaceFlagFunc func(arg string, flag string) ReplaceFlag

// ReplaceFlagPosix replaces a single-dash flag with a POSIX flag.
func ReplaceFlagPosix(arg string, flag string) ReplaceFlag {
	newFlag := "-" + flag
	return ReplaceFlag{
		Hint: newFlag,
		Args: []string{
			strings.Replace(arg, flag, newFlag, 1),
		},
	}
}

// ReplaceFlagExact will replace the matching flag with `replaceWith` exactly.
func ReplaceFlagExact(replaceWith string) ReplaceFlagFunc {
	return func(arg string, flag string) ReplaceFlag {
		return ReplaceFlag{
			Hint: replaceWith,
			Args: []string{strings.Replace(arg, flag, replaceWith, 1)},
		}
	}
}

// ReplaceFlagSubCommandPosArg replaces a subcomand sub-positional argument in the
// for `--foo=bar` with `--foo bar`.
func ReplaceFlagSubCommandPosArg(replaceWith string) ReplaceFlagFunc {
	return func(arg string, _ string) ReplaceFlag {
		parts := strings.SplitN(arg, "=", 2)
		parts[0] = replaceWith
		return ReplaceFlag{
			Hint: replaceWith,
			Args: parts,
		}
	}
}

// FixDeprecatedFlags transforms args so that they are conforming to the new CLI structure:
// - replace non-posix flags posix flags
// - replace deprecated flags with their command equivalents
// - display warnings when deprecated flags are encountered
func FixDeprecatedFlags(args []string, w io.Writer, m map[string]ReplaceFlagFunc) []string {

	var newArgs []string
	for _, arg := range args {
		var replaced bool

		for f, replacer := range m {
			if strings.HasPrefix(arg, f) {
				replacement := replacer(arg, f)
				newArgs = append(newArgs, replacement.Args...)

				fmt.Fprint(w, deprecatedFlagWarning(f, replacement.Hint))
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
