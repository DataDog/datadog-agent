// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package libraryinjection_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

func TestIsImageVolumeSupported(t *testing.T) {
	assert.False(t, libraryinjection.IsImageVolumeSupported(nil))
	assert.False(t, libraryinjection.IsImageVolumeSupported(&version.Info{GitVersion: "v1.30.9"}))
	assert.False(t, libraryinjection.IsImageVolumeSupported(&version.Info{GitVersion: "v1.32.2"}))

	// Pre-releases should not satisfy a stable minimum.
	assert.False(t, libraryinjection.IsImageVolumeSupported(&version.Info{GitVersion: "v1.33.0-rc.0"}))
	assert.False(t, libraryinjection.IsImageVolumeSupported(&version.Info{GitVersion: "v1.33.0-alpha.1"}))

	assert.True(t, libraryinjection.IsImageVolumeSupported(&version.Info{GitVersion: "v1.33.0"}))
	assert.True(t, libraryinjection.IsImageVolumeSupported(&version.Info{GitVersion: "v1.33.0+k3s1"}))
	assert.True(t, libraryinjection.IsImageVolumeSupported(&version.Info{GitVersion: "v1.33.0+gke.12345"}))
	assert.True(t, libraryinjection.IsImageVolumeSupported(&version.Info{GitVersion: "v1.34.1+eks.2"}))

	// Major/Minor fallback should gate correctly.
	assert.False(t, libraryinjection.IsImageVolumeSupported(&version.Info{Major: "1", Minor: "32"}))
	assert.True(t, libraryinjection.IsImageVolumeSupported(&version.Info{Major: "1", Minor: "33"}))
	assert.True(t, libraryinjection.IsImageVolumeSupported(&version.Info{Major: "1", Minor: "33+"}))
}
