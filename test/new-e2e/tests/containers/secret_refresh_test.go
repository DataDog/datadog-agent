// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redis"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

//go:embed fixtures/datadog-agent-secrets-refresh.yml
var secretsBackendRefresh string

type secretRefreshSuite struct {
	baseSuite[environments.Kubernetes]
}

// TestSecretRefreshSuite runs the secret refresh test suite
func TestSecretRefreshSuite(t *testing.T) {
	e2e.Run(t, &secretRefreshSuite{}, e2e.WithProvisioner(
		awskubernetes.Provisioner(
			awskubernetes.WithRunOptions(
				scenariokindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(secretsBackendRefresh),
				),
				scenariokindvm.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*compkube.Workload, error) {
					return redis.K8sAppDefinitionWithPassword(e, kubeProvider, "secret-refresh-workload", "redis-with-secret")
				}),
			),
		),
	))
}

// TestRedisCheckSecretRefresh tests that when a secret used by a redis check is updated,
// the check configuration is refreshed to use the new secret value
func (suite *secretRefreshSuite) TestRedisCheckSecretRefresh() {
	ctx := context.Background()
	namespace := "secret-refresh-workload"
	deploymentName := "redis-with-secret"

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "redis.net.instantaneous_ops_per_sec",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:redis$`,
				`^display_container_name:redis`,
				`^kube_namespace:secret-refresh-workload$`,
				`^image_id:ghcr\.io/datadog/redis@sha256:`,
				`^image_name:ghcr\.io/datadog/redis$`,
				`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
				`^kube_container_name:redis$`,
				`^short_image:redis$`,
			},
			AcceptUnexpectedTags: true,
		},
	})

	suite.T().Log("Password protected redis check successfully scheduled by autodiscovery.")

	k8sClient := suite.Env().KubernetesCluster.Client()
	_, err := k8sClient.CoreV1().Secrets(namespace).Patch(
		ctx,
		"redis-secret",
		types.StrategicMergePatchType,
		[]byte(fmt.Sprintf(`{"stringData": {"%s": "%s"}}`, "password", "new_s3cr3t")),
		metav1.PatchOptions{},
	)
	require.NoError(suite.T(), err, "updating secret failed")

	// Step 7: Redeploy redis to pick up the new password
	suite.T().Log("Redeploying redis service with new password.")
	deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	require.NoError(suite.T(), err, "Couldn't find redis deployment")

	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = map[string]string{}
	}
	deployment.Spec.Template.Annotations["force-redeploy"] = time.Now().String()

	_, err = k8sClient.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	require.NoError(suite.T(), err, "Failed to recreate redis deployment")

	// Step 8: Reset fake intake
	err = suite.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(suite.T(), err, "Failed to reset fake intake")

	// Step 9: Verify redis check can connect (implicit test that it's using new password)\
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "redis.net.instantaneous_ops_per_sec",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:redis$`,
				`^display_container_name:redis`,
				`^kube_namespace:secret-refresh-workload$`,
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
