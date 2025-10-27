// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"regexp"
	"strings"
	"sync"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// infraBasicAllowedChecks defines the default allowed checks and patterns in infra basic mode.
var infraBasicAllowedChecks = []string{
	"cpu",
	"agent_telemetry",
	"agentcrashdetect",
	"disk",
	"file_handle",
	"filehandles",
	"io",
	"load",
	"memory",
	"network",
	"ntp",
	"process",
	"service_discovery",
	"system",
	"system_core",
	"system_swap",
	"telemetry",
	"telemetryCheck",
	"uptime",
	"win32_event_log",
	"wincrashdetect",
	"winkmem",
	"winproc",
	"custom_.*", // Allow all custom checks
}

type compiledPattern struct {
	pattern string         // Original pattern string
	regex   *regexp.Regexp // Compiled regex (nil if exact match)
}

var (
	// Cache of compiled patterns from configuration
	compiledPatternsCache      []*compiledPattern
	compiledPatternsCacheKey   string
	compiledPatternsCacheMutex sync.RWMutex
)

// compilePatternList compiles a list of patterns (both exact matches and regex)
func compilePatternList(patterns []string) []*compiledPattern {
	compiled := make([]*compiledPattern, 0, len(patterns))
	for _, pattern := range patterns {
		cp := &compiledPattern{
			pattern: pattern,
		}

		if !strings.ContainsAny(pattern, ".*+?[]{}()^$|\\") {
			// Simple exact match, no regex needed (leave regex as nil)
			cp.regex = nil
		} else {
			re, err := regexp.Compile(pattern)
			if err != nil {
				log.Warnf("Invalid regex pattern: '%s', error: %v", pattern, err)
				continue
			}
			cp.regex = re
		}

		compiled = append(compiled, cp)
	}
	return compiled
}

// getCompiledPatterns returns the cached list of compiled patterns from both static and config sources.
// This ensures we only compile patterns once and reuse them across all check validations.
func getCompiledPatterns(cfg pkgconfigmodel.Reader) []*compiledPattern {
	additionalChecks := cfg.GetStringSlice("infra_basic_additional_checks")

	// Create a cache key from the additional patterns list
	cacheKey := strings.Join(additionalChecks, "|")

	// Check if we have a cached version
	compiledPatternsCacheMutex.RLock()
	if compiledPatternsCacheKey == cacheKey && compiledPatternsCache != nil {
		patterns := compiledPatternsCache
		compiledPatternsCacheMutex.RUnlock()
		return patterns
	}
	compiledPatternsCacheMutex.RUnlock()

	// Need to compile patterns
	compiledPatternsCacheMutex.Lock()
	defer compiledPatternsCacheMutex.Unlock()

	// Update cache
	compiledPatternsCache = compilePatternList(append(infraBasicAllowedChecks, additionalChecks...))
	compiledPatternsCacheKey = cacheKey

	return compiledPatternsCache
}

// matchesAnyPattern checks if a check name matches any of the compiled patterns
func matchesAnyPattern(checkName string, patterns []*compiledPattern) bool {
	for _, cp := range patterns {
		if cp.regex == nil {
			// Exact match
			if checkName == cp.pattern {
				return true
			}
		} else {
			// Regex match
			if cp.regex.MatchString(checkName) {
				return true
			}
		}
	}
	return false
}

// IsCheckAllowed returns true if the check is allowed.
// When not in basic mode, all checks are allowed (returns true).
// When in basic mode, only checks matching the allowed patterns are permitted.
// Patterns can be exact check names or regex patterns (e.g., "custom_.*" to match all checks starting with "custom_").
func IsCheckAllowed(checkName string, cfg pkgconfigmodel.Reader) bool {
	// When not in basic mode, all checks are allowed
	if cfg.GetString("infrastructure_mode") != "basic" {
		return true
	}

	// Check if it matches any pattern from static list or config
	patterns := getCompiledPatterns(cfg)
	return matchesAnyPattern(checkName, patterns)
}
