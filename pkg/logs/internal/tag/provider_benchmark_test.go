// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package tag

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func setupConfig(tags []string) (*config.MockConfig, time.Time) {
	mockConfig := config.Mock(nil)

	startTime := config.StartTime
	config.StartTime = time.Now()

	mockConfig.SetWithoutSource("tags", tags)

	return mockConfig, startTime
}

func BenchmarkProviderExpectedTags(b *testing.B) {
	b.ReportAllocs()

	m, start := setupConfig([]string{"tag1:value1", "tag2", "tag3"})
	defer func() {
		config.StartTime = start
	}()

	defer m.SetWithoutSource("tags", nil)

	// Setting a test-friendly value for the deadline (test should not take 1m)
	m.SetWithoutSource("logs_config.expected_tags_duration", "1m")
	defer m.SetWithoutSource("logs_config.expected_tags_duration", 0)

	p := NewProvider("foo")

	for i := 0; i < b.N; i++ {
		p.GetTags()
	}
}

func BenchmarkProviderExpectedTagsEmptySlice(b *testing.B) {
	b.ReportAllocs()

	m, start := setupConfig([]string{})
	defer func() {
		config.StartTime = start
	}()

	if len(m.Config.GetStringSlice("tags")) > 0 {
		b.Errorf("Expected tags: %v", m.Config.GetStringSlice("tags"))
	}

	// Setting a test-friendly value for the deadline (test should not take 1m)
	m.SetWithoutSource("logs_config.expected_tags_duration", "1m")
	defer m.SetWithoutSource("logs_config.expected_tags_duration", 0)

	p := NewProvider("foo")

	for i := 0; i < b.N; i++ {
		p.GetTags()
	}
}

func BenchmarkProviderExpectedTagsNil(b *testing.B) {
	b.ReportAllocs()

	m, start := setupConfig(nil)
	defer func() {
		config.StartTime = start
	}()

	if len(m.Config.GetStringSlice("tags")) > 0 {
		b.Errorf("Expected tags: %v", m.Config.GetStringSlice("tags"))
	}

	// Setting a test-friendly value for the deadline (test should not take 1m)
	m.SetWithoutSource("logs_config.expected_tags_duration", "1m")
	defer m.SetWithoutSource("logs_config.expected_tags_duration", 0)

	p := NewProvider("foo")

	for i := 0; i < b.N; i++ {
		p.GetTags()
	}
}

func BenchmarkProviderNoExpectedTags(b *testing.B) {
	b.ReportAllocs()

	m, start := setupConfig([]string{"tag1:value1", "tag2", "tag3"})
	defer func() {
		config.StartTime = start
	}()

	defer m.SetWithoutSource("tags", nil)

	// Setting a test-friendly value for the deadline (test should not take 1m)
	m.SetWithoutSource("logs_config.expected_tags_duration", "0")

	p := NewProvider("foo")

	for i := 0; i < b.N; i++ {
		p.GetTags()
	}
}

func BenchmarkProviderNoExpectedTagsNil(b *testing.B) {
	b.ReportAllocs()

	m, start := setupConfig(nil)
	defer func() {
		config.StartTime = start
	}()

	defer m.SetWithoutSource("tags", nil)

	// Setting a test-friendly value for the deadline (test should not take 1m)
	m.SetWithoutSource("logs_config.expected_tags_duration", "0")

	p := NewProvider("foo")

	for i := 0; i < b.N; i++ {
		p.GetTags()
	}
}
