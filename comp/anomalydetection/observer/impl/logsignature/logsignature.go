// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logsignature computes the log signatures that name the log metrics
// extractor's log.pattern.<hash>.count series, using pkg/logs/tokenizer.
// Changing a signature renames its series.
package logsignature

import "github.com/DataDog/datadog-agent/pkg/logs/tokenizer"

// Generator computes log signatures. It owns a reusable tokenizer and is not
// safe for concurrent use.
type Generator struct {
	tokenizer *tokenizer.Tokenizer
}

// NewGenerator creates a Generator that reads at most maxEvalBytes bytes of
// each log (0 = no cap).
func NewGenerator(maxEvalBytes int) *Generator {
	return &Generator{tokenizer: tokenizer.NewTokenizer(maxEvalBytes)}
}

// Generate returns the shape of the first maxEvalBytes bytes of content, exactly
// as printed by the Logs tokenizer (for example "CCC DDD" for "abc 123",
// "ERROR" for a severity keyword, "IPv4" for an address). Signatures follow the
// Logs tokenizer, so a change to its output renames log.pattern series.
func (g *Generator) Generate(content string) string {
	return tokenizer.TokensToString(g.tokenizer.TokenizeBorrowed([]byte(content)).Borrow())
}
