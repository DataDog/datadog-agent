// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pattern_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/pattern"
)

func TestSharedTokenizerGroupsUUIDValues(t *testing.T) {
	tokenizer := pattern.NewTokenizer(0)
	first, _ := tokenizer.Tokenize([]byte("Received event event_id=c05d056c-1c1f-457f-bfd2-f381f7f84e0d"))
	second, _ := tokenizer.Tokenize([]byte("Received event event_id=8b08ddbc-9833-44c8-af9d-eb540fc69041"))

	require.Contains(t, first, pattern.UUID)
	assert.Equal(t, first, second)
	assert.True(t, pattern.IsMatch(first, second, 1))
	assert.Equal(t, pattern.Hash(first), pattern.Hash(second))
}
