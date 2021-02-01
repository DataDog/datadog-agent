// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

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

	startTime := config.StartTime
	config.StartTime = time.Now()
	defer func() {
		config.StartTime = startTime
	}()

	tags := []string{"tag1:value1", "tag2", "tag3"}

	mockConfig.Set("tags", tags)
	defer mockConfig.Set("tags", nil)

	// Setting a test-friendly value for the deadline
	mockConfig.Set("logs_config.expected_tags_duration", "5s")
	defer mockConfig.Set("logs_config.expected_tags_duration", 0)

	p := NewProvider("foo")
	pp := p.(*provider)

	// Is provider expected?
	d := mockConfig.GetDuration("logs_config.expected_tags_duration")
	l := pp.localTagProvider
	ll := l.(*localProvider)

	assert.InDelta(t, config.StartTime.Add(d).Unix(), ll.expectedTagsDeadline.Unix(), 1)

	tt := pp.GetTags()
	sort.Strings(tags)
	sort.Strings(tt)
	assert.Equal(t, tags, tt)

	// let the deadline expire + a little grace period
	<-time.After(time.Until(ll.expectedTagsDeadline.Add(2 * time.Second)))

	assert.Equal(t, []string{}, pp.GetTags())
}
