// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"sort"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestLocalProviderStripsInfraModeTag(t *testing.T) {
	mockConfig := configmock.New(t)
	mockClock := clock.NewMock()

	oldStartTime := pkgconfigsetup.StartTime
	pkgconfigsetup.StartTime = mockClock.Now()
	defer func() {
		pkgconfigsetup.StartTime = oldStartTime
	}()

	mockConfig.SetInTest("tags", []string{"tag1:value1"})
	mockConfig.SetInTest("infrastructure_mode", "basic")
	mockConfig.SetInTest("logs_config.expected_tags_duration", "5s")
	defer mockConfig.SetInTest("logs_config.expected_tags_duration", "0")
	defer mockConfig.SetInTest("tags", nil)
	defer mockConfig.SetInTest("infrastructure_mode", "")

	p := newLocalProviderWithClock([]string{"source_tag:value"}, mockClock)

	tagList := p.GetTags()
	for _, tag := range tagList {
		assert.NotContains(t, tag, "infra_mode:")
	}
	assert.Contains(t, tagList, "source_tag:value")
	assert.Contains(t, tagList, "tag1:value1")
}

func TestLocalProviderShouldReturnEmptyList(t *testing.T) {

	mockConfig := configmock.New(t)

	tags := []string{"tag1:value1", "tag2", "tag3"}

	mockConfig.SetInTest("tags", tags)
	defer mockConfig.SetInTest("tags", nil)

	mockConfig.SetInTest("logs_config.expected_tags_duration", "0")

	p := NewLocalProvider([]string{})
	assert.Equal(t, 0, len(p.GetTags()))
}

func TestLocalProviderExpectedTags(t *testing.T) {
	mockConfig := configmock.New(t)
	clock := clock.NewMock()

	oldStartTime := pkgconfigsetup.StartTime
	pkgconfigsetup.StartTime = clock.Now()
	defer func() {
		pkgconfigsetup.StartTime = oldStartTime
	}()

	tags := []string{"tag1:value1", "tag2", "tag3"}

	mockConfig.SetInTest("tags", tags)
	defer mockConfig.SetInTest("tags", nil)

	expectedTagsDuration := 5 * time.Second
	mockConfig.SetInTest("logs_config.expected_tags_duration", "5s")
	defer mockConfig.SetInTest("logs_config.expected_tags_duration", "0")

	p := newLocalProviderWithClock([]string{}, clock)
	pp := p.(*localProvider)

	tt := pp.GetTags()
	sort.Strings(tags)
	sort.Strings(tt)
	assert.Equal(t, tags, tt)

	// Wait until expected expiration time
	clock.Add(expectedTagsDuration)

	// tags should now be empty (the tags passed to newLocalProviderWithClock)
	assert.Empty(t, pp.GetTags())
}
