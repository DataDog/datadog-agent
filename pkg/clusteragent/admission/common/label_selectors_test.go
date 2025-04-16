// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

var (
	globalUnlabelledSetting  = "admission_controller.validation.unlabelled"
	webhookUnlabelledSetting = "admission_controller.test.unlabelled"
)

// DefaultValidatingLabelSelectors returns the validating webhooks object selector based on the configuration.
func TestDefaultValidatingLabelSelectors(t *testing.T) {
	tests := []struct {
		name string

		useNamespaceSelector bool
		globalUnlabelled     *string
		webhookUnlabelled    *string

		wantNamespaceSelector *metav1.LabelSelector
		wantObjectSelector    *metav1.LabelSelector
	}{
		{
			name:                  "Default unlabelled settings",
			useNamespaceSelector:  false,
			wantNamespaceSelector: nil,
			wantObjectSelector:    &acceptAllLabelSelector,
		},
		{
			name:                 "Webhook unlabelled setting explicitly set to true",
			useNamespaceSelector: false,
			webhookUnlabelled: func() *string {
				s := "true"
				return &s
			}(),
			wantNamespaceSelector: nil,
			wantObjectSelector:    &acceptAllLabelSelector,
		},
		{
			name:                 "Webhook unlabelled setting explicitly set to false, global unlabelled set to true",
			useNamespaceSelector: false,
			globalUnlabelled: func() *string {
				s := "true"
				return &s
			}(),
			webhookUnlabelled: func() *string {
				s := "false"
				return &s
			}(),
			wantNamespaceSelector: nil,
			wantObjectSelector:    &acceptOnlyLabelSelector,
		},
		{
			name:                 "Global unlabelled set to true",
			useNamespaceSelector: false,
			globalUnlabelled: func() *string {
				s := "true"
				return &s
			}(),
			wantNamespaceSelector: nil,
			wantObjectSelector:    &acceptAllLabelSelector,
		},
		{
			name:                 "Global unlabelled set to false",
			useNamespaceSelector: false,
			globalUnlabelled: func() *string {
				s := "false"
				return &s
			}(),
			wantNamespaceSelector: nil,
			wantObjectSelector:    &acceptOnlyLabelSelector,
		},
		{
			name:                  "useNamespaceSelector set to true",
			useNamespaceSelector:  true,
			wantNamespaceSelector: &acceptAllLabelSelector,
			wantObjectSelector:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			if tt.globalUnlabelled != nil {
				mockConfig.SetWithoutSource(globalUnlabelledSetting, *tt.globalUnlabelled)
				defer mockConfig.UnsetForSource(globalUnlabelledSetting, model.SourceUnknown)
			}
			if tt.webhookUnlabelled != nil {
				mockConfig.SetWithoutSource(webhookUnlabelledSetting, *tt.webhookUnlabelled)
				defer mockConfig.UnsetForSource(webhookUnlabelledSetting, model.SourceUnknown)
			}

			namespaceSelector, objectSelector := DefaultValidatingLabelSelectors(tt.useNamespaceSelector, mockConfig, webhookUnlabelledSetting)
			assert.Equal(t, tt.wantNamespaceSelector, namespaceSelector)
			assert.Equal(t, tt.wantObjectSelector, objectSelector)
		})
	}
}
