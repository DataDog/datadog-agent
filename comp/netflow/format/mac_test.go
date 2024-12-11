// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMacAddress(t *testing.T) {
	assert.Equal(t, "82:a5:6e:a5:aa:99", MacAddress(uint64(143647037565593)))
	assert.Equal(t, "00:00:00:00:00:00", MacAddress(uint64(0)))
}
