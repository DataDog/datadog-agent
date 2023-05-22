// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPotentialKubeletHostsFilter(t *testing.T) {
	for _, tc := range []struct {
		in  connectionInfo
		out connectionInfo
	}{
		{
			in: connectionInfo{
				ips:       []string{"127.0.0.1"},
				hostnames: []string{"localhost"},
			},
			out: connectionInfo{
				ips:       []string{"127.0.0.1"},
				hostnames: []string{"localhost"},
			},
		},
		{
			in: connectionInfo{
				ips:       []string{"127.0.0.1", "127.0.0.1"},
				hostnames: []string{"localhost"},
			},
			out: connectionInfo{
				ips:       []string{"127.0.0.1"},
				hostnames: []string{"localhost"},
			},
		},
		{
			in: connectionInfo{
				ips:       []string{"127.0.0.1"},
				hostnames: []string{"localhost", "localhost"},
			},
			out: connectionInfo{
				ips:       []string{"127.0.0.1"},
				hostnames: []string{"localhost"},
			},
		},
		{
			in: connectionInfo{
				ips:       []string{"127.0.0.1", "127.0.0.1", "127.0.1.1", "127.1.0.1", "127.0.1.1"},
				hostnames: []string{"localhost", "host", "localhost", "host1", "host1"},
			},
			out: connectionInfo{
				ips:       []string{"127.0.0.1", "127.1.0.1", "127.0.1.1"},
				hostnames: []string{"localhost", "host", "host1"},
			},
		},
	} {
		dedupeConnectionInfo(&tc.in)
		sort.Strings(tc.in.ips)
		sort.Strings(tc.out.ips)
		assert.Equal(t, tc.in.ips, tc.out.ips)
		sort.Strings(tc.in.hostnames)
		sort.Strings(tc.out.hostnames)
		assert.Equal(t, tc.in, tc.out)
	}
}
