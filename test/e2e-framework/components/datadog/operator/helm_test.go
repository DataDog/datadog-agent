// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package operator

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/dda"
)

type operatorTestEnv struct {
	config.Env
	ctx *pulumi.Context
}

func (e *operatorTestEnv) Ctx() *pulumi.Context {
	return e.ctx
}

func (e *operatorTestEnv) WithProviders(...config.ProviderID) pulumi.ResourceOption {
	return pulumi.Providers()
}

func (e *operatorTestEnv) AgentAPIKey() pulumi.StringOutput {
	return pulumi.String("api-key").ToStringOutput()
}

func (e *operatorTestEnv) AgentAPPKey() pulumi.StringOutput {
	return pulumi.String("app-key").ToStringOutput()
}

func (e *operatorTestEnv) ImagePullRegistry() string {
	return "registry.example.com"
}

func (e *operatorTestEnv) ImagePullUsername() string {
	return "username"
}

func (e *operatorTestEnv) ImagePullPassword() pulumi.StringOutput {
	return pulumi.String("password").ToStringOutput()
}

func (e *operatorTestEnv) OperatorFullImagePath() string {
	return "registry.example.com/operator:latest"
}

type operatorMocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *operatorMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func (m *operatorMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources = append(m.resources, args)
	return args.Name + "-id", args.Inputs, nil
}

func (m *operatorMocks) kubernetesResourceIdentities() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var identities []string
	for _, registeredResource := range m.resources {
		resourceType := string(registeredResource.TypeToken)
		if !strings.HasPrefix(resourceType, "kubernetes:") {
			continue
		}

		metadata, ok := registeredResource.Inputs["metadata"]
		if !ok || !metadata.IsObject() {
			continue
		}
		metadataObject := metadata.ObjectValue()
		name, ok := metadataObject["name"]
		if !ok || !name.IsString() {
			return nil, fmt.Errorf("Kubernetes resource %q has no concrete metadata name", registeredResource.Name)
		}
		namespace := ""
		if namespaceValue, ok := metadataObject["namespace"]; ok && namespaceValue.IsString() {
			namespace = namespaceValue.StringValue()
		}

		// Patch resources address the same Kubernetes kind as their non-patch counterparts.
		resourceType = strings.TrimSuffix(resourceType, "Patch")
		identities = append(identities, fmt.Sprintf("%s/%s/%s", resourceType, namespace, name.StringValue()))
	}
	return identities, nil
}

func TestOperatorAndDDAResourcesHaveDistinctKubernetesIdentities(t *testing.T) {
	mocks := &operatorMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		env := &operatorTestEnv{ctx: ctx}
		kubeProvider, err := kubernetes.NewProvider(ctx, "kubernetes", nil)
		if err != nil {
			return err
		}

		_, err = NewHelmInstallation(env, HelmInstallationArgs{
			KubeProvider:          kubeProvider,
			Namespace:             "e2e-operator",
			OperatorFullImagePath: "registry.example.com/operator:latest",
			ChartPath:             "datadog-operator",
			RepoURL:               "https://helm.datadoghq.com",
		})
		if err != nil {
			return err
		}

		params, err := agentwithoperatorparams.NewParams(agentwithoperatorparams.WithNamespace("e2e-operator"))
		if err != nil {
			return err
		}
		_, err = dda.K8sAppDefinition(env, kubeProvider, params)
		return err
	}, pulumi.WithMocks("project", "stack", mocks))
	if err != nil {
		t.Fatalf("installing Operator and DDA components: %v", err)
	}

	identities, err := mocks.kubernetesResourceIdentities()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]struct{}, len(identities))
	for _, identity := range identities {
		if _, exists := seen[identity]; exists {
			t.Errorf("Kubernetes resource %q is independently owned more than once", identity)
		}
		seen[identity] = struct{}{}
	}

	for _, secretName := range []string{
		operatorCredentialsSecretName,
		operatorImagePullSecretName,
		"dda-datadog-credentials",
		utils.DefaultImagePullSecretName,
	} {
		identity := fmt.Sprintf("kubernetes:core/v1:Secret/e2e-operator/%s", secretName)
		if _, exists := seen[identity]; !exists {
			t.Errorf("expected Kubernetes resource %q was not registered", identity)
		}
	}
}
