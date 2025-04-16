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
		name              string
		labels            map[string]string
		annotations       map[string]string
		labelsAsTags      map[string]string
		annotationsAsTags map[string]string
		want              []string
	}{
		{
			name: "labels and annotations have matching tags",
			labels: map[string]string{
				"app":  "my-app",
				"team": "my-team",
			},
			annotations: map[string]string{
				"annotation-key": "annotation-value",
			},
			labelsAsTags: map[string]string{
				"app":  "application",
				"team": "team-name",
			},
			annotationsAsTags: map[string]string{
				"annotation-key": "annotation_key",
			},
			want: []string{"application:my-app", "team-name:my-team", "annotation_key:annotation-value"},
		},
		{
			name: "no matching labels or annotations",
			labels: map[string]string{
				"random": "value",
			},
			annotations: map[string]string{
				"another-random": "value",
			},
			labelsAsTags:      map[string]string{"app": "application"},
			annotationsAsTags: map[string]string{"annotation-key": "annotation_key"},
			want:              []string{},
		},
		{
			name: "only annotations match",
			labels: map[string]string{
				"random": "value",
			},
			annotations: map[string]string{
				"annotation-key": "annotation-value",
			},
			labelsAsTags:      map[string]string{"app": "application"},
			annotationsAsTags: map[string]string{"annotation-key": "annotation_key"},
			want:              []string{"annotation_key:annotation-value"},
		},
		{
			name: "only labels match",
			labels: map[string]string{
				"app": "my-app",
			},
			annotations: map[string]string{
				"random-annotation": "value",
			},
			labelsAsTags:      map[string]string{"app": "application"},
			annotationsAsTags: map[string]string{"annotation-key": "annotation_key"},
			want:              []string{"application:my-app"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RetrieveMetadataTags(tt.labels, tt.annotations, tt.labelsAsTags, tt.annotationsAsTags)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
