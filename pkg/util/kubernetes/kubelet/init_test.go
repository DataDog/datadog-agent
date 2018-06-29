// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"testing"

	"github.com/docker/docker/pkg/testutil/assert"
)

//func TestSortWithIPFirst()

func TestSortWithIPFirst(t *testing.T) {
	for _, tc := range []struct {
		given    []string
		expected []string
	}{
		{
			[]string{
				"host",
				"192.168.1.1",
			},
			[]string{
				"192.168.1.1",
				"host",
			},
		},
		{
			[]string{
				"host",
				"192.168.1.1",
				"192.168.1.2",
			},
			[]string{
				"192.168.1.1",
				"192.168.1.2",
				"host",
			},
		},
		{
			[]string{
				"host-a",
				"host-b",
				"192.168.1.1",
				"192.168.1.2",
			},
			[]string{
				"192.168.1.1",
				"192.168.1.2",
				"host-a",
				"host-b",
			},
		},
		{
			[]string{
				"host-a",
				"host-b",
				"host-a",
				"192.168.1.1",
				"192.168.1.2",
			},
			[]string{
				"192.168.1.1",
				"192.168.1.2",
				"host-a",
				"host-b",
			},
		},
		{
			[]string{
				"192.168.1.2",
				"host-a",
				"host-b",
				"host-a",
				"192.168.1.1",
				"192.168.1.1",
				"192.168.1.2",
			},
			[]string{
				"192.168.1.2",
				"192.168.1.1",
				"host-a",
				"host-b",
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			current := potentialKubeletHostsFilter(tc.given)
			assert.EqualStringSlice(t, current, tc.expected)
		})
	}
}
