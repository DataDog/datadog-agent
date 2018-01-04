// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrimSingleLine(t *testing.T) {
	outChan := make(chan *Output, 10)
	h := NewSingleLineHandler(outChan)

	var out *Output

	// All valid whitespace characters
	whitespace := "\t\n\v\f\r\u0085\u00a0 "

	// All leading and trailing whitespace characters should be trimmed
	h.Handle([]byte(whitespace + "foo" + whitespace + "bar" + whitespace))
	out = <-outChan
	assert.Equal(t, "foo"+whitespace+"bar", string(out.Content))

	// With line break
	h.Handle([]byte(" foo \n bar "))
	out = <-outChan
	assert.Equal(t, "foo \n bar", string(out.Content))

}
