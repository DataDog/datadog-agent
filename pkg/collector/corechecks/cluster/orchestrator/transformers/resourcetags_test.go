// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package transformers

import (
	"reflect"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"github.com/stretchr/testify/assert"
)

func TestRetrieveUST(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("env", "staging")
	cfg.SetWithoutSource(tagKeyService, "not-applied")
	cfg.SetWithoutSource(tagKeyVersion, "not-applied")

	tests := []struct {
		name   string
		labels map[string]string
		want   []string
	}{
		{
			name:   "label contains ust, labels ust takes precedence",
			labels: map[string]string{kubernetes.EnvTagLabelKey: "prod", kubernetes.VersionTagLabelKey: "123", kubernetes.ServiceTagLabelKey: "app"},
			want:   []string{"env:prod", "version:123", "service:app"},
		},
		{
			name:   "label does not contain env, takes from config",
			labels: map[string]string{},
			want:   []string{"env:staging"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RetrieveUnifiedServiceTags(tt.labels); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RetrieveUnifiedServiceTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetrieveMetadataTags(t *testing.T) {
	tests := []struct {
		name                  string
		labels                map[string]string
		annotations           map[string]string
		autoTeamTagCollection bool
		want                  []string
	}{
		{
			name:   "no team in labels or annotations",
			labels: map[string]string{},
			annotations: map[string]string{
				"annotation-key": "annotation-value",
			},
			autoTeamTagCollection: true,
			want:                  []string{},
		},
		{
			name: "auto team tag collection enabled - team in labels",
			labels: map[string]string{
				"team": "platform",
			},
			annotations:           map[string]string{},
			autoTeamTagCollection: true,
			want:                  []string{"team:platform"},
		},
		{
			name:   "auto team tag collection enabled - team in annotations",
			labels: map[string]string{},
			annotations: map[string]string{
				"team": "platform",
			},
			autoTeamTagCollection: true,
			want:                  []string{"team:platform"},
		},
		{
			name: "auto team tag collection enabled - team in both labels and annotations, prefer label",
			labels: map[string]string{
				"team": "platform-label",
			},
			annotations: map[string]string{
				"team": "platform-annotation",
			},
			autoTeamTagCollection: true,
			want:                  []string{"team:platform-label"},
		},
		{
			name: "auto team tag collection disabled - team in labels",
			labels: map[string]string{
				"team": "platform",
			},
			annotations:           map[string]string{},
			autoTeamTagCollection: false,
			want:                  []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetWithoutSource("auto_team_tag_collection", tt.autoTeamTagCollection)

			got := RetrieveMetadataTags(tt.labels, tt.annotations)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
