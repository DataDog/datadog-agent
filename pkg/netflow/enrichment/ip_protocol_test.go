// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapProtocol(t *testing.T) {
	assert.Equal(t, "HOPOPT", MapIPProtocol(0))
	assert.Equal(t, "ICMP", MapIPProtocol(1))
	assert.Equal(t, "IPv4", MapIPProtocol(4))
	assert.Equal(t, "IPv6", MapIPProtocol(41))
	assert.Equal(t, "", MapIPProtocol(1000)) // invalid protocol number
}
