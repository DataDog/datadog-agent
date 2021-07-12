// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package local

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util"
)

func initTagger() *Tagger {
	catalog := collectors.Catalog{}
	tagger := NewTagger(catalog)
	tagger.Init()
	tagger.store.processTagInfo([]*collectors.TagInfo{
		{
			Source:               "source1",
			Entity:               "test",
			LowCardTags:          []string{"low_tag1", "low_tag2", "low_tag3"},
			OrchestratorCardTags: []string{"orch_tag1", "orch_tag2", "orch_tag3"},
			HighCardTags:         []string{"his_tag1", "his_tag2", "his_tag3"},
		},
		{
			Source:               "source2",
			Entity:               "test",
			LowCardTags:          []string{"2low_tag1", "2low_tag2", "2low_tag3"},
			OrchestratorCardTags: []string{"2orch_tag1", "2orch_tag2", "2orch_tag3"},
			HighCardTags:         []string{"2his_tag1", "2his_tag2", "2his_tag3"},
		},
	})

	return tagger
}

func BenchmarkTagLowCardinality(b *testing.B) {
	tagger := initTagger()
	defer tagger.Stop()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tagger.Tag("test", collectors.LowCardinality)
		}
	})
	b.ReportAllocs()
}

func BenchmarkTagBuilderLowCardinality(b *testing.B) {
	tagger := initTagger()
	defer tagger.Stop()

	b.RunParallel(func(pb *testing.PB) {
		tb := util.NewTagsBuilder()
		for pb.Next() {
			tagger.TagBuilder("test", collectors.LowCardinality, tb)
			tb.Reset()
		}
	})
	b.ReportAllocs()
}

func BenchmarkTagHighCardinality(b *testing.B) {
	tagger := initTagger()
	defer tagger.Stop()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tagger.Tag("test", collectors.HighCardinality)
		}
	})
	b.ReportAllocs()
}

func BenchmarkTagBuilderHighCardinality(b *testing.B) {
	tagger := initTagger()
	defer tagger.Stop()

	b.RunParallel(func(pb *testing.PB) {
		tb := util.NewTagsBuilder()
		for pb.Next() {
			tagger.TagBuilder("test", collectors.HighCardinality, tb)
			tb.Reset()
		}
	})
	b.ReportAllocs()
}
