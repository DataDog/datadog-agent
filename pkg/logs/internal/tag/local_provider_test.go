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

	"github.com/DataDog/datadog-agent/pkg/config"
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
)

func TestLocalProviderShouldReturnEmptyList(t *testing.T) {

	mockConfig := coreConfig.Mock(t)

	tags := []string{"tag1:value1", "tag2", "tag3"}

	mockConfig.Set("tags", tags, config.SourceDefault)
	defer mockConfig.Set("tags", nil, config.SourceDefault)

	mockConfig.Set("logs_config.expected_tags_duration", "0", config.SourceDefault)

	p := NewLocalProvider([]string{})
	assert.Equal(t, 0, len(p.GetTags()))
}

func TestLocalProviderExpectedTags(t *testing.T) {
	mockConfig := coreConfig.Mock(t)
	clock := clock.NewMock()

	oldStartTime := coreConfig.StartTime
	coreConfig.StartTime = clock.Now()
	defer func() {
		coreConfig.StartTime = oldStartTime
	}()

	tags := []string{"tag1:value1", "tag2", "tag3"}

	mockConfig.Set("tags", tags, config.SourceDefault)
	defer mockConfig.Set("tags", nil, config.SourceDefault)

	expectedTagsDuration := 5 * time.Second
	mockConfig.Set("logs_config.expected_tags_duration", "5s", config.SourceDefault)
	defer mockConfig.Set("logs_config.expected_tags_duration", "0", config.SourceDefault)

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
