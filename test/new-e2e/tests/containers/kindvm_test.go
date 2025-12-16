// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	_ "embed"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	scenkind "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	provkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

//go:embed fixtures/datadog-agent-secrets-refresh.yml
var secretsBackendRefresh string

type kindSuite struct {
	k8sSuite
}

func TestKindSuite(t *testing.T) {
	e2e.Run(t, &kindSuite{}, e2e.WithProvisioner(provkind.Provisioner(
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
				kubernetesagentparams.WithHelmValues(secretsBackendRefresh),
			),
			scenkind.WithDeployArgoRollout(),
			scenkind.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*compkube.Workload, error) {
				return redis.K8sAppDefinitionWithPassword(e, kubeProvider, "workload-secret-refresh", "redis-with-secret")
			}),
		),
	)))
}

func (suite *kindSuite) SetupSuite() {
	suite.k8sSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
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
		Optional: testMetricExpectArgs{
			Tags: &[]string{
				`^contentType:`,
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
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kube_scheduler.schedule_attempts",
		},
		Expect: testMetricExpectArgs{
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

// TestAutodiscoveryRefreshesCheckSecrets tests that when a secret used by a check is updated,
// the check configuration is refreshed to use the new secret value.
func (suite *kindSuite) TestAutodiscoveryRefreshesCheckSecrets() {
	ctx := suite.T().Context()
	namespace := "workload-secret-refresh"

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "redis.net.instantaneous_ops_per_sec",
			Tags: []string{
				`^container_name:redis$`,
				`^kube_namespace:workload-secret-refresh$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:redis$`,
				`^display_container_name:redis`,
				`^kube_namespace:workload-secret-refresh$`,
				`^image_id:ghcr\.io/datadog/redis@sha256:`,
				`^image_name:ghcr\.io/datadog/redis$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^kube_container_name:redis$`,
				`^short_image:redis$`,
			},
			AcceptUnexpectedTags: true,
		},
	})

	k8sClient := suite.Env().KubernetesCluster.Client()
	_, err := k8sClient.CoreV1().Secrets(namespace).Patch(
		ctx,
		"redis-secret",
		types.StrategicMergePatchType,
		[]byte(fmt.Sprintf(`{"stringData": {"%s": "%s"}}`, "password", "new_s3cr3t")),
		metav1.PatchOptions{},
	)
	suite.Require().NoError(err, "updating secret failed")

	// Redeploy redis to pick up the new password
	suite.T().Log("Redeploying redis service with new password.")
	deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, "redis-with-secret", metav1.GetOptions{})
	suite.Require().NoError(err, "Couldn't find redis deployment")

	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = map[string]string{}
	}
	deployment.Spec.Template.Annotations["force-redeploy"] = time.Now().String()

	_, err = k8sClient.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	suite.Require().NoError(err, "Failed to recreate redis deployment")

	// Wait for the deployment to be ready before flushing the fake intake
	suite.T().Log("Waiting for redis deployment to be ready after update.")
	suite.Require().EventuallyWithT(func(c *assert.CollectT) {
		updatedDeployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, "redis-with-secret", metav1.GetOptions{})
		if !assert.NoError(c, err) {
			c.Errorf("failed to get deployment redis-with-secret in namespace %s: %v", namespace, err)
			return
		}

		// Check that the deployment has completed its rollout
		if !assert.NotEqual(c, updatedDeployment.Status.AvailableReplicas, 0) {
			c.Errorf("deployment redis-with-secret has 0 available replicas")
			return
		}

		// Check that the observed generation matches the desired generation
		if !assert.NotEqual(c, updatedDeployment.Status.ObservedGeneration, updatedDeployment.Generation) {
			c.Errorf("deployment redis-with-secret is still rolling out (observedGeneration: %d, generation: %d)",
				updatedDeployment.Status.ObservedGeneration, updatedDeployment.Generation)
		}
	}, 2*time.Minute, 5*time.Second, "redis deployment did not become ready after update")

	suite.T().Log("Flushing fake intake.")
	err = suite.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	suite.Require().NoError(err, "Failed to reset fake intake")

	// Should continue to receive redis metrics when check secret is refreshed. It refresh fails then metrics
	// will stop because of failed authentication.
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "redis.net.instantaneous_ops_per_sec",
			Tags: []string{
				`^container_name:redis$`,
				`^kube_namespace:workload-secret-refresh$`,
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:redis$`,
				`^display_container_name:redis`,
				`^kube_namespace:workload-secret-refresh$`,
				`^image_id:ghcr\.io/datadog/redis@sha256:`,
				`^image_name:ghcr\.io/datadog/redis$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^kube_container_name:redis$`,
				`^short_image:redis$`,
			},
			AcceptUnexpectedTags: true,
		},
	})
}
