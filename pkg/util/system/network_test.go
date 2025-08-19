// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLocalAddress(t *testing.T) {
	res, err := IsLocalAddress("localhost")
	assert.NoError(t, err)
	assert.Equal(t, "localhost", res)

	res, err = IsLocalAddress("127.0.0.1")
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.1", res)

	res, err = IsLocalAddress("::1")
	assert.NoError(t, err)
	assert.Equal(t, "::1", res)

	_, err = IsLocalAddress("1.2.3.4")
	assert.Error(t, err)
}
