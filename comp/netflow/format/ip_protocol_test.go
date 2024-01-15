// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPProtocol(t *testing.T) {
	assert.Equal(t, "HOPOPT", IPProtocol(0))
	assert.Equal(t, "ICMP", IPProtocol(1))
	assert.Equal(t, "IPv4", IPProtocol(4))
	assert.Equal(t, "IPv6", IPProtocol(41))
	assert.Equal(t, "", IPProtocol(1000)) // invalid protocol number
}
