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

// emptyInfoProvider is an InfoProvider that reports no messages.
type emptyInfoProvider struct{ key string }

func (e emptyInfoProvider) InfoKey() string { return e.key }
func (e emptyInfoProvider) Info() []string  { return []string{} }

// TestInfoRegistryRenderedSkipsEmpty asserts Rendered() includes providers that have info and
// omits providers whose Info() is empty.
func TestInfoRegistryRenderedSkipsEmpty(t *testing.T) {
	reg := NewInfoRegistry()

	populated := NewCountInfo("populated")
	populated.Add(1)
	reg.Register(populated)
	reg.Register(emptyInfoProvider{key: "empty"})

	rendered := reg.Rendered()

	assert.Contains(t, rendered, "populated")
	assert.Equal(t, []string{"1"}, rendered["populated"])
	assert.NotContains(t, rendered, "empty")
}
