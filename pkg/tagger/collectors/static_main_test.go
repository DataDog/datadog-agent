// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func Test_fargateStaticTags(t *testing.T) {
	mockConfig := config.Mock()
	tests := []struct {
		name        string
		loadFunc    func()
		cleanupFunc func()
		want        []string
	}{
		{
			name:        "dd tags",
			loadFunc:    func() { mockConfig.Set("tags", "dd_tag1:dd_val1 dd_tag2:dd_val2") },
			cleanupFunc: func() { mockConfig.Set("tags", "") },
			want:        []string{"dd_tag1:dd_val1", "dd_tag2:dd_val2"},
		},
		{
			name: "eks fargate node",
			loadFunc: func() {
				mockConfig.Set("eks_fargate", true)
				mockConfig.Set("kubernetes_kubelet_nodename", "fargate_node_name")
			},
			cleanupFunc: func() {
				mockConfig.Set("eks_fargate", false)
				mockConfig.Set("kubernetes_kubelet_nodename", "")
			},
			want: []string{"eks_fargate_node:fargate_node_name"},
		},
		{
			name: "dd tags and eks fargate node",
			loadFunc: func() {
				mockConfig.Set("tags", "dd_tag1:dd_val1 dd_tag2:dd_val2")
				mockConfig.Set("eks_fargate", true)
				mockConfig.Set("kubernetes_kubelet_nodename", "fargate_node_name")
			},
			cleanupFunc: func() {
				mockConfig.Set("tags", "")
				mockConfig.Set("eks_fargate", false)
				mockConfig.Set("kubernetes_kubelet_nodename", "")
			},
			want: []string{"dd_tag1:dd_val1", "dd_tag2:dd_val2", "eks_fargate_node:fargate_node_name"},
		},
		{
			name:        "no tags",
			loadFunc:    func() {},
			cleanupFunc: func() {},
			want:        nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.loadFunc()
			defer tt.cleanupFunc()

			assert.Equal(t, tt.want, fargateStaticTags())
		})
	}
}
