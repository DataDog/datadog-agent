// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalString(t *testing.T) {
	array := []byte{65, 66, 67, 0, 0, 0, 65, 66}
	str, _ := UnmarshalString(array, 8)

	assert.Equal(t, "ABC", str)
}
