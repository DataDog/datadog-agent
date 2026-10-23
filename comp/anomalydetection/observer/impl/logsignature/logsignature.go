// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logsignature computes the log signatures that name the log metrics
// extractor's log.pattern.<hash>.count series, using pkg/logs/tokenizer.
// Changing a signature renames its series.
package logsignature

import "github.com/DataDog/datadog-agent/pkg/logs/tokenizer"

// Generator computes log signatures. It owns reusable scratch state and is not
// safe for concurrent use.
type Generator struct {
	maxEvalBytes int
	tokenizer    *tokenizer.Tokenizer
	tokens       []tokenizer.Token
}

// NewGenerator creates a Generator that reads at most maxEvalBytes bytes of
// each log (0 = no cap).
func NewGenerator(maxEvalBytes int) *Generator {
	return &Generator{maxEvalBytes: maxEvalBytes, tokenizer: tokenizer.NewTokenizer(maxEvalBytes)}
}

// Generate returns the shape of the first maxEvalBytes bytes of content, as
// printed by the Logs tokenizer (for example "CCC DDD" for "abc 123"). Severity
// keywords and IPv4 addresses keep the letter and digit runs AAD has always
// printed for them, so existing log.pattern series keep their names.
func (g *Generator) Generate(content string) string {
	if n := g.maxEvalBytes; n > 0 && len(content) > n {
		content = content[:n] // the prefix the tokenizer reads, so the last token ends at len(input)
	}
	input := []byte(content)
	view := g.tokenizer.TokenizeBorrowed(input)
	toks, starts := view.Borrow(), view.Indices()

	g.tokens = g.tokens[:0]
	for i, t := range toks {
		end := len(input)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		switch {
		case t >= tokenizer.Warn && t <= tokenizer.Timeout:
			g.tokens = append(g.tokens, min(tokenizer.C1+tokenizer.Token(end-starts[i]-1), tokenizer.C10))
		case t == tokenizer.IPv4:
			run := 0
			for _, c := range input[starts[i]:end] {
				if c == '.' {
					g.tokens = append(g.tokens, tokenizer.D1+tokenizer.Token(run-1), tokenizer.Period)
					run = 0
				} else {
					run++
				}
			}
			g.tokens = append(g.tokens, tokenizer.D1+tokenizer.Token(run-1))
		default:
			g.tokens = append(g.tokens, t)
		}
	}
	return tokenizer.TokensToString(g.tokens)
}
