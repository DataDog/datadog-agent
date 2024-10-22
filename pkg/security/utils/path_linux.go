// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// PathPatternMatchOpts PathPatternMatch options
type PathPatternMatchOpts struct {
	WildcardLimit      int // max number of wildcard in the pattern
	PrefixNodeRequired int // number of prefix nodes required
	SuffixNodeRequired int // number of suffix nodes required
	NodeSizeLimit      int // min size required to substitute with a wildcard
}

// PathPatternMatch pattern builder for files
func PathPatternMatch(pattern string, path string, opts PathPatternMatchOpts) bool {
	var (
		i, j, offsetPath                     = 0, 0, 0
		wildcardCount, nodeCount, suffixNode = 0, 0, 0
		patternLen, pathLen                  = len(pattern), len(path)
		wildcard                             bool

		computeNode = func() bool {
			if wildcard {
				wildcardCount++
				if wildcardCount > opts.WildcardLimit {
					return false
				}
				if nodeCount < opts.PrefixNodeRequired {
					return false
				}
				if opts.NodeSizeLimit != 0 && j-offsetPath < opts.NodeSizeLimit {
					return false
				}

				suffixNode = 0
			} else {
				suffixNode++
			}

			offsetPath = j

			if i > 0 {
				nodeCount++
			}
			return true
		}
	)

	if patternLen > 0 && pattern[0] != '/' {
		return false
	}

	if pathLen > 0 && path[0] != '/' {
		return false
	}

	for i < len(pattern) && j < len(path) {
		pn, ph := pattern[i], path[j]
		if pn == '/' && ph == '/' {
			if !computeNode() {
				return false
			}
			wildcard = false

			i++
			j++
			continue
		}

		if pn != ph {
			wildcard = true
		}
		if pn != '/' {
			i++
		}
		if ph != '/' {
			j++
		}
	}

	if patternLen != i || pathLen != j {
		wildcard = true
	}

	for i < patternLen {
		if pattern[i] == '/' {
			return false
		}
		i++
	}

	for j < pathLen {
		if path[j] == '/' {
			return false
		}
		j++
	}

	if !computeNode() {
		return false
	}

	if opts.SuffixNodeRequired == 0 || suffixNode >= opts.SuffixNodeRequired {
		return true
	}

	return false
}

// PathPatternBuilder pattern builder for files
func PathPatternBuilder(pattern string, path string, opts PathPatternMatchOpts) (string, bool) {
	lenMax := len(pattern)
	if l := len(path); l > lenMax {
		lenMax = l
	}

	var (
		i, j                                 = 0, 0
		wildcardCount, nodeCount, suffixNode = 0, 0, 0
		offsetPattern, offsetPath, size      = 0, 0, 0
		patternLen, pathLen                  = len(pattern), len(path)
		wildcard                             bool
		result                               = make([]byte, lenMax)

		computeNode = func() bool {
			if wildcard {
				wildcardCount++
				if wildcardCount > opts.WildcardLimit {
					return false
				}
				if nodeCount < opts.PrefixNodeRequired {
					return false
				}
				if opts.NodeSizeLimit != 0 && j-offsetPath < opts.NodeSizeLimit {
					return false
				}

				result[size], result[size+1] = '/', '*'
				size += 2

				offsetPattern, suffixNode = i, 0
			} else {
				copy(result[size:], pattern[offsetPattern:i])
				size += i - offsetPattern
				offsetPattern = i
				suffixNode++
			}

			offsetPath = j

			if i > 0 {
				nodeCount++
			}
			return true
		}
	)

	if patternLen > 0 && pattern[0] != '/' {
		return "", false
	}

	if pathLen > 0 && path[0] != '/' {
		return "", false
	}

	for i < len(pattern) && j < len(path) {
		pn, ph := pattern[i], path[j]
		if pn == '/' && ph == '/' {
			if !computeNode() {
				return "", false
			}
			wildcard = false

			i++
			j++
			continue
		}

		if pn != ph {
			wildcard = true
		}
		if pn != '/' {
			i++
		}
		if ph != '/' {
			j++
		}
	}

	if patternLen != i || pathLen != j {
		wildcard = true
	}

	for i < patternLen {
		if pattern[i] == '/' {
			return "", false
		}
		i++
	}

	for j < pathLen {
		if path[j] == '/' {
			return "", false
		}
		j++
	}

	if !computeNode() {
		return "", false
	}

	if opts.SuffixNodeRequired == 0 || suffixNode >= opts.SuffixNodeRequired {
		return string(result[:size]), true
	}

	return "", false
}

// BuildPatterns finds and builds patterns for the path in the ruleset
func BuildPatterns(ruleset []*rules.RuleDefinition) []*rules.RuleDefinition {
	for _, rule := range ruleset {
		findAndReplacePatterns(&rule.Expression)
	}
	return ruleset
}

func findAndReplacePatterns(expression *string) {
	re := regexp.MustCompile(`\[(.*?)\]`)
	matches := re.FindAllStringSubmatch(*expression, -1)

	for _, match := range matches {
		if len(match) > 1 {
			arrayContent := match[1]
			paths := replacePatterns(strings.Split(arrayContent, ","))
			// Reconstruct the modified array as a string
			modifiedArrayString := "[" + strings.Join(paths, ", ") + "]"
			// Replace the original array with the modified array in the input string
			*expression = strings.Replace(*expression, match[0], modifiedArrayString, 1)
		}
	}
}

func replacePatterns(paths []string) []string {
	// Using a map to eliminate duplicates efficiently
	result := make(map[string]struct{})

	for _, pattern := range paths {
		strippedPattern := strings.Trim(pattern, `~" `)
		processed := false

		for _, path := range paths {
			if pattern == path {
				continue
			}

			strippedPath := strings.Trim(path, `~" `)
			pathPatternMatchOpts := determinePatternMatchOpts(strippedPath)

			finalPath, ok := PathPatternBuilder(strippedPattern, strippedPath, pathPatternMatchOpts)
			if ok {
				finalPath = fmt.Sprintf("~\"%s\"", finalPath)
				result[finalPath] = struct{}{}
				processed = true
				break // Exit the inner loop once a match is found
			}
		}

		// If no match was found, add the original pattern
		if !processed {
			result[strippedPattern] = struct{}{}
		}
	}

	// Convert the map to a slice
	finalResult := make([]string, 0, len(result))
	for path := range result {
		finalResult = append(finalResult, path)
	}

	return finalResult
}

func determinePatternMatchOpts(path string) PathPatternMatchOpts {
	pathPatternMatchOpts := PathPatternMatchOpts{
		WildcardLimit:      1,
		PrefixNodeRequired: 1,
	}

	if containsExceptions(path) {
		pathPatternMatchOpts.PrefixNodeRequired = 4
	}

	return pathPatternMatchOpts
}

func containsExceptions(path string) bool {
	exceptions := []string{"bin", "sbin"}

	for _, ex := range exceptions {
		if strings.Contains(path, ex) {
			return true
		}
	}
	return false
}
