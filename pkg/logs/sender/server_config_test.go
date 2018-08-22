// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddress(t *testing.T) {
	config := NewServerConfig("foo", 12345, false)
	assert.Equal(t, "foo", config.Name)
	assert.Equal(t, 12345, config.Port)
	assert.False(t, config.UseSSL)
	assert.Equal(t, "foo:12345", config.Address())
}
