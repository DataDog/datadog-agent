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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/singlestep"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

//go:embed testdata/helm_values.yaml
var helmValues string

// expectedRegistry returns the container registry the admission controller
// should auto-select for the current E2E provisioner.
func expectedRegistry() string {
	provisioner, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.Provisioner, "")
	if err == nil {
		switch strings.ToLower(provisioner) {
		case "eks":
			return "public.ecr.aws/datadog"
		case "gke":
			return "gcr.io/datadoghq"
		case "aks":
			return "datadoghq.azurecr.io"
		}
	}
	// Kind (local or AWS) — no cloud provider detected.
	return "registry.datadoghq.com"
}

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
// admission controller use the expected cloud-provider-specific registry.
func (s *registrySuite) TestInitContainerRegistry() {
	k8s := s.Env().KubernetesCluster.Client()
	expected := expectedRegistry()
	s.T().Logf("Expecting init container images from registry %q", expected)

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods("registry-test").List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=registry-test-app",
		})
		require.NoError(c, err, "failed to list pods")
		require.NotEmpty(c, pods.Items, "no pods found for registry-test-app")

		pod := pods.Items[0]
		require.NotEmpty(c, pod.Spec.InitContainers, "pod has no init containers — admission controller may not have mutated it")

		for _, ic := range pod.Spec.InitContainers {
			if !isDatadogInitContainer(ic.Name) {
				continue
			}
			require.True(c, strings.HasPrefix(ic.Image, expected+"/"),
				"init container %q image %q does not use expected registry %q", ic.Name, ic.Image, expected)
		}
	}, 3*time.Minute, 10*time.Second)
}

// TestInitContainersRunning verifies that all Datadog-injected init containers
// complete successfully and the pod is not stuck in ImagePullBackOff or
// ErrImagePull.
func (s *registrySuite) TestInitContainersRunning() {
	k8s := s.Env().KubernetesCluster.Client()

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods("registry-test").List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=registry-test-app",
		})
		require.NoError(c, err, "failed to list pods")
		require.NotEmpty(c, pods.Items, "no pods found for registry-test-app")

		pod := pods.Items[0]

		// Check that no init container is in a waiting state with an image pull error.
		for _, cs := range pod.Status.InitContainerStatuses {
			if !isDatadogInitContainer(cs.Name) {
				continue
			}
			if cs.State.Waiting != nil {
				require.NotContains(c, cs.State.Waiting.Reason, "ImagePullBackOff",
					"init container %q is in ImagePullBackOff: %s", cs.Name, cs.State.Waiting.Message)
				require.NotContains(c, cs.State.Waiting.Reason, "ErrImagePull",
					"init container %q has ErrImagePull: %s", cs.Name, cs.State.Waiting.Message)
			}
		}

		// Verify the pod reaches Running or Succeeded phase (init containers completed).
		require.Contains(c, []corev1.PodPhase{corev1.PodRunning, corev1.PodSucceeded}, pod.Status.Phase,
			"pod phase is %q, expected Running or Succeeded", pod.Status.Phase)
	}, 5*time.Minute, 10*time.Second)
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
