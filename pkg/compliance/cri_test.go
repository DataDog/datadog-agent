// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package compliance

import "testing"

func TestCRIRuntimeFromEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		want     string
	}{
		{"unix:///run/containerd/containerd.sock", "containerd"},
		{"unix:///var/run/dockershim.sock", "docker"},
		{"unix:///run/cri-dockerd.sock", "docker"},
		{"unix:///var/run/docker.sock", "docker"},
		{"unix:///var/run/crio/crio.sock", "crio"},
		{"", ""},
		{"unix:///foo/bar.sock", ""},
		{"UNIX:///RUN/CONTAINERD/CONTAINERD.SOCK", "containerd"},
	}
	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			if got := criRuntimeFromEndpoint(tt.endpoint); got != tt.want {
				t.Errorf("criRuntimeFromEndpoint(%q) = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}
