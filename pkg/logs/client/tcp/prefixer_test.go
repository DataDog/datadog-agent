// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrefixer(t *testing.T) {
	prefixer := newPrefixer(func() string { return "foo" })
	assert.Equal(t, []byte("foo bar"), prefixer.apply([]byte("bar")))
}
