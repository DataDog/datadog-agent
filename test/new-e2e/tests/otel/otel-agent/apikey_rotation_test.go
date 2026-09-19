// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	"context"
	_ "embed"
	"io"
	"strings"
	"testing"
	"time"

	pulumicorev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8scorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	otelstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/otel-standalone"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	provlocal "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
)

//go:embed config/apikey-rotation.yml
var apiKeyRotationConfig string

const (
	apiKeyRotationNamespace = "datadog"
	apiKeyRotationSecret    = "otel-apikey-rotation"
	apiKeyRotationAppLabel  = "standalone-otel-agent"
	apiKeyRotationContainer = "otel-agent"

	// Two distinct, well-formed but fake API keys.
	apiKeyBeforeRotation = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	apiKeyAfterRotation  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// The forwarder's domain resolver logs this on every rotation, once per forwarder
	// wired to the config that changed. See
	// comp/forwarder/defaultforwarder/resolver.OnUpdateConfig.
	rotationLogMarker = "rotating API key for 'api_key'"
)

// apiKeyRotationTestSuite verifies that a rotated Datadog API key reaches the
// `datadog` extension's own forwarder, not just the exporter's (OTAGENT-1353).
type apiKeyRotationTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// apiKeyRotationProvisioner deploys the otel-agent as a standalone DaemonSet with its
// api_key supplied through the secrets backend, so the key can be rotated mid-test by
// updating the Kubernetes secret.
//
// Standalone is the right mode here: it has no core Agent and no configsync, so the only
// rotation writer is the otel-agent's own secrets resolver
// (cmd/otel-agent/config/agent_config.go -> pkgconfigsetup.ResolveSecrets), which writes
// at SourceSecret. The bridge subscribes to the config rather than to configsync
// specifically, so this exercises the harder of the two DDOT paths.
func apiKeyRotationProvisioner() provisioners.TypedProvisioner[environments.Kubernetes] {
	deployFn := func(e config.Env, kubeProvider *kubernetes.Provider, fi *fakeintakeComp.Fakeintake) (*agent.KubernetesAgent, error) {
		return otelstandalone.K8sAppDefinition(
			e, kubeProvider, apiKeyRotationNamespace, apiKeyRotationConfig, fi,
			otelstandalone.WithK8sSecret(apiKeyRotationSecret, map[string]string{
				"api_key": apiKeyBeforeRotation,
			}),
			otelstandalone.WithExtraEnvVars(
				&pulumicorev1.EnvVarArgs{
					Name:  pulumi.String("DD_API_KEY"),
					Value: pulumi.String("ENC[file@/etc/" + apiKeyRotationSecret + "/api_key]"),
				},
				&pulumicorev1.EnvVarArgs{
					Name:  pulumi.String("DD_SECRET_BACKEND_COMMAND"),
					Value: pulumi.String("/readsecret_multiple_providers.sh"),
				},
				// Refresh often so the test does not wait minutes for the rotation.
				&pulumicorev1.EnvVarArgs{
					Name:  pulumi.String("DD_SECRET_REFRESH_INTERVAL"),
					Value: pulumi.String("15"),
				},
			),
			otelstandalone.WithExtraVolumes(
				&pulumicorev1.VolumeArgs{
					Name: pulumi.String(apiKeyRotationSecret),
					Secret: &pulumicorev1.SecretVolumeSourceArgs{
						SecretName: pulumi.String(apiKeyRotationSecret),
					},
				},
			),
			otelstandalone.WithExtraVolumeMounts(
				&pulumicorev1.VolumeMountArgs{
					Name:      pulumi.String(apiKeyRotationSecret),
					MountPath: pulumi.String("/etc/" + apiKeyRotationSecret),
					ReadOnly:  pulumi.BoolPtr(true),
				},
			),
		)
	}

	if isKindLocal() {
		return provlocal.Provisioner(
			provlocal.WithStandaloneOTelAgent(deployFn),
		)
	}
	return provkindvm.Provisioner(
		provkindvm.WithRunOptions(
			scenkindvm.WithStandaloneOTelAgent(deployFn),
		),
	)
}

// TestOTelAgentAPIKeyRotation is the entry point for the API-key rotation suite.
func TestOTelAgentAPIKeyRotation(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &apiKeyRotationTestSuite{},
		e2e.WithProvisioner(apiKeyRotationProvisioner()),
		e2e.WithCoverageRequired(map[string]bool{
			"agent":      false,
			"otel-agent": true,
		}),
	)
}

// otelAgentPod returns the standalone otel-agent pod.
func (s *apiKeyRotationTestSuite) otelAgentPod(ctx context.Context) k8scorev1.Pod {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().
		Pods(apiKeyRotationNamespace).
		List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("app", apiKeyRotationAppLabel).String(),
		})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), res.Items, "no standalone otel-agent pod found")
	return res.Items[0]
}

// otelAgentLogs returns the otel-agent container's logs so far.
func (s *apiKeyRotationTestSuite) otelAgentLogs(ctx context.Context, podName string) string {
	req := s.Env().KubernetesCluster.Client().CoreV1().
		Pods(apiKeyRotationNamespace).
		GetLogs(podName, &k8scorev1.PodLogOptions{Container: apiKeyRotationContainer})
	stream, err := req.Stream(ctx)
	require.NoError(s.T(), err)
	defer stream.Close()
	body, err := io.ReadAll(stream)
	require.NoError(s.T(), err)
	return string(body)
}

// TestAPIKeyReachesDatadogExtension rotates the API key through the secrets backend and
// asserts that *two* forwarders rotate: the exporter's, which has always worked, and the
// `datadog` extension's own, which is the bug this change fixes.
//
// Before the fix the extension's private config was never written to after startup, so
// only one rotation line appeared and the extension kept 403ing on /api/v1/metadata.
func (s *apiKeyRotationTestSuite) TestAPIKeyReachesDatadogExtension() {
	ctx := context.Background()
	pod := s.otelAgentPod(ctx)

	// Sanity-check the starting state: the extension must actually be running, otherwise
	// a missing second rotation line would prove nothing.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs := s.otelAgentLogs(ctx, pod.Name)
		assert.Contains(c, logs, "datadog/dd-autoconfigured",
			"the datadog extension should have been autoconfigured from the Agent api_key")
	}, 3*time.Minute, 10*time.Second)

	require.Equal(s.T(), 0, strings.Count(s.otelAgentLogs(ctx, pod.Name), rotationLogMarker),
		"precondition: no rotation should have happened yet")

	s.T().Log("Rotating the API key in the Kubernetes secret")
	secrets := s.Env().KubernetesCluster.Client().CoreV1().Secrets(apiKeyRotationNamespace)
	sec, err := secrets.Get(ctx, apiKeyRotationSecret, metav1.GetOptions{})
	require.NoError(s.T(), err)
	sec.Data["api_key"] = []byte(apiKeyAfterRotation)
	_, err = secrets.Update(ctx, sec, metav1.UpdateOptions{})
	require.NoError(s.T(), err)

	// kubelet propagates an updated secret to the mounted volume on its sync period
	// (up to ~1 minute), and the Agent's secret refresh then has to tick, so allow
	// generously here.
	s.T().Log("Waiting for both forwarders to rotate")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs := s.otelAgentLogs(ctx, pod.Name)
		assert.GreaterOrEqual(c, strings.Count(logs, rotationLogMarker), 2,
			"expected both the exporter's and the datadog extension's forwarder to rotate; "+
				"exactly one occurrence means the extension is still on the stale key")
	}, 6*time.Minute, 15*time.Second)
}
