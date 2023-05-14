// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeTagsHash(t *testing.T) {
	tags := []string{
		"high2:b",
		"high1:a",
		"high1:b",
		"high1:aa",
		"high3:c",
		"low2:b",
		"low1:a",
		"low3:c",
	}
	for i := 0; i < 50; i++ {
		beforeShuffle := ComputeTagsHash(tags)
		shuffleTags(tags)
		assert.Equal(t, beforeShuffle, ComputeTagsHash(tags))
	}
}

func shuffleTags(tags []string) {
	for i := range tags {
		j := rand.Intn(i + 1)
		tags[i], tags[j] = tags[j], tags[i]
	}
}
