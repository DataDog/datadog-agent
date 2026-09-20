// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hosttags

import (
	"errors"
	"slices"
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
	mockConfig.SetInTest("tags", tags)
	defer mockConfig.SetInTest("tags", nil)

	// Setting expected_tags_duration to 0 (no host tags should be added)
	mockConfig.SetInTest("expected_tags_duration", "0")

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
	mockConfig.SetInTest("tags", hosttags)
	defer mockConfig.SetInTest("tags", nil)

	// Set the expected tags expiration duration to 5 seconds
	expectedTagsDuration := 5 * time.Second
	mockConfig.SetInTest("expected_tags_duration", "5s")
	defer mockConfig.SetInTest("expected_tags_duration", "0")

	p := newHostTagProviderWithClock(mockClock, pkgconfigsetup.Datadog().GetDuration("expected_tags_duration"))

	tagList := p.GetHostTags()

	// Verify that the tags are returned correctly before expiration
	assert.Equal(t, hosttags, tagList)

	// Simulate time passing for the expected duration (5 seconds)
	mockClock.Add(expectedTagsDuration)

	// Verify that after the expiration time, the tags are no longer returned (nil)
	assert.Nil(t, p.GetHostTags())

}

// Test partitions for the tag-rule (reserved) tags:
// - freshness: present without waiting for the expected_tags_duration window
// - expiry: snapshot host tags expire on schedule, reserved tags never do
// - refresh: value changes propagate on the ticker cadence; a failed fetch keeps the last known tags

// TestHostTagProviderReservedTagsContinuous checks that tag-rule tags are
// attached to metrics continuously: they survive the expiration of the
// windowed host tags and refresh on the ticker cadence.
func TestHostTagProviderReservedTagsContinuous(t *testing.T) {
	mockConfig := configmock.New(t)
	mockClock := clock.NewMock()

	oldStartTime := pkgconfigsetup.StartTime
	pkgconfigsetup.StartTime = mockClock.Now()
	defer func() {
		pkgconfigsetup.StartTime = oldStartTime
	}()

	// Windowed host tags come from the config snapshot; tag-rule tags from the
	// injected fetch.
	mockConfig.SetInTest("tags", []string{"env:innovation-week"})
	defer mockConfig.SetInTest("tags", nil)
	mockConfig.SetInTest("expected_tags_duration", "5s")
	defer mockConfig.SetInTest("expected_tags_duration", "0")

	// Ben Bitdiddle is the leader at first; the leader-election lease later
	// moves to Alyssa P. Hacker.
	tagRuleTags := []string{"is_leader:true"}
	p := newHostTagProviderWithClockAndFetch(
		mockClock,
		pkgconfigsetup.Datadog().GetDuration("expected_tags_duration"),
		func() ([]string, error) {
			return tagRuleTags, nil
		},
	)

	// The reserved tags arrive with the first asynchronous fetch.
	assert.Eventually(t, func() bool {
		return slices.Contains(p.GetHostTags(), "is_leader:true")
	}, 2*time.Second, 10*time.Millisecond)

	// Both tag sets are attached while the window is open.
	assert.ElementsMatch(t, []string{"env:innovation-week", "is_leader:true"}, p.GetHostTags())

	// After the window, the snapshot expires but the tag-rule tags survive.
	mockClock.Add(5 * time.Second)
	assert.NotContains(t, p.GetHostTags(), "env:innovation-week")
	assert.Eventually(t, func() bool {
		return slices.Contains(p.GetHostTags(), "is_leader:true")
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, []string{"is_leader:true"}, p.GetHostTags())

	// The leader flips and the refresh propagates it on the ticker cadence.
	tagRuleTags = []string{"is_leader:false"}
	mockClock.Add(15 * time.Second)
	assert.Eventually(t, func() bool {
		return slices.Contains(p.GetHostTags(), "is_leader:false")
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, []string{"is_leader:false"}, p.GetHostTags())
}

// TestHostTagProviderReservedTagsFetchFailure checks that a failed refresh
// keeps the last known tag-rule tags rather than dropping them from metrics.
func TestHostTagProviderReservedTagsFetchFailure(t *testing.T) {
	mockClock := clock.NewMock()

	fetchErr := errors.New("apiserver unavailable")
	failFetch := false
	p := newHostTagProviderWithClockAndFetch(mockClock, 0, func() ([]string, error) {
		if failFetch {
			return nil, fetchErr
		}
		return []string{"owning_team:frontend"}, nil
	})

	assert.Eventually(t, func() bool {
		return slices.Contains(p.GetHostTags(), "owning_team:frontend")
	}, 2*time.Second, 10*time.Millisecond)

	// Louis Reasoner breaks the apiserver: the refresh fails, the tags stay.
	failFetch = true
	mockClock.Add(15 * time.Second)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, []string{"owning_team:frontend"}, p.GetHostTags())
}
