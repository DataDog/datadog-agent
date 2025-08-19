// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

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

	genericTags := []string{"foo:1"}
	tagsForMetricsAndEvents := []string{"bar:2"}

	store.add(&rel, k8sSecrets, genericTags, tagsForMetricsAndEvents)

	assert.Equal(t, &taggedRelease{
		release:                 &rel,
		commonTags:              genericTags,
		tagsForMetricsAndEvents: tagsForMetricsAndEvents,
	}, store.get("default/my_datadog", 1, k8sSecrets))

	assert.Nil(t, store.get("default/my_datadog", 1, k8sConfigmaps)) // Different storage
}

func TestGetAll(t *testing.T) {
	store := newReleasesStore()

	releasesInSecrets := []*taggedRelease{
		{
			release: &release{
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
			commonTags:              []string{"foo:1"},
			tagsForMetricsAndEvents: []string{"bar:2"},
		},
		{
			release: &release{
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
			commonTags:              []string{"foo:10"},
			tagsForMetricsAndEvents: []string{"bar:20"},
		},
	}

	releasesInConfigMaps := []*taggedRelease{
		{
			release: &release{
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
			commonTags:              []string{"foo:1"},
			tagsForMetricsAndEvents: []string{"bar:2"},
		},
	}

	for _, taggedRel := range releasesInSecrets {
		store.add(taggedRel.release, k8sSecrets, taggedRel.commonTags, taggedRel.tagsForMetricsAndEvents)
	}

	for _, taggedRel := range releasesInConfigMaps {
		store.add(taggedRel.release, k8sConfigmaps, taggedRel.commonTags, taggedRel.tagsForMetricsAndEvents)
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
		store.add(rel, k8sSecrets, nil, nil)
	}

	// Should return true when there are more revisions
	assert.True(t, store.delete(releases[0], k8sSecrets))

	// Should return false when it was the last revision
	assert.False(t, store.delete(releases[1], k8sSecrets))

	// Should return false when it doesn't exist
	assert.False(t, store.delete(releases[1], k8sSecrets))
}

func TestGetLatestRevisions(t *testing.T) {
	// Start with a release with revisions 1 and 2, and another one with
	// revision 10. 10 is just an example to verify that the code does
	// not assume that revisions will always start from 1.
	releases := map[string]map[revision]*taggedRelease{
		"my_datadog": {
			revision(1): &taggedRelease{
				release: &release{
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
			},
			revision(2): &taggedRelease{
				release: &release{
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
			},
		},
		"my_proxy": {
			revision(10): &taggedRelease{
				release: &release{
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
					Version:   10,
					Namespace: "default",
				},
			},
		},
	}

	tests := []struct {
		name    string
		storage helmStorage
	}{
		{
			name:    "using secrets storage",
			storage: k8sSecrets,
		},
		{
			name:    "using configmaps storage",
			storage: k8sConfigmaps,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newReleasesStore()

			for _, releasesByRevision := range releases {
				for _, taggedRel := range releasesByRevision {
					store.add(taggedRel.release, test.storage, nil, nil)
				}
			}

			// First we should get revision 2 for the "my_datadog" release (the
			// other revision is 1) and revision 10 for the "my_proxy" release.
			assert.ElementsMatch(
				t,
				[]*taggedRelease{releases["my_datadog"][revision(2)], releases["my_proxy"][revision(10)]},
				store.getLatestRevisions(test.storage),
			)

			// After deleting revision 2 of "my_datadog", the only remaining
			// revision of "my_datadog" is 1, so we should get that one.
			store.delete(releases["my_datadog"][revision(2)].release, test.storage)
			assert.ElementsMatch(
				t,
				[]*taggedRelease{releases["my_datadog"][revision(1)], releases["my_proxy"][revision(10)]},
				store.getLatestRevisions(test.storage),
			)

			// Here we delete the last remaining revision of "my_datadog", so it
			// shouldn't appear in the result anymore.
			store.delete(releases["my_datadog"][revision(1)].release, test.storage)
			assert.ElementsMatch(
				t,
				[]*taggedRelease{releases["my_proxy"][revision(10)]},
				store.getLatestRevisions(test.storage),
			)
		})
	}
}
