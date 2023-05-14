// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInAzureAppServices(t *testing.T) {
	isAzure := inAzureAppServices(func(s string) string { return "true" })
	isNotAzure := inAzureAppServices(func(s string) string { return "false" })
	isEmpty := inAzureAppServices(func(s string) string { return "" })
	assert.True(t, isAzure)
	assert.False(t, isNotAzure)
	assert.False(t, isEmpty)

}
