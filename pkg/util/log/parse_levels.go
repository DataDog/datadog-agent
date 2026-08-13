// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// modulePrefix is the Go import path substituted for a leading "." in a
// package pattern given to ParseLogLevels.
const modulePrefix = "github.com/DataDog/datadog-agent"

// ParseLogLevels parses a log level specification into a types.LevelsConfig.
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
//     but relative to the datadog-agent module root
//     (github.com/DataDog/datadog-agent); "." alone refers to the module
//     root itself.
//
// When several instructions select the same package, the most specific one
// wins (see types.NewLevelsConfig). At most one bare level may be given; if
// none is given, the default level is InfoLvl.
func ParseLogLevels(spec string) (*types.LevelsConfig, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, errors.New("empty log level specification")
	}

	defaultLevel := InfoLvl
	haveDefault := false
	haveInstruction := false
	var rules []types.LevelRule

	for _, rawInstruction := range strings.Split(spec, ",") {
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
			lvl, err := ValidateLogLevel(instruction)
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

		rules = append(rules, parsePackagePattern(pattern, lvl))
	}

	if !haveInstruction {
		return nil, fmt.Errorf("log level specification %q: no instructions found", spec)
	}

	return types.NewLevelsConfig(types.ToSlogLevel(defaultLevel), rules...).WithSpec(spec), nil
}

// DefaultLevelString parses a log level specification (see ParseLogLevels)
// and returns its default level as a canonical, bare level string (e.g.
// "debug"), discarding any per-package overrides. For a plain level
// specification, this is equivalent to validating and canonicalizing it.
//
// This is meant for callers that forward the "log_level" setting to a
// consumer that only understands a single, package-agnostic level, such as
// an embedded subprocess (e.g. JMXFetch, the trace-agent APM library, or
// system-probe-lite).
func DefaultLevelString(spec string) (string, error) {
	cfg, err := ParseLogLevels(spec)
	if err != nil {
		return "", err
	}
	return types.FromSlogLevel(cfg.DefaultLevel()).String(), nil
}

// parsePackagePattern turns a package pattern (see ParseLogLevels) and its
// associated level into a types.LevelRule.
func parsePackagePattern(pattern string, level LogLevel) types.LevelRule {
	switch {
	case pattern == ".":
		pattern = modulePrefix
	case strings.HasPrefix(pattern, "./"):
		pattern = modulePrefix + "/" + pattern[len("./"):]
	}

	recursive := strings.HasSuffix(pattern, "/...")
	if recursive {
		pattern = strings.TrimSuffix(pattern, "/...")
	}

	return types.LevelRule{Prefix: pattern, Recursive: recursive, Level: types.ToSlogLevel(level)}
}
