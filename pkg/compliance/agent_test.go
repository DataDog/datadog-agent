// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import "testing"

func TestMakeDefaultRuleFilter(t *testing.T) {
	tests := []struct {
		name     string
		isK8s    bool
		cri      string
		rule     *Rule
		wantKept bool
	}{
		{"non-k8s + k8s-scoped rule", false, "", &Rule{Scopes: []RuleScope{KubernetesNodeScope}}, false},
		{"non-k8s + docker rule", false, "", &Rule{Scopes: []RuleScope{DockerScope}}, true},
		{"k8s + SkipOnK8s rule", true, "", &Rule{SkipOnK8s: true}, false},
		{"k8s + docker rule + unknown CRI", true, "", &Rule{Scopes: []RuleScope{DockerScope}}, true},
		{"k8s + docker rule + docker CRI", true, "docker", &Rule{Scopes: []RuleScope{DockerScope}}, true},
		{"k8s + docker rule + containerd CRI", true, "containerd", &Rule{Scopes: []RuleScope{DockerScope}}, false},
		{"k8s + k8s-scoped rule + containerd CRI", true, "containerd", &Rule{Scopes: []RuleScope{KubernetesNodeScope}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := makeDefaultRuleFilter(
				"test-host",
				func() bool { return tt.isK8s },
				func() string { return tt.cri },
			)
			if got := filter(tt.rule); got != tt.wantKept {
				t.Errorf("filter(rule) = %v, want %v", got, tt.wantKept)
			}
		})
	}
}
