// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/kindvm"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type kindSuite struct {
	k8sSuite
}

func TestKindSuite(t *testing.T) {
	suite.Run(t, &kindSuite{})
}

func (suite *kindSuite) SetupSuite() {
	ctx := context.Background()

	stackConfig := runner.ConfigMap{
		"ddagent:deploy":        auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake":    auto.ConfigValue{Value: "true"},
		"ddtestworkload:deploy": auto.ConfigValue{Value: "true"},
		"dddogstatsd:deploy":    auto.ConfigValue{Value: "true"},
	}

	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(ctx, "kind-cluster", stackConfig, kindvm.Run, false, nil)
	if !suite.Assert().NoError(err) {
		stackName, err := infra.GetStackManager().GetPulumiStackName("kind-cluster")
		suite.Require().NoError(err)
		suite.T().Log(dumpKindClusterState(ctx, stackName))
		if !runner.GetProfile().AllowDevMode() || !*keepStacks {
			infra.GetStackManager().DeleteStack(ctx, "kind-cluster", nil)
		}
		suite.T().FailNow()
	}

	fakeintakeHost := stackOutput.Outputs["fakeintake-host"].Value.(string)
	suite.Fakeintake = fakeintake.NewClient(fmt.Sprintf("http://%s", fakeintakeHost))
	suite.KubeClusterName = stackOutput.Outputs["kube-cluster-name"].Value.(string)
	suite.AgentLinuxHelmInstallName = stackOutput.Outputs["agent-linux-helm-install-name"].Value.(string)
	suite.AgentWindowsHelmInstallName = "none"

	kubeconfig := stackOutput.Outputs["kubeconfig"].Value.(string)

	kubeconfigFile := path.Join(suite.T().TempDir(), "kubeconfig")
	suite.Require().NoError(os.WriteFile(kubeconfigFile, []byte(kubeconfig), 0o600))

	suite.K8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigFile)
	suite.Require().NoError(err)

	suite.K8sClient = kubernetes.NewForConfigOrDie(suite.K8sConfig)

	suite.k8sSuite.SetupSuite()
}

func (suite *kindSuite) TearDownSuite() {
	suite.k8sSuite.TearDownSuite()

	ctx := context.Background()
	stackName, err := infra.GetStackManager().GetPulumiStackName("kind-cluster")
	suite.Require().NoError(err)
	suite.T().Log(dumpKindClusterState(ctx, stackName))
}

func (suite *kindSuite) TestControlPlane() {
	// Test `kube_apiserver` check is properly working
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kube_apiserver.apiserver_request_total",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^apiserver:`,
				`^code:[[:digit:]]{3}$`,
				`^component:(?:|apiserver)$`,
				`^container_id:`,
				`^container_name:kube-apiserver$`,
				`^display_container_name:kube-apiserver_kube-apiserver-kind-control-plane$`,
				`^dry_run:$`,
				`^group:`,
				`^image_id:`,
				`^image_name:registry.k8s.io/kube-apiserver$`,
				`^image_tag:v1\.`,
				`^kube_container_name:kube-apiserver$`,
				`^kube_namespace:kube-system$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^pod_name:kube-apiserver-kind-control-plane$`,
				`^pod_phase:running$`,
				`^resource:`,
				`^scope:(?:|cluster|namespace|resource)$`,
				`^short_image:kube-apiserver$`,
				`^subresource:`,
				`^verb:(?:APPLY|DELETE|GET|LIST|PATCH|POST|PUT|PATCH)$`,
				`^version:`,
			},
		},
	})

	// Test `kube_controller_manager` check is properly working
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kube_controller_manager.queue.adds",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:kube-controller-manager$`,
				`^display_container_name:kube-controller-manager_kube-controller-manager-kind-control-plane$`,
				`^image_id:`,
				`^image_name:registry.k8s.io/kube-controller-manager$`,
				`^image_tag:v1\.`,
				`^kube_container_name:kube-controller-manager$`,
				`^kube_namespace:kube-system$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^pod_name:kube-controller-manager-kind-control-plane$`,
				`^pod_phase:running$`,
				`^queue:`,
				`^short_image:kube-controller-manager$`,
			},
		},
	})

	// Test `kube_scheduler` check is properly working
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kube_scheduler.schedule_attempts",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:kube-scheduler$`,
				`^display_container_name:kube-scheduler_kube-scheduler-kind-control-plane$`,
				`^image_id:`,
				`^image_name:registry.k8s.io/kube-scheduler$`,
				`^image_tag:v1\.`,
				`^kube_container_name:kube-scheduler$`,
				`^kube_namespace:kube-system$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^pod_name:kube-scheduler-kind-control-plane$`,
				`^pod_phase:running$`,
				`^profile:default-scheduler$`,
				`^result:(?:scheduled|unschedulable|error)$`,
				`^short_image:kube-scheduler$`,
			},
		},
	})
}
