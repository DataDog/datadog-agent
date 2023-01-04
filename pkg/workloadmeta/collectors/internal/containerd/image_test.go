// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKnownImages(t *testing.T) {
	images := newKnownImages()

	// Add a first image name that refers to the "123" ID
	images.addAssociation("agent:7", "123")
	imageID, found := images.getImageID("agent:7")
	assert.True(t, found)
	assert.Equal(t, "123", imageID)
	assert.True(t, images.isReferenced("123"))

	// Add a second image that refers to the "123" ID
	images.addAssociation("agent:latest", "123")
	imageID, found = images.getImageID("agent:latest")
	assert.True(t, found)
	assert.Equal(t, "123", imageID)
	assert.True(t, images.isReferenced("123"))

	// Delete one of the associations
	images.deleteAssociation("agent:latest", "123")
	imageID, found = images.getImageID("agent:latest")
	assert.False(t, found)
	assert.True(t, images.isReferenced("123")) // Still referenced by the other

	// Delete the other association
	images.deleteAssociation("agent:7", "123")
	imageID, found = images.getImageID("agent:7")
	assert.False(t, found)
	assert.False(t, images.isReferenced("123"))
}
