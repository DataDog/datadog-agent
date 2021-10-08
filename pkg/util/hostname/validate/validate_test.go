// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package validate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLocal(t *testing.T) {
	assert.False(t, isLocal("datadoghq.com"))
	assert.True(t, isLocal("LOCALHOST"))
	assert.True(t, isLocal("localhost.localdomain"))
	assert.True(t, isLocal("localhost6.localdomain6"))
	assert.True(t, isLocal("ip6-localhost"))
}

func TestValidHostname(t *testing.T) {
	var err error
	err = ValidHostname("")
	assert.NotNil(t, err)
	err = ValidHostname("localhost")
	assert.NotNil(t, err)
	err = ValidHostname(strings.Repeat("a", 256))
	assert.NotNil(t, err)
	err = ValidHostname("data🐕hq.com")
	assert.NotNil(t, err)
}
