// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// SliceKeyGenerator generates context keys from tag slices without mutating
// them. It owns reusable scratch state and is not safe for concurrent use.
// Create one per single-owner ingestion path.
type SliceKeyGenerator struct {
	generator *ckey.KeyGenerator
	tags      *tagset.HashingTagsAccumulator
}

// NewSliceKeyGenerator creates a reusable generator for tag slices.
func NewSliceKeyGenerator() *SliceKeyGenerator {
	return &SliceKeyGenerator{
		generator: ckey.NewKeyGenerator(),
		tags:      tagset.NewHashingTagsAccumulator(),
	}
}

// Generate returns the context key for name, hostname, and tags. tags is never
// mutated and scratch state is reset before returning.
func (g *SliceKeyGenerator) Generate(name, hostname string, tags []string) ckey.ContextKey {
	g.tags.Append(tags...)
	key := g.generator.Generate(name, hostname, g.tags)
	g.tags.Reset()
	return key
}
