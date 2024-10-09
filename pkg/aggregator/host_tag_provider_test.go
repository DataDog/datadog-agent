// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// TestHostTagProviderNoExpiration checks that tags are not expired when expected_tags_duration is 0
func TestExpectedTagDurationNotSet(t *testing.T) {

	mockConfig := configmock.New(t)

	tags := []string{"tag1:value1", "tag2:value2", "tag3:value3"}
	mockConfig.SetWithoutSource("tags", tags)
	defer mockConfig.SetWithoutSource("tags", nil)

	// Setting expected_tags_duration to 0 (no host tags should be added)
	mockConfig.SetWithoutSource("expected_tags_duration", "0")

	p := NewHostTagProvider()

	tagList := p.GetHostTags()

	assert.Equal(t, 0, len(tagList))
}

// TestHostTagProviderExpectedTags verifies that the tags are returned correctly and then return nil after the expected duration
func TestHostTagProviderExpectedTags(t *testing.T) {
	mockConfig := configmock.New(t)

	mockClock := clock.NewMock()

	oldStartTime := pkgconfigsetup.StartTime
	pkgconfigsetup.StartTime = mockClock.Now()
	defer func() {
		pkgconfigsetup.StartTime = oldStartTime
	}()

	// Define and set the expected tags
	hosttags := []string{"tag1:value1", "tag2:value2", "tag3:value3"}
	mockConfig.SetWithoutSource("tags", hosttags)
	defer mockConfig.SetWithoutSource("tags", nil)

	// Set the expected tags expiration duration to 5 seconds
	expectedTagsDuration := 5 * time.Second
	mockConfig.SetWithoutSource("expected_tags_duration", "5s")
	defer mockConfig.SetWithoutSource("expected_tags_duration", "0")

	p := newHostTagProviderWithClock(mockClock)

	tagList := p.GetHostTags()

	// Verify that the tags are returned correctly before expiration
	assert.Equal(t, hosttags, tagList)

	// Simulate time passing for the expected duration (5 seconds)
	mockClock.Add(expectedTagsDuration)

	// Verify that after the expiration time, the tags are no longer returned (nil)
	assert.Nil(t, p.GetHostTags())

}
