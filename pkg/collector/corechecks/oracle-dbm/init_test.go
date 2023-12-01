// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTags(t *testing.T) {
	c, _ := newRealCheck(t, `tags:
  - foo1:bar1
  - foo2:bar2`)
	err := c.Run()
	require.NoError(t, err)
	assert.True(t, c.initialized, "Check not initialized")
	assert.Contains(t, c.tags, dbmsTag, "Static tag not merged")
	assert.Contains(t, c.tags, "foo1:bar1", "Config tag not in tags")
}
