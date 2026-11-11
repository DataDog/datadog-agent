// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"testing"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeGetResourceAction(apiVersion, kind, namespace string) *kubeactions.KubeAction {
	return &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			ApiVersion: apiVersion,
			Kind:       kind,
			Namespace:  namespace,
			Name:       "my-resource",
			ResourceId: "resource-id",
		},
		Action: &kubeactions.KubeAction_GetResource_{
			GetResource_: &kubeactions.GetResourceParams{},
		},
	}
}

func TestValidateActionGetResource(t *testing.T) {
	tests := []struct {
		testName   string
		apiVersion string
		kind       string
		namespace  string
		wantErr    string
	}{
		{testName: "nominal", apiVersion: "v1", kind: "configmaps", namespace: "default"},
		{testName: "nominal with group", apiVersion: "apps/v1", kind: "deployments", namespace: "default"},
		{testName: "missing api version", apiVersion: "", kind: "configmaps", namespace: "default", wantErr: "resource.api_version must be set"},
		{testName: "protected kind", apiVersion: "v1", kind: "Secrets", namespace: "default", wantErr: "protected kind"},
		{testName: "protected namespace", apiVersion: "v1", kind: "configmaps", namespace: "kube-system", wantErr: "protected namespace"},
		{testName: "protected namespace mixed case", apiVersion: "v1", kind: "configmaps", namespace: "Kube-System", wantErr: "protected namespace"},
		{testName: "protected namespace upper case", apiVersion: "v1", kind: "configmaps", namespace: "KUBE-PUBLIC", wantErr: "protected namespace"},
		{testName: "kind traversal to protected kind", apiVersion: "v1", kind: "configmaps/../secrets", namespace: "default", wantErr: "invalid resource kind"},
		{testName: "kind traversal to protected namespace", apiVersion: "v1", kind: "../kube-system/configmaps", namespace: "default", wantErr: "invalid resource kind"},
		{testName: "version traversal", apiVersion: "apps/v1/../../../api/v1/namespaces/default/secrets", kind: "configmaps", namespace: "default", wantErr: "invalid api version"},
	}

	validator := NewActionValidator()
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			err := validator.ValidateAction(makeGetResourceAction(tt.apiVersion, tt.kind, tt.namespace))
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
