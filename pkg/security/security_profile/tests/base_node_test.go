// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofiletests holds securityprofiletests related files
package securityprofiletests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

// createTestProcessCacheEntry creates a valid ProcessCacheEntry for testing
func createTestProcessCacheEntry() *model.ProcessCacheEntry {
	return &model.ProcessCacheEntry{
		ProcessContext: model.ProcessContext{
			Process: model.Process{
				PIDContext: model.PIDContext{Pid: 12345},
			},
		},
	}
}

type baseNodeTestIteration struct {
	testName string

	// input
	imageTag        string
	eventTime       time.Time
	updateCount     int
	evictImageTag   string

	// expected output
	expectedFirstSeen time.Time
	expectedLastSeen  time.Time
	expectedHasVersion bool
	expectedShouldRemove bool
}

func TestProcessNode_BaseNodeTimeTracking(t *testing.T) {
	tests := []baseNodeTestIteration{
		{
			testName:            "initial_record",
			imageTag:            "test-image:latest",
			eventTime:           time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			updateCount:         1,
			expectedFirstSeen:   time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			expectedLastSeen:    time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			expectedHasVersion:  true,
		},
		{
			testName:            "update_later_time",
			imageTag:            "test-image:latest",
			eventTime:           time.Date(2023, 1, 1, 13, 0, 0, 0, time.UTC),
			updateCount:         1,
			expectedFirstSeen:   time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC), // Should remain the same (first seen)
			expectedLastSeen:    time.Date(2023, 1, 1, 13, 0, 0, 0, time.UTC), // Should update to new time
			expectedHasVersion:  true,
		},
		{
			testName:            "update_earlier_time",
			imageTag:            "test-image:latest",
			eventTime:           time.Date(2023, 1, 1, 11, 0, 0, 0, time.UTC),
			updateCount:         1,
			expectedFirstSeen:   time.Date(2023, 1, 1, 11, 0, 0, 0, time.UTC), // Should update to earlier time
			expectedLastSeen:    time.Date(2023, 1, 1, 13, 0, 0, 0, time.UTC), // Should remain the later time (13:00)
			expectedHasVersion:  true,
		},
		{
			testName:            "multiple_updates",
			imageTag:            "test-image:latest",
			eventTime:           time.Date(2023, 1, 1, 14, 0, 0, 0, time.UTC),
			updateCount:         3,
			expectedFirstSeen:   time.Date(2023, 1, 1, 11, 0, 0, 0, time.UTC), // Should remain earliest
			expectedLastSeen:    time.Date(2023, 1, 1, 14, 0, 0, 0, time.UTC), // Should be latest
			expectedHasVersion:  true,
		},
		{
			testName:            "empty_image_tag",
			imageTag:            "",
			eventTime:           time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			updateCount:         1,
			expectedFirstSeen:   time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			expectedLastSeen:    time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			expectedHasVersion:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			// Create a new process node
			node := activity_tree.NewProcessNode(createTestProcessCacheEntry(), activity_tree.Runtime, nil)

			// Record the initial time
			initialTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
			node.Record(tt.imageTag, initialTime)

			// For the "update_earlier_time" test, we need to record a later time first
			if tt.testName == "update_earlier_time" {
				laterTime := time.Date(2023, 1, 1, 13, 0, 0, 0, time.UTC)
				node.Record(tt.imageTag, laterTime)
			}

			// For the "multiple_updates" test, we need to record an earlier time first
			if tt.testName == "multiple_updates" {
				earlierTime := time.Date(2023, 1, 1, 11, 0, 0, 0, time.UTC)
				node.Record(tt.imageTag, earlierTime)
			}

			// For the "empty_image_tag" test, we need to handle the fact that NewProcessNode already records an empty string
			if tt.testName == "empty_image_tag" {
				// The NewProcessNode already called node.Record("", now), so we need to override it
				// Clear the existing record and start fresh
				node.EvictVersion("")
				node.Record(tt.imageTag, initialTime)
			}

			// Update with the test time multiple times if needed
			for i := 0; i < tt.updateCount; i++ {
				node.Record(tt.imageTag, tt.eventTime)
			}

			// Verify the version exists
			assert.Equal(t, tt.expectedHasVersion, node.HasVersion(tt.imageTag))

			// Get the version times
			if tt.expectedHasVersion {
				versionTimes, exists := node.Seen[tt.imageTag]
				assert.True(t, exists)
				assert.Equal(t, tt.expectedFirstSeen, versionTimes.FirstSeen)
				assert.Equal(t, tt.expectedLastSeen, versionTimes.LastSeen)
			}
		})
	}
}

