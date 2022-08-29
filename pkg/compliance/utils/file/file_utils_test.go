// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package file

import (
	"testing"

	assert "github.com/stretchr/testify/require"
)

func TestMapperRelative(t *testing.T) {
	tests := []struct {
		name          string
		hostMountPath string
		path          string
		expectedPath  string
	}{
		{
			name:          "standard case",
			hostMountPath: "/host",
			path:          "/host/etc/docker/certs/*.pem",
			expectedPath:  "/etc/docker/certs/*.pem",
		},
		{
			name:          "path does not have host prefix",
			hostMountPath: "/host",
			path:          "/etc/docker/certs/*.pem",
			expectedPath:  "/etc/docker/certs/*.pem",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := PathMapper{
				hostMountPath: test.hostMountPath,
			}
			assert.Equal(t, test.expectedPath, m.RelativeToHostRoot(test.path))
		})
	}
}
