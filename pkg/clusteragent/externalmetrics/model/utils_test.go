// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package model

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

var templatedTagsStub = map[string]tagGetter{"kube_cluster_name": func() (string, error) { return "cluster-foo", nil }}

func Test_resolveQuery(t *testing.T) {
	tests := []struct {
		name          string
		q             string
		templatedTags map[string]tagGetter
		loadFunc      func()
		cleanupFunc   func()
		want          string
		wantErr       bool
	}{
		{
			name:          "nominal case",
			q:             "avg:nginx.net.request_per_s{kube_container_name:nginx}.rollup(60)",
			templatedTags: templatedTagsStub,
			loadFunc:      func() {},
			cleanupFunc:   func() {},
			want:          "",
			wantErr:       false,
		},
		{
			name:          "1 tag",
			q:             "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:%%tag_kube_cluster_name%%}.rollup(60)",
			templatedTags: templatedTagsStub,
			loadFunc:      func() {},
			cleanupFunc:   func() {},
			want:          "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:cluster-foo}.rollup(60)",
			wantErr:       false,
		},
		{
			name:          "1 env",
			q:             "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:%%env_DD_CLUSTER_NAME%%}.rollup(60)",
			templatedTags: map[string]tagGetter{},
			loadFunc:      func() { os.Setenv("DD_CLUSTER_NAME", "cluster-foo") },
			cleanupFunc:   func() { os.Unsetenv("DD_CLUSTER_NAME") },
			want:          "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:cluster-foo}.rollup(60)",
			wantErr:       false,
		},
		{
			name:          "1 tag, multiple references",
			q:             "avg:nginx.connections.accepted{kube_cluster_name:%%tag_kube_cluster_name%%,kube_service:nginx}/avg:nginx.net.connections{kube_cluster_name:%%tag_kube_cluster_name%%,kube_service:nginx}",
			templatedTags: templatedTagsStub,
			loadFunc:      func() {},
			cleanupFunc:   func() {},
			want:          "avg:nginx.connections.accepted{kube_cluster_name:cluster-foo,kube_service:nginx}/avg:nginx.net.connections{kube_cluster_name:cluster-foo,kube_service:nginx}",
			wantErr:       false,
		},
		{
			name:          "1 env, multiple references",
			q:             "avg:nginx.connections.accepted{kube_cluster_name:%%env_DD_CLUSTER_NAME%%,kube_service:nginx}/avg:nginx.net.connections{kube_cluster_name:%%env_DD_CLUSTER_NAME%%,kube_service:nginx}",
			templatedTags: map[string]tagGetter{},
			loadFunc:      func() { os.Setenv("DD_CLUSTER_NAME", "cluster-foo") },
			cleanupFunc:   func() { os.Unsetenv("DD_CLUSTER_NAME") },
			want:          "avg:nginx.connections.accepted{kube_cluster_name:cluster-foo,kube_service:nginx}/avg:nginx.net.connections{kube_cluster_name:cluster-foo,kube_service:nginx}",
			wantErr:       false,
		},
		{
			name: "multiple tag values",
			q:    "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:%%tag_kube_cluster_name%%,datacenter:%%tag_datacenter%%}.rollup(60)",
			templatedTags: map[string]tagGetter{
				"kube_cluster_name": func() (string, error) { return "cluster-foo", nil },
				"datacenter":        func() (string, error) { return "dc-foo", nil },
			},
			loadFunc:    func() {},
			cleanupFunc: func() {},
			want:        "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:cluster-foo,datacenter:dc-foo}.rollup(60)",
			wantErr:     false,
		},
		{
			name:          "multiple env values",
			q:             "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:%%env_DD_CLUSTER_NAME%%,datacenter:%%env_DATACENTER%%}.rollup(60)",
			templatedTags: map[string]tagGetter{},
			loadFunc:      func() { os.Setenv("DD_CLUSTER_NAME", "cluster-foo"); os.Setenv("DATACENTER", "dc-foo") },
			cleanupFunc:   func() {},
			want:          "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:cluster-foo,datacenter:dc-foo}.rollup(60)",
			wantErr:       false,
		},
		{
			name:          "mixing env and tag",
			q:             "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:%%tag_kube_cluster_name%%,datacenter:%%env_DATACENTER%%}.rollup(60)",
			templatedTags: templatedTagsStub,
			loadFunc:      func() { os.Setenv("DATACENTER", "dc-foo") },
			cleanupFunc:   func() { os.Unsetenv("DATACENTER") },
			want:          "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:cluster-foo,datacenter:dc-foo}.rollup(60)",
			wantErr:       false,
		},
		{
			name: "cannot get tag",
			q:    "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:%%tag_kube_cluster_name%%}.rollup(60)",
			templatedTags: map[string]tagGetter{
				"kube_cluster_name": func() (string, error) { return "", errors.New("cannot get tag") },
			},
			loadFunc:    func() {},
			cleanupFunc: func() {},
			want:        "",
			wantErr:     true,
		},
		{
			name:          "unknown tag",
			q:             "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:%%tag_unknown%%}.rollup(60)",
			templatedTags: templatedTagsStub,
			loadFunc:      func() {},
			cleanupFunc:   func() {},
			want:          "",
			wantErr:       true,
		},
		{
			name:          "env not found",
			q:             "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:%%env_NOT_FOUND%%}.rollup(60)",
			templatedTags: map[string]tagGetter{},
			loadFunc:      func() {},
			cleanupFunc:   func() {},
			want:          "",
			wantErr:       true,
		},
		{
			name:          "unknown template variable",
			q:             "avg:nginx.net.request_per_s{kube_container_name:nginx,kube_cluster_name:%%foo_unknown%%}.rollup(60)",
			templatedTags: templatedTagsStub,
			loadFunc:      func() {},
			cleanupFunc:   func() {},
			want:          "",
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			templatedTags = tt.templatedTags
			tt.loadFunc()
			defer tt.cleanupFunc()
			got, err := resolveQuery(tt.q)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.want, got)
		})
	}
}
