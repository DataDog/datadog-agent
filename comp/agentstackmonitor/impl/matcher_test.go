// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentstackmonitorimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestSubjectKindFor(t *testing.T) {
	cases := []struct {
		name      string
		labels    map[string]string
		wantKind  SubjectKind
		wantMatch bool
	}{
		{
			name:      "cluster-agent label matches",
			labels:    map[string]string{"app.kubernetes.io/name": "datadog-cluster-agent"},
			wantKind:  SubjectKindClusterAgent,
			wantMatch: true,
		},
		{
			name:      "cluster-check-runner label matches",
			labels:    map[string]string{"app.kubernetes.io/name": "datadog-cluster-checks-runner"},
			wantKind:  SubjectKindClusterCheckRunner,
			wantMatch: true,
		},
		{
			name:      "unrelated pod does not match",
			labels:    map[string]string{"app.kubernetes.io/name": "some-customer-app"},
			wantMatch: false,
		},
		{
			name:      "wrong label key does not match",
			labels:    map[string]string{"app": "datadog-cluster-agent"},
			wantMatch: false,
		},
		{
			name:      "missing labels does not match",
			labels:    nil,
			wantMatch: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pod := &workloadmeta.KubernetesPod{}
			pod.EntityMeta.Labels = tc.labels
			gotKind, gotMatch := subjectKindFor(pod)
			assert.Equal(t, tc.wantMatch, gotMatch)
			if tc.wantMatch {
				assert.Equal(t, tc.wantKind, gotKind)
			}
		})
	}

	t.Run("nil pod does not match", func(t *testing.T) {
		_, ok := subjectKindFor(nil)
		assert.False(t, ok)
	})
}
