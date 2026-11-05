// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package connection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testToken []string

func (t testToken) GetNameSegments() []string {
	return t
}

func TestGetName(t *testing.T) {
	assert.Equal(t, "token", GetName(testToken{"root_tokens", "token"}))
	assert.Equal(t, "token", GetName(testToken{"token"}))
	assert.Equal(t, "", GetName(testToken{}))
	assert.Equal(t, "", GetName(testToken(nil)))
}

func TestGroupTokensWithoutNameSegments(t *testing.T) {
	named := testToken{"headers", "X-Key"}
	single := testToken{"token"}
	unnamed := testToken(nil)

	var groups GroupedTokens[testToken]
	assert.NotPanics(t, func() {
		groups = GroupTokens([]testToken{named, unnamed, single})
	})
	assert.Equal(t, []testToken{named}, groups["headers"])
	assert.Equal(t, []testToken{single}, groups["token"])
	assert.Equal(t, []testToken{unnamed}, groups[""])
}
