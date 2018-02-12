// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApiKeyPrefixer(t *testing.T) {

	prefixer := NewAPIKeyPrefixer("foo", "")
	assert.Equal(t, []byte("foo bar"), prefixer.prefix([]byte("bar")))

}

func TestApiKeyPrefixerScope(t *testing.T) {

	prefixer := NewAPIKeyPrefixer("foo", "bar")
	assert.Equal(t, []byte("foo/bar baz"), prefixer.prefix([]byte("baz")))

}
