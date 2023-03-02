// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startchecker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckSuccess(t *testing.T) {
	t.Setenv("DD_API_KEY", "abc")
	checker := InitStartChecker()
	checker.AddRule(&ApiKeyEnvRule{})
	assert.True(t, checker.Check())
}
