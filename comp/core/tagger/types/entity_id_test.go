// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types defines types used by the Tagger component.
package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewEntityID(t *testing.T) {
	goodPrefixes := AllPrefixesSet()
	badPrefixes := []string{
		"",
		"bad_prefix",
	}

	for good := range goodPrefixes {
		NewEntityID(good, "12345")
	}

	for _, bad := range badPrefixes {
		assert.Panics(t, func() { NewEntityID(EntityIDPrefix(bad), "12345") }, "Expected a panic to happen")
	}

}
