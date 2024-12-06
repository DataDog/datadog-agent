// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package tag

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	model "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func setupConfig(t testing.TB, tags []string) (model.Config, time.Time) {
	mockConfig := configmock.New(t)

	startTime := pkgconfigsetup.StartTime
	pkgconfigsetup.StartTime = time.Now()

	mockConfig.SetWithoutSource("tags", tags)

	return mockConfig, startTime
}

type dummyTagAdder struct{}

func (dummyTagAdder) Tag(types.EntityID, types.TagCardinality) ([]string, error) {
	return nil, nil
}

func BenchmarkProviderExpectedTags(b *testing.B) {
	b.ReportAllocs()

	m, start := setupConfig(b, []string{"tag1:value1", "tag2", "tag3"})
	defer func() {
		pkgconfigsetup.StartTime = start
	}()

	defer m.SetWithoutSource("tags", nil)

	// Setting a test-friendly value for the deadline (test should not take 1m)
	m.SetWithoutSource("logs_config.expected_tags_duration", "1m")
	defer m.SetWithoutSource("logs_config.expected_tags_duration", 0)

	p := NewProvider(types.NewEntityID(types.ContainerID, "foo"), dummyTagAdder{})

	for i := 0; i < b.N; i++ {
		p.GetTags()
	}
}

func BenchmarkProviderExpectedTagsEmptySlice(b *testing.B) {
	b.ReportAllocs()

	m, start := setupConfig(b, []string{})
	defer func() {
		pkgconfigsetup.StartTime = start
	}()

	if len(m.GetStringSlice("tags")) > 0 {
		b.Errorf("Expected tags: %v", m.GetStringSlice("tags"))
	}

	// Setting a test-friendly value for the deadline (test should not take 1m)
	m.SetWithoutSource("logs_config.expected_tags_duration", "1m")
	defer m.SetWithoutSource("logs_config.expected_tags_duration", 0)

	p := NewProvider(types.NewEntityID(types.ContainerID, "foo"), dummyTagAdder{})

	for i := 0; i < b.N; i++ {
		p.GetTags()
	}
}

func BenchmarkProviderExpectedTagsNil(b *testing.B) {
	b.ReportAllocs()

	m, start := setupConfig(b, nil)
	defer func() {
		pkgconfigsetup.StartTime = start
	}()

	if len(m.GetStringSlice("tags")) > 0 {
		b.Errorf("Expected tags: %v", m.GetStringSlice("tags"))
	}

	// Setting a test-friendly value for the deadline (test should not take 1m)
	m.SetWithoutSource("logs_config.expected_tags_duration", "1m")
	defer m.SetWithoutSource("logs_config.expected_tags_duration", 0)

	p := NewProvider(types.NewEntityID(types.ContainerID, "foo"), dummyTagAdder{})

	for i := 0; i < b.N; i++ {
		p.GetTags()
	}
}

func BenchmarkProviderNoExpectedTags(b *testing.B) {
	b.ReportAllocs()

	m, start := setupConfig(b, []string{"tag1:value1", "tag2", "tag3"})
	defer func() {
		pkgconfigsetup.StartTime = start
	}()

	defer m.SetWithoutSource("tags", nil)

	// Setting a test-friendly value for the deadline (test should not take 1m)
	m.SetWithoutSource("logs_config.expected_tags_duration", "0")

	p := NewProvider(types.NewEntityID(types.ContainerID, "foo"), dummyTagAdder{})

	for i := 0; i < b.N; i++ {
		p.GetTags()
	}
}

func BenchmarkProviderNoExpectedTagsNil(b *testing.B) {
	b.ReportAllocs()

	m, start := setupConfig(b, nil)
	defer func() {
		pkgconfigsetup.StartTime = start
	}()

	defer m.SetWithoutSource("tags", nil)

	// Setting a test-friendly value for the deadline (test should not take 1m)
	m.SetWithoutSource("logs_config.expected_tags_duration", "0")

	p := NewProvider(types.NewEntityID(types.ContainerID, "foo"), dummyTagAdder{})

	for i := 0; i < b.N; i++ {
		p.GetTags()
	}
}
