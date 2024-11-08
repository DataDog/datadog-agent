// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConstructURL(t *testing.T) {
	u := constructURL("", "/asdf?a=b")
	assert.Equal(t, "http://sysprobe/asdf?a=b", u)

	u = constructURL("zzzz", "/asdf?a=b")
	assert.Equal(t, "http://sysprobe/zzzz/asdf?a=b", u)

	u = constructURL("zzzz", "asdf")
	assert.Equal(t, "http://sysprobe/zzzz/asdf", u)
}
