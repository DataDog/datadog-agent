// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package discovery

import (
	"fmt"
	"testing"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awskind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	endpointChecksNamespace    = "workload-nginx-endpoints"
	endpointChecksServiceName  = "nginx-endpoints"
	endpointChecksInstanceName = "Nginx_Endpoints"
)

type endpointChecksSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	fakeintake *fakeintake.Client
}

func TestEndpointChecksSuiteWithKubeEndpoints(t *testing.T) {
	e2e.Run(t, &endpointChecksSuite{}, e2e.WithProvisioner(endpointChecksProvisioner(false)))
}

func TestEndpointChecksSuiteWithEndpointSlices(t *testing.T) {
	e2e.Run(t, &endpointChecksSuite{}, e2e.WithProvisioner(endpointChecksProvisioner(true)))
}

func (s *endpointChecksSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.fakeintake = s.Env().FakeIntake.Client()
}

func (s *endpointChecksSuite) TestEndpointChecksAreScheduled() {
	expectedTags := []string{
		"kube_namespace:" + endpointChecksNamespace,
		"kube_service:" + endpointChecksServiceName,
		"instance:" + endpointChecksInstanceName,
	}

	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.fakeintake.FilterMetrics(
			"network.http.response_time",
			fakeintake.WithTags[*aggregator.MetricSeries](expectedTags),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 3*time.Minute, 10*time.Second, "Expected endpoint check metrics for %s", endpointChecksServiceName)
}

func endpointChecksProvisioner(useEndpointSlices bool) provisioners.Provisioner {
	return awskind.Provisioner(
		awskind.WithRunOptions(
			scenkindvm.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(endpointChecksHelmValues(useEndpointSlices)),
			),
			scenkindvm.WithWorkloadApp(endpointChecksWorkloadApp),
		),
	)
}

func endpointChecksHelmValues(useEndpointSlices bool) string {
	useEndpointSlicesValue := "false"
	if useEndpointSlices {
		useEndpointSlicesValue = "true"
	}

	return fmt.Sprintf(`
datadog:
  env:
    - name: DD_EXTRA_CONFIG_PROVIDERS
      value: "clusterchecks endpointschecks"
agents:
  env:
    - name: DD_EXTRA_CONFIG_PROVIDERS
      value: "clusterchecks endpointschecks"
clusterChecksRunner:
  env:
    - name: DD_EXTRA_CONFIG_PROVIDERS
      value: "clusterchecks endpointschecks"
clusterAgent:
  env:
    - name: DD_EXTRA_LISTENERS
      value: "kube_endpoints"
    - name: DD_EXTRA_CONFIG_PROVIDERS
      value: "kube_endpoints"
    - name: DD_KUBERNETES_USE_ENDPOINT_SLICES
      value: "%s"
`, useEndpointSlicesValue)
}

func endpointChecksWorkloadApp(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
	workload, err := nginx.K8sAppDefinition(e, kubeProvider, endpointChecksNamespace, 80, "", false)
	if err != nil {
		return nil, err
	}

	endpointCheckConfig := utils.JSONMustMarshal(map[string]interface{}{
		"http_check": map[string]interface{}{
			"init_config": map[string]interface{}{},
			"instances": []map[string]interface{}{
				{
					"name":    endpointChecksInstanceName,
					"url":     "http://%%host%%",
					"timeout": 1,
				},
			},
		},
	})

	_, err = corev1.NewService(e.Ctx(), endpointChecksNamespace+"/nginx-endpoints", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(endpointChecksServiceName),
			Namespace: pulumi.String(endpointChecksNamespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("nginx-endpoints"),
			},
			Annotations: pulumi.StringMap{
				"ad.datadoghq.com/endpoints.checks": pulumi.String(endpointCheckConfig),
			},
		},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{
				"app": pulumi.String("nginx-endpoints"),
			},
			Ports: &corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(80),
					TargetPort: pulumi.String("http"),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}, pulumi.Provider(kubeProvider), pulumi.Parent(workload), utils.PulumiDependsOn(workload))
	if err != nil {
		return nil, err
	}

	return workload, nil
}
