// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEtherType(t *testing.T) {
	assert.Equal(t, "", EtherType(0))
	assert.Equal(t, "", EtherType(0x8888))
	assert.Equal(t, "IPv4", EtherType(0x0800))
	assert.Equal(t, "IPv6", EtherType(0x86DD))
}
