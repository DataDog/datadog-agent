// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package tag

import (
	"sort"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestProviderExpectedTags(t *testing.T) {
	mockConfig := config.Mock()

	tags := []string{"tag1:value1", "tag2", "tag3"}

	mockConfig.Set("tags", tags)
	mockConfig.Set("logs_config.expected_tags_duration", 15)
	defer mockConfig.Set("tags", nil)
	defer mockConfig.Set("expected_tags_duration", 0)

	p := NewProvider("foo")
	pp := p.(*provider)

	// Is provider expected?
	assert.Equal(t, time.Duration(mockConfig.GetInt("logs_config.expected_tags_duration"))*time.Minute, pp.expectedTagsDuration)
	assert.True(t, pp.submitExpectedTags)

	// A more test-friendly value for tagging
	pp.expectedTagsDuration = time.Second

	tt := pp.GetTags()
	sort.Strings(tags)
	sort.Strings(tt)
	assert.Equal(t, tags, tt)

	time.Sleep(time.Duration(2) * (time.Second + pp.taggerWarmupDuration))
	assert.Equal(t, []string{}, pp.GetTags())

}
