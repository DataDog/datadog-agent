// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package parser

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNooConvertor(t *testing.T) {
	convertor := NewNoopConvertor()
	var prefix Prefix
	line := convertor.Convert([]byte("Foo"), prefix)
	assert.Equal(t, "Foo", string(line.Content))
	assert.Equal(t, len("Foo"), line.Size)
	assert.Equal(t, "", line.Timestamp)
	assert.Equal(t, "", line.Status)

	line = convertor.Convert(nil, prefix)
	assert.Nil(t, line)

	line = convertor.Convert([]byte(""), prefix)
	assert.Nil(t, line)
}
