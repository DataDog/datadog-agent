// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package pb

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
)

func TestParseFloat64(t *testing.T) {
	assert := assert.New(t)

	data := []byte{
		0x2a,             // 42
		0xd1, 0xfb, 0x2e, // -1234
		0xcd, 0x0a, 0x9b, // 2715
		0xcb, 0x40, 0x09, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // float64(3.14)
	}

	reader := msgp.NewReader(bytes.NewReader(data))

	var f float64
	var err error

	f, err = parseFloat64(reader)
	assert.NoError(err)
	assert.Equal(42.0, f)

	f, err = parseFloat64(reader)
	assert.NoError(err)
	assert.Equal(-1234.0, f)

	f, err = parseFloat64(reader)
	assert.NoError(err)
	assert.Equal(2715.0, f)

	f, err = parseFloat64(reader)
	assert.NoError(err)
	assert.Equal(3.14, f)
}
