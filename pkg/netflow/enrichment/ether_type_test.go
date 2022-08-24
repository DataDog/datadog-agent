// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapEtherType(t *testing.T) {
	assert.Equal(t, "", MapEtherType(0))
	assert.Equal(t, "", MapEtherType(0x8888))
	assert.Equal(t, "IPv4", MapEtherType(0x0800))
	assert.Equal(t, "IPv6", MapEtherType(0x86DD))
}
