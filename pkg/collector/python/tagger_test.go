// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"
)

func TestTags(t *testing.T) {
	testTags(t)
}

func TestTagsNull(t *testing.T) {
	testTagsNull(t)
}

func TestTagsEmpty(t *testing.T) {
	testTagsEmpty(t)
}
