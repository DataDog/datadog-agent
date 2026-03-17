// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package admissioncontroller contains E2E tests for admission controller features.
package admissioncontroller

import (
	"context"
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/singlestep"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

//go:embed testdata/helm_values.yaml
var helmValues string

// registrySuite verifies that the admission controller uses the correct
// container registry based on the detected cloud provider when no explicit
// registry is configured.
type registrySuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestRegistryAutoDetection(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &registrySuite{}, e2e.WithProvisioner(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(helmValues),
		},
		AgentDependentWorkloadAppFunc: func(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*compkube.Workload, error) {
			return singlestep.Scenario(e, kubeProvider, "registry-test", []singlestep.Namespace{
				{
					Name: "registry-test",
					Apps: []singlestep.App{
						{
							Name:    "registry-test-app",
							Image:   "registry.datadoghq.com/injector-dev/python",
							Version: "16ad9d4b",
							Port:    8080,
						},
					},
				},
			}, dependsOnAgent)
		},
	})))
}

// TestInitContainerRegistry verifies that init containers injected by the
// admission controller use a Datadog container registry. On Kind (no cloud
// provider detected), the default is registry.datadoghq.com. On EKS it should
// be public.ecr.aws/datadog, on GKE gcr.io/datadoghq, on AKS datadoghq.azurecr.io.
func (s *registrySuite) TestInitContainerRegistry() {
	k8s := s.Env().KubernetesCluster.Client()

	// Wait for the pod to be created and mutated by the admission controller.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods("registry-test").List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=registry-test-app",
		})
		require.NoError(c, err, "failed to list pods")
		require.NotEmpty(c, pods.Items, "no pods found for registry-test-app")

		pod := pods.Items[0]
		require.NotEmpty(c, pod.Spec.InitContainers, "pod has no init containers — admission controller may not have mutated it")

		// Verify all init containers injected by the admission controller use a
		// known Datadog registry. The specific registry depends on the cloud
		// provider: Kind → registry.datadoghq.com, EKS → public.ecr.aws/datadog,
		// GKE → gcr.io/datadoghq, AKS → datadoghq.azurecr.io.
		knownRegistries := []string{
			"registry.datadoghq.com",
			"public.ecr.aws/datadog",
			"gcr.io/datadoghq",
			"datadoghq.azurecr.io",
			"docker.io/datadog",
		}

		for _, ic := range pod.Spec.InitContainers {
			// Skip init containers that aren't Datadog-injected (e.g., user-defined).
			if !isDatadogInitContainer(ic.Name) {
				continue
			}
			matched := false
			for _, reg := range knownRegistries {
				if strings.HasPrefix(ic.Image, reg+"/") {
					matched = true
					break
				}
			}
			require.True(c, matched, "init container %q image %q does not use a known Datadog registry", ic.Name, ic.Image)
		}
	}, 3*time.Minute, 10*time.Second)
}

// isDatadogInitContainer returns true for init container names that are
// typically injected by the Datadog admission controller.
func isDatadogInitContainer(name string) bool {
	prefixes := []string{
		"datadog-lib-",
		"datadog-init-",
		"dd-lib-",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
