// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"errors"
	"fmt"
	"strings"
)

// ValidateLevelRules reports whether spec is a valid log level
// specification, as accepted by ParseLevelRules.
func ValidateLevelRules(spec string) error {
	// The module prefix only affects how relative patterns are resolved,
	// never whether a specification is valid, so any non-empty value works.
	_, err := ParseLevelRules(spec, "dummy")
	return err
}

// ParseLevelRules parses a log level specification into a LevelRules.
//
// A specification is a comma-separated list of instructions. Each
// instruction is either a bare level (e.g. "debug"), which sets the default
// level applied to any package not selected by a more specific instruction,
// or "<pattern>=<level>", which overrides the level for the packages
// selected by pattern:
//
//   - "some/pkg/path" selects exactly that package.
//   - "some/pkg/path/..." selects that package and any of its subpackages.
//   - "./relative/path" and "./relative/path/..." are the same as above,
//     but relative to modulePrefix.
//   - "." alone refers to modulePrefix itself.
//
// When several instructions select the same package, the most specific one
// wins. At most one bare level may be given; if none is given, the default
// level is InfoLvl.
func ParseLevelRules(spec, modulePrefix string) (*LevelRules, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, errors.New("empty log level specification")
	}

	defaultLevel := InfoLvl
	haveDefault := false
	haveInstruction := false
	var rules []rule

	for rawInstruction := range strings.SplitSeq(spec, ",") {
		instruction := strings.TrimSpace(rawInstruction)
		if instruction == "" {
			continue
		}
		haveInstruction = true

		pattern, levelStr, hasPattern := strings.Cut(instruction, "=")
		if !hasPattern {
			if haveDefault {
				return nil, fmt.Errorf("log level specification %q: only one bare level is allowed, found a second one: %q", spec, instruction)
			}
			lvl, err := ValidateLogLevel(strings.TrimSpace(instruction))
			if err != nil {
				return nil, fmt.Errorf("log level specification %q: %w", spec, err)
			}
			defaultLevel = lvl
			haveDefault = true
			continue
		}

		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return nil, fmt.Errorf("log level specification %q: empty package pattern in instruction %q", spec, instruction)
		}

		lvl, err := ValidateLogLevel(strings.TrimSpace(levelStr))
		if err != nil {
			return nil, fmt.Errorf("log level specification %q: %w", spec, err)
		}

		r, err := parsePackagePattern(pattern, modulePrefix, lvl)
		if err != nil {
			return nil, fmt.Errorf("log level specification %q: %w", spec, err)
		}
		rules = append(rules, r)
	}

	if !haveInstruction {
		return nil, fmt.Errorf("log level specification %q: no instructions found", spec)
	}

	return newLevelRules(defaultLevel, spec, rules...), nil
}

func parsePackagePattern(pattern, modulePrefix string, level LogLevel) (rule, error) {
	if pattern == "." {
		if modulePrefix == "" {
			return rule{}, errors.New("relative package pattern requires a non-empty module prefix")
		}
		pattern = modulePrefix
	} else if relative, hasPrefix := strings.CutPrefix(pattern, "./"); hasPrefix {
		if modulePrefix == "" {
			return rule{}, errors.New("relative package pattern requires a non-empty module prefix")
		}
		pattern = modulePrefix + "/" + relative
	}

	trimmed, recursive := strings.CutSuffix(pattern, "/...")
	if recursive {
		pattern = trimmed
	}

	return rule{prefix: pattern, recursive: recursive, level: level}, nil
}