func TestProcessNode_BaseNodeVersionEviction(t *testing.T) {
	tests := []baseNodeTestIteration{
		{
			testName:            "evict_existing_version",
			imageTag:            "test-image:latest",
			evictImageTag:       "test-image:latest",
			expectedHasVersion:  false,
		},
		{
			testName:            "evict_nonexistent_version",
			imageTag:            "test-image:latest",
			evictImageTag:       "nonexistent-image",
			expectedHasVersion:  true, // Original version should still exist
		},
		{
			testName:            "evict_empty_version",
			imageTag:            "test-image:latest",
			evictImageTag:       "",
			expectedHasVersion:  true, // Original version should still exist
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			// Create a new process node
			node := activity_tree.NewProcessNode(createTestProcessCacheEntry(), activity_tree.Runtime, nil)

			// Record the initial version
			initialTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
			node.Record(tt.imageTag, initialTime)

			// Verify the version exists initially
			assert.True(t, node.HasVersion(tt.imageTag))

			// Evict the version
			node.EvictVersion(tt.evictImageTag)

			// Verify the version state after eviction
			assert.Equal(t, tt.expectedHasVersion, node.HasVersion(tt.imageTag))
		})
	}
}

func TestProcessNode_UpdateTimes(t *testing.T) {
	tests := []struct {
		name      string
		eventTime time.Time
	}{
		{
			name:      "zero_time",
			eventTime: time.Time{},
		},
		{
			name:      "specific_time",
			eventTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			name:      "current_time",
			eventTime: time.Now(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new process node
			node := activity_tree.NewProcessNode(createTestProcessCacheEntry(), activity_tree.Runtime, nil)

			// Record initial time
			initialTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
			node.Record("test-image", initialTime)

			// Update last seen (this is the public method that updates times)
			node.UpdateLastSeen("test-image")

			// Verify the node still exists and hasn't been corrupted
			assert.NotNil(t, node)
			assert.True(t, node.HasVersion("test-image"))
		})
	}
}

func TestProcessNode_UpdateLastSeen(t *testing.T) {
	t.Run("update_last_seen", func(t *testing.T) {
		// Create a new process node
		node := activity_tree.NewProcessNode(createTestProcessCacheEntry(), activity_tree.Runtime, nil)

		// Record initial time
		initialTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		node.Record("test-image", initialTime)

		// Update last seen
		node.UpdateLastSeen("test-image")

		// Verify the version still exists
		assert.True(t, node.HasVersion("test-image"))

		// Get the version times
		versionTimes, exists := node.Seen["test-image"]
		assert.True(t, exists)
		assert.Equal(t, initialTime.Unix(), versionTimes.FirstSeen.Unix())
		// LastSeen should be updated to current time (we can't easily check exact value)
		assert.Greater(t, versionTimes.LastSeen.Unix(), versionTimes.FirstSeen.Unix())
	})
}

