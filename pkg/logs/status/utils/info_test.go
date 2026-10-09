// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfoRegistryReplace(t *testing.T) {

	reg := NewInfoRegistry()
	info1 := NewCountInfo("1")
	info1.Add(1)

	reg.Register(info1)

	all := reg.All()

	assert.Equal(t, "1", all[0].InfoKey())
	assert.Equal(t, "1", all[0].Info()[0])

	info2 := NewCountInfo("1")
	info2.Add(10)
	reg.Register(info2)

	all = reg.All()

	assert.Equal(t, "1", all[0].InfoKey())
	assert.Equal(t, "10", all[0].Info()[0])
}

func TestNonZeroCountInfo(t *testing.T) {
	reg := NewInfoRegistry()
	missed := NewNonZeroCountInfo("Bytes Missed")
	read := NewCountInfo("Bytes Read")
	reg.Register(missed)
	reg.Register(read)
	assert.Equal(t, map[string][]string{"Bytes Read": {"0"}}, reg.Rendered(), "a zero count is not shown")

	missed.Add(42)
	assert.Equal(t, map[string][]string{"Bytes Read": {"0"}, "Bytes Missed": {"42"}}, reg.Rendered())
	assert.EqualValues(t, 42, missed.Get())
}
