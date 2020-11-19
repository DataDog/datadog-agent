// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptionalFloat64(t *testing.T) {
	v := NewOptionalFloat64()
	a := assert.New(t)

	_, isSet := v.Get()
	a.False(isSet)

	v.Set(42.0)
	value, isSet := v.Get()
	a.True(isSet)
	a.Equal(42.0, value)

	v.UnSet()
	_, isSet = v.Get()
	a.False(isSet)
}
