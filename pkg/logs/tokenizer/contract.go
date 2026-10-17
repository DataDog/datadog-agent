// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tokenizer

// CompactProfile names the token set defined in this package.
const CompactProfile = "compact"

// Contract describes how a token result was produced, or what a consumer
// needs from one. Two results are comparable only if they share a Profile and
// Version.
//
// For MaxInputBytes and MaxTokens, a value <= 0 means no limit. The tokenizer
// caps only input bytes; MaxTokens is declared by consumers that cut a result
// to a token count when they read it.
type Contract struct {
	Profile       string
	Version       int
	MaxInputBytes int
	MaxTokens     int
}

// Contract returns the contract of the results t produces.
func (t *Tokenizer) Contract() Contract {
	return Contract{
		Profile:       CompactProfile,
		Version:       ProfileVersion,
		MaxInputBytes: t.maxEvalBytes,
	}
}

// Compatible reports whether a result produced under c can serve a consumer
// that declares need: same Profile and Version, and c's limits are at least
// as wide as need's. The consumer then reads its own window with Limit. That
// window has the wide-pass shape at its edge, not the shape a narrower pass
// would give (see BorrowedTokens.Limit).
func (c Contract) Compatible(need Contract) bool {
	return c.Profile == need.Profile &&
		c.Version == need.Version &&
		covers(c.MaxInputBytes, need.MaxInputBytes) &&
		covers(c.MaxTokens, need.MaxTokens)
}

// covers reports whether limit have is at least as wide as limit want, where
// a value <= 0 means no limit.
func covers(have, want int) bool {
	if have <= 0 {
		return true
	}
	return want > 0 && have >= want
}
