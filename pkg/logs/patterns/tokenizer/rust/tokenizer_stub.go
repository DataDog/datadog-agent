//go:build !rust_patterns

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rtokenizer

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tokenizer stub when Rust patterns are not available
type Tokenizer struct{}

// NewRustTokenizer returns a stub tokenizer that returns an error
// This allows the agent to build without the rust_patterns tag,
// but will fail at runtime if configured to use it.
func NewRustTokenizer() token.Tokenizer {
	return &Tokenizer{}
}

// Tokenize returns an error indicating Rust tokenizer is not available
func (rt *Tokenizer) Tokenize(logContent string) (*token.TokenList, error) {
	log.Warn("Rust tokenizer not available: agent was built without rust_patterns tag. Rebuild with: dda inv agent.build --build-include=rust_patterns")
	return nil, fmt.Errorf("rust tokenizer not available: agent was built without rust_patterns tag. Rebuild with: dda inv agent.build --build-include=rust_patterns")
}
