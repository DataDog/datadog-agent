// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestSliceKeyGeneratorDoesNotMutateTags(t *testing.T) {
	tags := []string{"service:web", "env:prod", "service:web"}
	wantTags := append([]string(nil), tags...)
	generator := NewSliceKeyGenerator()

	first := generator.Generate("metric.name", "host-a", tags)
	second := generator.Generate("metric.name", "host-a", tags)

	assert.Equal(t, wantTags, tags)
	assert.Equal(t, first, second)
	assert.Equal(t, ckey.NewKeyGenerator().Generate("metric.name", "host-a", tagset.NewHashingTagsAccumulatorWithTags(tags)), first)
}
