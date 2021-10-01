// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKubeletSvcEqual(t *testing.T) {
	tests := []struct {
		name   string
		first  Service
		second Service
		want   bool
	}{
		{
			name:   "equal",
			first:  &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.1"}, adIdentifiers: []string{"foo"}, ports: []ContainerPort{{Port: 80, Name: "http"}}, checkNames: []string{"foo_check"}, ready: true},
			second: &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.1"}, adIdentifiers: []string{"foo"}, ports: []ContainerPort{{Port: 80, Name: "http"}}, checkNames: []string{"foo_check"}, ready: true},
			want:   true,
		},
		{
			name:   "host change",
			first:  &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.1"}, adIdentifiers: []string{"foo"}, ports: []ContainerPort{{Port: 80, Name: "http"}}, checkNames: []string{"foo_check"}, ready: true},
			second: &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.2"}, adIdentifiers: []string{"foo"}, ports: []ContainerPort{{Port: 80, Name: "http"}}, checkNames: []string{"foo_check"}, ready: true},
			want:   false,
		},
		{
			name:   "ad change",
			first:  &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.1"}, adIdentifiers: []string{"foo"}, ports: []ContainerPort{{Port: 80, Name: "http"}}, checkNames: []string{"foo_check"}, ready: true},
			second: &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.1"}, adIdentifiers: []string{"bar"}, ports: []ContainerPort{{Port: 80, Name: "http"}}, checkNames: []string{"foo_check"}, ready: true},
			want:   false,
		},
		{
			name:   "port change",
			first:  &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.1"}, adIdentifiers: []string{"foo"}, ports: []ContainerPort{{Port: 80, Name: "http"}}, checkNames: []string{"foo_check"}, ready: true},
			second: &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.1"}, adIdentifiers: []string{"foo"}, ports: []ContainerPort{{Port: 8080, Name: "http"}}, checkNames: []string{"foo_check"}, ready: true},
			want:   false,
		},
		{
			name:   "checkname change",
			first:  &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.1"}, adIdentifiers: []string{"foo"}, ports: []ContainerPort{{Port: 80, Name: "http"}}, checkNames: []string{"foo_check"}, ready: true},
			second: &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.1"}, adIdentifiers: []string{"foo"}, ports: []ContainerPort{{Port: 80, Name: "http"}}, checkNames: []string{"bar_check"}, ready: true},
			want:   false,
		},
		{
			name:   "rediness change",
			first:  &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.1"}, adIdentifiers: []string{"foo"}, ports: []ContainerPort{{Port: 80, Name: "http"}}, checkNames: []string{"foo_check"}, ready: true},
			second: &KubeContainerService{hosts: map[string]string{"pod": "10.0.1.1"}, adIdentifiers: []string{"foo"}, ports: []ContainerPort{{Port: 80, Name: "http"}}, checkNames: []string{"foo_check"}, ready: false},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, kubeletSvcEqual(tt.first, tt.second))
		})
	}
}
