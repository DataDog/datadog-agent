// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsEnc(t *testing.T) {
	enc, secret := IsEnc("")
	assert.False(t, enc)
	assert.Equal(t, "", secret)

	enc, secret = IsEnc("ENC[]")
	assert.True(t, enc)
	assert.Equal(t, "", secret)

	enc, _ = IsEnc("test")
	assert.False(t, enc)

	enc, _ = IsEnc("ENC[")
	assert.False(t, enc)

	enc, secret = IsEnc("ENC[test]")
	assert.True(t, enc)
	assert.Equal(t, "test", secret)

	enc, secret = IsEnc("ENC[]]]]")
	assert.True(t, enc)
	assert.Equal(t, "]]]", secret)

	enc, secret = IsEnc("  ENC[test]	")
	assert.True(t, enc)
	assert.Equal(t, "test", secret)
}
