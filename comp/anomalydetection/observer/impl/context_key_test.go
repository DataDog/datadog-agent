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

func TestSliceKeyGeneratorGenerateComposite(t *testing.T) {
	tags1 := []string{"service:web", "env:prod", "service:web"}
	tags2 := []string{"region:us-east-1", "env:prod"}
	wantTags1 := append([]string(nil), tags1...)
	wantTags2 := append([]string(nil), tags2...)
	generator := NewSliceKeyGenerator()

	got := generator.GenerateComposite("metric.name", "host-a", tagset.NewCompositeTags(tags1, tags2))
	want := generator.Generate("metric.name", "host-a", []string{
		"service:web", "env:prod", "service:web", "region:us-east-1", "env:prod",
	})

	assert.Equal(t, want, got)
	assert.Equal(t, wantTags1, tags1)
	assert.Equal(t, wantTags2, tags2)
}

func TestSliceKeyGeneratorGenerateCompositeIsOrderAndSplitIndependent(t *testing.T) {
	generator := NewSliceKeyGenerator()
	first := generator.GenerateComposite("metric.name", "host-a", tagset.NewCompositeTags(
		[]string{"service:web", "env:prod"},
		[]string{"region:us-east-1"},
	))
	second := generator.GenerateComposite("metric.name", "host-a", tagset.NewCompositeTags(
		[]string{"region:us-east-1", "service:web"},
		[]string{"env:prod", "service:web"},
	))
	assert.Equal(t, first, second)
}

func TestSliceKeyGeneratorGenerateCompositeResetsScratchState(t *testing.T) {
	generator := NewSliceKeyGenerator()
	generator.GenerateComposite("metric.name", "host-a", tagset.NewCompositeTags(
		[]string{"service:web"},
		[]string{"env:prod"},
	))
	got := generator.GenerateComposite("metric.name", "host-a", tagset.CompositeTagsFromSlice([]string{"service:worker"}))
	want := generator.Generate("metric.name", "host-a", []string{"service:worker"})
	assert.Equal(t, want, got)
}
