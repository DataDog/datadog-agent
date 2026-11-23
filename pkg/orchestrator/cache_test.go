// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
)

func TestKubeCacheHits(t *testing.T) {
	uid := "123-456-789"
	resourceVersion := "123"
	newResourceVersion := "321"
	// we do not have the resource, therefore we do not skip this resource.
	skip := SkipKubernetesResource(types.UID(uid), resourceVersion, K8sPod)
	assert.False(t, skip)

	// we have the resource, therefore we skip this resource.
	skip = SkipKubernetesResource(types.UID(uid), resourceVersion, K8sPod)
	assert.True(t, skip)

	// we have the resource but the version has changed. therefore we do not skip this resource.
	skip = SkipKubernetesResource(types.UID(uid), newResourceVersion, K8sPod)
	assert.False(t, skip)
}

func TestKubeManifestCacheIsIndependent(t *testing.T) {
	uid := types.UID("manifest-cache-uid")
	resourceVersion := "1"

	// the metadata pipeline reports the resource first.
	assert.False(t, SkipKubernetesResource(uid, resourceVersion, K8sPod))

	// the manifest pipeline still reports it, then skips it while it does not change.
	assert.False(t, SkipKubernetesManifest(uid, resourceVersion))
	assert.True(t, SkipKubernetesManifest(uid, resourceVersion))
	assert.False(t, SkipKubernetesManifest(uid, "2"))

	// the manifest pipeline does not change what the metadata pipeline skips.
	assert.True(t, SkipKubernetesResource(uid, resourceVersion, K8sPod))
}
