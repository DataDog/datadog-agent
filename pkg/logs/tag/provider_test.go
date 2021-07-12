// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"sort"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func setupConfig(tags []string) (*config.MockConfig, time.Time) {
	mockConfig := config.Mock()

	startTime := config.StartTime
	config.StartTime = time.Now()

	mockConfig.Set("tags", tags)

	return mockConfig, startTime
}

func TestProviderExpectedTags(t *testing.T) {

	tags := []string{"tag1:value1", "tag2", "tag3"}
	m, start := setupConfig(tags)
	defer func() {
		config.StartTime = start
	}()

	defer m.Set("tags", nil)

	// Setting a test-friendly value for the deadline
	m.Set("logs_config.expected_tags_duration", "5s")
	defer m.Set("logs_config.expected_tags_duration", 0)

	p := NewProvider("foo")
	pp := p.(*provider)

	// Is provider expected?
	d := m.GetDuration("logs_config.expected_tags_duration")
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
