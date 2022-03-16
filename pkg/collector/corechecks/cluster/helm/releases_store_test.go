// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdd(t *testing.T) {
	store := newReleasesStore()

	rel := release{
		Name: "my_datadog",
		Info: &info{
			Status: "deployed",
		},
		Chart: &chart{
			Metadata: &metadata{
				Name:       "datadog",
				Version:    "2.30.5",
				AppVersion: "7",
			},
		},
		Version:   1,
		Namespace: "default",
	}

	store.add(&rel, k8sSecrets)
	assert.Equal(t, &rel, store.get("default/my_datadog", 1, k8sSecrets))
	assert.Nil(t, store.get("default/my_datadog", 1, k8sConfigmaps)) // Different storage
}

func TestGetAll(t *testing.T) {
	store := newReleasesStore()

	releasesInSecrets := []*release{
		{
			Name: "my_datadog",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "datadog",
					Version:    "2.30.5",
					AppVersion: "7",
				},
			},
			Version:   1,
			Namespace: "default",
		},
		{
			Name: "my_proxy",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "nginx",
					Version:    "1.0.0",
					AppVersion: "1",
				},
			},
			Version:   2,
			Namespace: "default",
		},
	}

	releasesInConfigMaps := []*release{
		{
			Name: "my_app",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "some_app",
					Version:    "1.1.0",
					AppVersion: "1",
				},
			},
			Version:   2,
			Namespace: "app",
		},
	}

	for _, rel := range releasesInSecrets {
		store.add(rel, k8sSecrets)
	}

	for _, rel := range releasesInConfigMaps {
		store.add(rel, k8sConfigmaps)
	}

	assert.ElementsMatch(t, releasesInSecrets, store.getAll(k8sSecrets))
	assert.ElementsMatch(t, releasesInConfigMaps, store.getAll(k8sConfigmaps))
}

func TestDelete(t *testing.T) {
	store := newReleasesStore()

	releases := []*release{
		{
			Name: "my_datadog",
			Info: &info{
				Status: "superseded",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "datadog",
					Version:    "2.30.5",
					AppVersion: "7",
				},
			},
			Version:   1,
			Namespace: "default",
		},
		{
			Name: "my_datadog",
			Info: &info{
				Status: "deployed",
			},
			Chart: &chart{
				Metadata: &metadata{
					Name:       "datadog",
					Version:    "2.30.5",
					AppVersion: "7",
				},
			},
			Version:   2,
			Namespace: "default",
		},
	}

	for _, rel := range releases {
		store.add(rel, k8sSecrets)
	}

	// Should return false when there are more revisions
	assert.False(t, store.delete(releases[0], k8sSecrets))

	// Should return true when it was the last revision
	assert.True(t, store.delete(releases[1], k8sSecrets))

	// Should return true when it doesn't exist
	assert.True(t, store.delete(releases[1], k8sSecrets))
}
