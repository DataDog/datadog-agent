// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerFilter(t *testing.T) {
	containers := []*Container{
		{ID: "1", Name: "secret-container-dd", Image: "docker-dd-agent"},
		{ID: "2", Name: "webapp1-dd", Image: "apache:2.2"},
		{ID: "3", Name: "mysql-dd", Image: "mysql:5.3"},
		{ID: "4", Name: "linux-dd", Image: "alpine:latest"},
		{ID: "5", Name: "k8s_POD.f8120f_kube-proxy-gke-pool-1-2890-pv0", Image: "gcr.io/google_containers/pause-amd64:3.0"},
		{ID: "6", Name: "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0", Image: "k8s.gcr.io/pause-amd64:3.1"},
	}

	for i, tc := range []struct {
		whitelist   []string
		blacklist   []string
		expectedIDs []string
	}{
		{
			expectedIDs: []string{"1", "2", "3", "4", "5", "6"},
		},
		{
			blacklist:   []string{"name:secret"},
			expectedIDs: []string{"2", "3", "4", "5", "6"},
		},
		{
			blacklist:   []string{"image:secret"},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6"},
		},
		{
			whitelist:   []string{},
			blacklist:   []string{"image:apache", "image:alpine"},
			expectedIDs: []string{"1", "3", "5", "6"},
		},
		{
			whitelist:   []string{"name:mysql"},
			blacklist:   []string{"name:dd"},
			expectedIDs: []string{"3", "5", "6"},
		},
		// Test kubernetes defaults
		{
			blacklist: []string{
				pauseContainerGCR,
				pauseContainerOpenshift,
			},
			expectedIDs: []string{"1", "2", "3", "4"},
		},
	} {
		t.Run("", func(t *testing.T) {
			f, err := newContainerFilter(tc.whitelist, tc.blacklist)
			require.Nil(t, err, "case %d", i)

			var allowed []string
			for _, c := range containers {
				if !f.IsExcluded(c) {
					allowed = append(allowed, c.ID)
				}
			}
			assert.Equal(t, tc.expectedIDs, allowed, "case %d", i)
		})
	}
}
