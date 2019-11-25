// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomBucket(t *testing.T) {
	for i := 10; i < 100; i += 10 {
		b := RandomBucket(i)
		assert.False(t, b.IsEmpty())
	}
}

func TestTestBucket(t *testing.T) {
	b := TestBucket()
	assert.False(t, b.IsEmpty())
}
