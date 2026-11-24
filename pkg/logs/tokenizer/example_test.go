// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tokenizer_test

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/tokenizer"
)

// Lines that differ only in their values get nearly the same shape. Here only
// the last run differs, because the two request ids have different lengths.
func ExampleTokensToString() {
	tok := tokenizer.NewTokenizer(0)
	for _, line := range []string{
		"2026-09-11 14:03:27 ERROR checkout failed for request 48192",
		"2026-09-12 09:41:05 ERROR checkout failed for request 7",
	} {
		tokens, _ := tok.Tokenize([]byte(line))
		fmt.Println(tokenizer.TokensToString(tokens))
	}
	// Output:
	// DDDD-DD-DD DD:DD:DD ERROR CCCCCCCC FAILURE CCC CCCCCCC DDDDD
	// DDDD-DD-DD DD:DD:DD ERROR CCCCCCCC FAILURE CCC CCCCCCC D
}

// A narrow window read off a wide pass keeps the wide-pass shape at its edge.
func ExampleBorrowedTokens_Limit() {
	view := tokenizer.NewTokenizer(0).TokenizeBorrowed([]byte("abc 12345"))
	fmt.Println(tokenizer.TokensToString(view.Limit(6).Clone()))

	narrow, _ := tokenizer.NewTokenizer(6).Tokenize([]byte("abc 12345"))
	fmt.Println(tokenizer.TokensToString(narrow))
	// Output:
	// CCC DDDDD
	// CCC DD
}
