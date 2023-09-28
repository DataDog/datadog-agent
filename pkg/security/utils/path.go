// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

// PathPatternBuilderOpts PathPatternBuilder options
type PathPatternBuilderOpts struct {
	WildcardLimit      int // max number of wildcard in the pattern
	PrefixNodeRequired int // number of prefix nodes required
	SuffixNodeRequired int // number of suffix nodes required
	NodeSizeLimit      int // min size required to substitute with a wildcard
}

// PathPatternBuilder pattern builder for files
func PathPatternBuilder(pattern string, path string, opts PathPatternBuilderOpts) (bool, string) {
	lenMax := len(pattern)
	if l := len(path); l > lenMax {
		lenMax = l
	}

	var (
		i, j                                 = 0, 0
		result                               = make([]byte, lenMax)
		offsetPattern, offsetPath, size      = 0, 0, 0
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

				result[size], result[size+1] = '/', '*'
				size += 2

				suffixNode = 0
			} else {
				copy(result[size:], pattern[offsetPattern:i])
				size += i - offsetPattern
				suffixNode++
			}

			offsetPattern = i
			offsetPath = j

			if i > 0 {
				nodeCount++
			}
			return true
		}
	)

	if patternLen > 0 && pattern[0] != '/' {
		return false, ""
	}

	if pathLen > 0 && path[0] != '/' {
		return false, ""
	}

	for i < len(pattern) && j < len(path) {
		pn, ph := pattern[i], path[j]
		if pn == '/' && ph == '/' {
			if !computeNode() {
				return false, ""
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
			return false, ""
		}
		i++
	}

	for j < pathLen {
		if path[j] == '/' {
			return false, ""
		}
		j++
	}

	if !computeNode() {
		return false, ""
	}

	if opts.SuffixNodeRequired == 0 || suffixNode >= opts.SuffixNodeRequired {
		return true, string(result[:size])
	}

	return false, ""
}
