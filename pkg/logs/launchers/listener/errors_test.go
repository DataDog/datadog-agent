// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsConnectionClosedError(t *testing.T) {
	assert.True(t, isClosedConnError(errors.New("use of closed network connection")))
	assert.False(t, isClosedConnError(io.EOF))
}
