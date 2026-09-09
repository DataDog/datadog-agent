// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"fmt"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

type imagePullSecretTestEnv struct {
	config.Env
	ctx *pulumi.Context
}

func (e *imagePullSecretTestEnv) Ctx() *pulumi.Context {
	return e.ctx
}

func (e *imagePullSecretTestEnv) ImagePullRegistry() string {
	return "registry.example.com"
}

func (e *imagePullSecretTestEnv) ImagePullUsername() string {
	return "username"
}

func (e *imagePullSecretTestEnv) ImagePullPassword() pulumi.StringOutput {
	return pulumi.String("password").ToStringOutput()
}

type imagePullSecretMocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *imagePullSecretMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func (m *imagePullSecretMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources = append(m.resources, args)
	return args.Name + "-id", args.Inputs, nil
}

func (m *imagePullSecretMocks) secretResource() (pulumi.MockResourceArgs, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, registeredResource := range m.resources {
		if registeredResource.TypeToken == "kubernetes:core/v1:Secret" {
			return registeredResource, nil
		}
	}
	return pulumi.MockResourceArgs{}, fmt.Errorf("image pull Secret was not registered")
}

func TestNewImagePullSecretNames(t *testing.T) {
	tests := []struct {
		name               string
		secretName         string
		expectedPulumiName string
		useDefault         bool
	}{
		{
			name:               "default",
			secretName:         DefaultImagePullSecretName,
			expectedPulumiName: "registry-credentials-e2e-operator",
			useDefault:         true,
		},
		{
			name:               "component scoped",
			secretName:         "operator-registry-credentials",
			expectedPulumiName: "operator-registry-credentials-e2e-operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks := &imagePullSecretMocks{}
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				env := &imagePullSecretTestEnv{ctx: ctx}
				if tt.useDefault {
					_, err := NewImagePullSecret(env, "e2e-operator")
					return err
				}
				_, err := NewImagePullSecretWithName(env, "e2e-operator", tt.secretName)
				return err
			}, pulumi.WithMocks("project", "stack", mocks))
			if err != nil {
				t.Fatalf("NewImagePullSecretWithName() error = %v", err)
			}

			secret, err := mocks.secretResource()
			if err != nil {
				t.Fatal(err)
			}
			if secret.Name != tt.expectedPulumiName {
				t.Errorf("Pulumi resource name = %q, want %q", secret.Name, tt.expectedPulumiName)
			}

			metadata := secret.Inputs["metadata"].ObjectValue()
			if got := metadata["namespace"].StringValue(); got != "e2e-operator" {
				t.Errorf("Kubernetes namespace = %q, want %q", got, "e2e-operator")
			}
			if got := metadata["name"].StringValue(); got != tt.secretName {
				t.Errorf("Kubernetes name = %q, want %q", got, tt.secretName)
			}
		})
	}
}
