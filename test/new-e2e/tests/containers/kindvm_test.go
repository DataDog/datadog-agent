// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	scenkind "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	provkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

type kindSuite struct {
	k8sSuite
}

func TestKindSuite(t *testing.T) {
	newProvisioner := func(helmValues string) provisioners.Provisioner {
		return provkind.Provisioner(
			provkind.WithRunOptions(
				scenkind.WithVMOptions(
					scenec2.WithInstanceType("t3.xlarge"),
				),
				scenkind.WithFakeintakeOptions(
					fakeintake.WithMemory(2048),
				),
				scenkind.WithDeployDogstatsd(),
				scenkind.WithDeployTestWorkload(),
				scenkind.WithAgentOptions(
					kubernetesagentparams.WithDualShipping(),
					kubernetesagentparams.WithHelmValues(helmValues),
				),
				scenkind.WithDeployArgoRollout(),
			),
		)
	}
	e2e.Run(t, &kindSuite{k8sSuite{newProvisioner: newProvisioner}}, e2e.WithProvisioner(newProvisioner("")))
}

func (suite *kindSuite) SetupSuite() {
	suite.k8sSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
}

func (suite *kindSuite) TestControlPlane() {
	// Test `kube_apiserver` check is properly working
	suite.TestMetric(&TestMetricArgs{
		Filter: TestMetricFilterArgs{
			Name: "kube_apiserver.apiserver_request_total",
		},
		Expect: TestMetricExpectArgs{
			Tags: &[]string{
				`^apiserver:`,
				`^code:[[:digit:]]{3}$`,
				`^component:(?:|apiserver)$`,
				`^container_id:`,
				`^container_name:kube-apiserver$`,
				`^display_container_name:kube-apiserver_kube-apiserver-.*-control-plane$`,
				`^dry_run:$`,
				`^group:`,
				`^image_id:`,
				`^image_name:(?:k8s\.gcr\.io|registry\.k8s\.io)/kube-apiserver$`,
				`^image_tag:v1\.`,
				`^kube_container_name:kube-apiserver$`,
				`^kube_namespace:kube-system$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^pod_name:kube-apiserver-.*-control-plane$`,
				`^pod_phase:running$`,
				`^resource:`,
				`^scope:(?:|cluster|namespace|resource)$`,
				`^short_image:kube-apiserver$`,
				`^subresource:`,
				`^verb:(?:APPLY|DELETE|GET|LIST|PATCH|POST|PUT|PATCH|WATCH|TOTAL)$`,
				`^version:`,
			},
		},
		Optional: TestMetricExpectArgs{
			Tags: &[]string{
				`^contentType:`,
			},
		},
	})

	// Test `kube_controller_manager` check is properly working
	suite.TestMetric(&TestMetricArgs{
		Filter: TestMetricFilterArgs{
			Name: "kube_controller_manager.queue.adds",
		},
		Expect: TestMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:kube-controller-manager$`,
				`^display_container_name:kube-controller-manager_kube-controller-manager-.*-control-plane$`,
				`^image_id:`,
				`^image_name:(?:k8s\.gcr\.io|registry\.k8s\.io)/kube-controller-manager$`,
				`^image_tag:v1\.`,
				`^kube_container_name:kube-controller-manager$`,
				`^kube_namespace:kube-system$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^pod_name:kube-controller-manager-.*-control-plane$`,
				`^pod_phase:running$`,
				`^queue:`,
				`^short_image:kube-controller-manager$`,
			},
		},
	})

	// Test `kube_scheduler` check is properly working
	suite.TestMetric(&TestMetricArgs{
		Filter: TestMetricFilterArgs{
			Name: "kube_scheduler.schedule_attempts",
		},
		Expect: TestMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:kube-scheduler$`,
				`^display_container_name:kube-scheduler_kube-scheduler-.*-control-plane$`,
				`^image_id:`,
				`^image_name:(?:k8s\.gcr\.io|registry\.k8s\.io)/kube-scheduler$`,
				`^image_tag:v1\.`,
				`^kube_container_name:kube-scheduler$`,
				`^kube_namespace:kube-system$`,
				`^kube_priority_class:system-node-critical$`,
				`^kube_qos:Burstable$`,
				`^pod_name:kube-scheduler-.*-control-plane$`,
				`^pod_phase:running$`,
				`^profile:default-scheduler$`,
				`^result:(?:scheduled|unschedulable|error)$`,
				`^short_image:kube-scheduler$`,
			},
		},
	})
}
