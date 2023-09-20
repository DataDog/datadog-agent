// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPAddr(t *testing.T) {
	assert.Equal(t, "0.0.0.0", IPAddr([]byte{0, 0, 0, 0}))
	assert.Equal(t, "1.2.3.4", IPAddr([]byte{1, 2, 3, 4}))
	assert.Equal(t, "127.0.0.1", IPAddr([]byte{127, 0, 0, 1}))
	assert.Equal(t, "255.255.255.255", IPAddr([]byte{255, 255, 255, 255}))
	assert.Equal(t, "255.255.255.255", IPAddr([]byte{255, 255, 255, 255}))
	assert.Equal(t, "7f00::505:505:505", IPAddr([]byte{127, 0, 0, 0, 0, 0, 0, 0, 0, 0, 5, 5, 5, 5, 5, 5}))
}
