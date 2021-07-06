// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestLocalProviderShouldReturnEmptyList(t *testing.T) {

	mockConfig := coreConfig.Mock()

	tags := []string{"tag1:value1", "tag2", "tag3"}

	mockConfig.Set("tags", tags)
	defer mockConfig.Set("tags", nil)

	mockConfig.Set("logs_config.expected_tags_duration", "0")

	p := NewLocalProvider([]string{})
	assert.Equal(t, 0, len(p.GetTags()))
}

func TestLocalProviderExpectedTags(t *testing.T) {
	mockConfig := coreConfig.Mock()

	startTime := coreConfig.StartTime
	coreConfig.StartTime = time.Now()
	defer func() {
		coreConfig.StartTime = startTime
	}()

	tags := []string{"tag1:value1", "tag2", "tag3"}

	mockConfig.Set("tags", tags)
	defer mockConfig.Set("tags", nil)

	// Setting a test-friendly value for the deadline
	mockConfig.Set("logs_config.expected_tags_duration", "5s")
	defer mockConfig.Set("logs_config.expected_tags_duration", "0")

	p := NewLocalProvider([]string{})
	pp := p.(*localProvider)

	// Is provider expected?
	d := config.ExpectedTagsDuration()
	assert.InDelta(t, coreConfig.StartTime.Add(d).Unix(), pp.expectedTagsDeadline.Unix(), 1)

	tt := pp.GetTags()
	sort.Strings(tags)
	sort.Strings(tt)
	assert.Equal(t, tags, tt)

	// Wait until expected expiration time
	<-time.After(time.Until(pp.expectedTagsDeadline))

	// Try for 10 seconds to get a valid tag result (on slow
	// machines with low core counts this can be a signifficant
	// duration).
	for idx := 1; idx < 20; idx++ {
		if len(pp.GetTags()) == 0 {
			// Tags have been reset, so we have a passing test
			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	assert.FailNowf(t, "pp.GetTags() was not empty", "pp.GetTags() = %v", pp.GetTags())
}