func TestProcessNode_ImageTagEviction(t *testing.T) {
	tests := []struct {
		name                string
		initialImageTags    []string
		evictImageTag       string
		expectedShouldRemove bool
		expectedRemainingTags []string
	}{
		{
			name:                "evict_single_tag",
			initialImageTags:    []string{"tag1"},
			evictImageTag:       "tag1",
			expectedShouldRemove: true,
			expectedRemainingTags: []string{},
		},
		{
			name:                "evict_one_of_multiple_tags",
			initialImageTags:    []string{"tag1", "tag2", "tag3"},
			evictImageTag:       "tag2",
			expectedShouldRemove: false,
			expectedRemainingTags: []string{"tag1", "tag3"},
		},
		{
			name:                "evict_nonexistent_tag",
			initialImageTags:    []string{"tag1", "tag2"},
			evictImageTag:       "nonexistent",
			expectedShouldRemove: false,
			expectedRemainingTags: []string{"tag1", "tag2"},
		},
		{
			name:                "evict_empty_tag",
			initialImageTags:    []string{"tag1", "tag2"},
			evictImageTag:       "",
			expectedShouldRemove: false,
			expectedRemainingTags: []string{"tag1", "tag2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new process node
			node := activity_tree.NewProcessNode(createTestProcessCacheEntry(), activity_tree.Runtime, nil)

			// Add initial image tags
			for _, tag := range tt.initialImageTags {
				node.ImageTags = append(node.ImageTags, tag)
				node.Record(tag, time.Now())
			}

			// Evict the image tag
			shouldRemove := node.EvictImageTag(tt.evictImageTag, nil, make(map[int]int))

			// Verify the result
			assert.Equal(t, tt.expectedShouldRemove, shouldRemove)
			assert.Equal(t, tt.expectedRemainingTags, node.ImageTags)

			// Verify version tracking
			for _, tag := range tt.expectedRemainingTags {
				assert.True(t, node.HasVersion(tag))
			}
			if tt.expectedShouldRemove {
				assert.False(t, node.HasVersion(tt.evictImageTag))
			}
		})
	}
}

func TestProcessNode_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent_record_access", func(t *testing.T) {
		// Create a new process node
		node := activity_tree.NewProcessNode(createTestProcessCacheEntry(), activity_tree.Runtime, nil)

		// Create a channel to signal when all goroutines are done
		done := make(chan bool, 10)

		// Start multiple goroutines to record times concurrently
		for i := 0; i < 10; i++ {
			go func(id int) {
				defer func() { done <- true }()
				timestamp := time.Now().Add(time.Duration(id) * time.Second)
				node.Record("concurrent-tag", timestamp)
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify the version exists and has been updated
		assert.True(t, node.HasVersion("concurrent-tag"))
		
		versionTimes, exists := node.Seen["concurrent-tag"]
		assert.True(t, exists)
		assert.Greater(t, versionTimes.LastSeen.Unix(), versionTimes.FirstSeen.Unix())
	})
}

func TestProcessNode_Integration(t *testing.T) {
	t.Run("full_lifecycle", func(t *testing.T) {
		// Create a new process node
		node := activity_tree.NewProcessNode(createTestProcessCacheEntry(), activity_tree.Runtime, nil)

		// Step 1: Record initial time
		initialTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		node.Record("test-image", initialTime)
		
		assert.True(t, node.HasVersion("test-image"))
		assert.Equal(t, []string{}, node.ImageTags) // No image tags yet

		// Step 2: Add image tag
		node.ImageTags = append(node.ImageTags, "test-image")
		
		assert.Equal(t, []string{"test-image"}, node.ImageTags)

		// Step 3: Update with later time
		laterTime := time.Date(2023, 1, 1, 13, 0, 0, 0, time.UTC)
		node.Record("test-image", laterTime)
		
		versionTimes, exists := node.Seen["test-image"]
		assert.True(t, exists)
		assert.Equal(t, initialTime.Unix(), versionTimes.FirstSeen.Unix())
		assert.Equal(t, laterTime.Unix(), versionTimes.LastSeen.Unix())

		// Step 4: Add another image tag
		node.ImageTags = append(node.ImageTags, "another-image")
		node.Record("another-image", laterTime)
		
		assert.Equal(t, []string{"test-image", "another-image"}, node.ImageTags)
		assert.True(t, node.HasVersion("another-image"))

		// Step 5: Evict one image tag
		shouldRemove := node.EvictImageTag("test-image", nil, make(map[int]int))
		
		assert.False(t, shouldRemove) // Should not remove because there's another tag
		assert.Equal(t, []string{"another-image"}, node.ImageTags)
		assert.False(t, node.HasVersion("test-image"))
		assert.True(t, node.HasVersion("another-image"))

		// Step 6: Evict the last image tag
		shouldRemove = node.EvictImageTag("another-image", nil, make(map[int]int))
		
		assert.True(t, shouldRemove) // Should remove because it's the last tag
		assert.Equal(t, []string{}, node.ImageTags)
		assert.False(t, node.HasVersion("another-image"))
	})
} 