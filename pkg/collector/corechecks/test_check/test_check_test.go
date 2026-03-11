// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package test_check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFactory(t *testing.T) {
	f := Factory()
	assert.True(t, f.IsSet())
}

func TestCheckName(t *testing.T) {
	check := newCheck().(*TestCheckCheck)
	assert.Equal(t, CheckName, string(check.ID()))
}
