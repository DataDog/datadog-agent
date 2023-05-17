// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package transformers

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestRetrieveUST(t *testing.T) {
	cfg := config.Mock(t)
	cfg.Set("env", "staging")
	cfg.Set(tagKeyService, "not-applied")
	cfg.Set(tagKeyVersion, "not-applied")

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
