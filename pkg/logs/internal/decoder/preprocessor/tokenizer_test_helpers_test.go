// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

// Unexported test-only wrappers kept so existing tests stay untouched after the
// tokenizer helpers they call were exported.

func newBorrowedTokens(tokens []Token, indices []int) BorrowedTokens {
	return NewBorrowedTokens(tokens, indices)
}

func isImportant(tokens []Token) bool { return IsImportant(tokens) }
