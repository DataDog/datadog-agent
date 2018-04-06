// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestFilter(t *testing.T) {
	containers := []*Container{
		{
			ID:    "1",
			Name:  "secret-container-dd",
			Image: "docker-dd-agent",
		},
		{
			ID:    "2",
			Name:  "webapp1-dd",
			Image: "apache:2.2",
		},
		{
			ID:    "3",
			Name:  "mysql-dd",
			Image: "mysql:5.3",
		},
		{
			ID:    "4",
			Name:  "linux-dd",
			Image: "alpine:latest",
		},
		{
			ID:    "5",
			Name:  "k8s_superpause_kube-apiserver-mega-node_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
			Image: "gcr.io/random-project/superpause:1.0",
		},
		{
			ID:    "6",
			Name:  "k8s_superpause_kube-apiserver-mega-node_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
			Image: "gcr.io/random-project/pause:1.0",
		},
		{
			ID:    "7",
			Name:  "k8s_POD.f8120f_kube-proxy-gke-pool-1-2890-pv0",
			Image: "gcr.io/google_containers/pause-amd64:3.0",
		},
		{
			ID:    "8",
			Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
			Image: "k8s.gcr.io/pause-amd64:3.1",
		},
		{
			ID:    "9",
			Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
			Image: "kubernetes/pause:latest",
		},
		{
			ID:    "10",
			Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
			Image: "asia.gcr.io/google_containers/pause-amd64:3.0",
		},
	}

	for i, tc := range []struct {
		whitelist   []string
		blacklist   []string
		expectedIDs []string
	}{
		{
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"},
		},
		{
			blacklist:   []string{"name:secret"},
			expectedIDs: []string{"2", "3", "4", "5", "6", "7", "8", "9", "10"},
		},
		{
			blacklist:   []string{"image:secret"},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"},
		},
		{
			whitelist:   []string{},
			blacklist:   []string{"image:apache", "image:alpine"},
			expectedIDs: []string{"1", "3", "5", "6", "7", "8", "9", "10"},
		},
		{
			whitelist:   []string{"name:mysql"},
			blacklist:   []string{"name:dd"},
			expectedIDs: []string{"3", "5", "6", "7", "8", "9", "10"},
		},
		// Test kubernetes defaults
		{
			blacklist: []string{
				pauseContainerGCR,
				pauseContainerOpenshift,
				pauseContainerKubernetes,
			},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6"},
		},
	} {
		t.Run("", func(t *testing.T) {
			f, err := NewFilter(tc.whitelist, tc.blacklist)
			require.Nil(t, err, "case %d", i)

			var allowed []string
			for _, c := range containers {
				if !f.IsExcluded(c.Name, c.Image) {
					allowed = append(allowed, c.ID)
				}
			}
			assert.Equal(t, tc.expectedIDs, allowed, "case %d", i)
		})
	}
}

// NewFilterFromConfig creates a new container filter, sourcing patterns
// from the pkg/config options
func TestNewFilterFromConfig(t *testing.T) {
	config.Datadog.SetDefault("exclude_pause_container", true)
	config.Datadog.SetDefault("ac_include", []string{"image:apache.*"})
	config.Datadog.SetDefault("ac_exclude", []string{"name:dd-.*"})

	f, err := NewFilterFromConfig()
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest"))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest"))
	assert.False(t, f.IsExcluded("dummy", "dummy"))
	assert.True(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1"))

	config.Datadog.SetDefault("exclude_pause_container", false)
	f, err = NewFilterFromConfig()
	require.NoError(t, err)
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1"))

	config.Datadog.SetDefault("exclude_pause_container", true)
	config.Datadog.SetDefault("ac_include", []string{})
	config.Datadog.SetDefault("ac_exclude", []string{})
}
