// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1pulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8scorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
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
	apiKeyRotationContainer = "otel-agent"

	// apiKeyRotationSecret holds the live API key as a mounted file. Rotating the test
	// means writing a new value here: kubelet republishes the file into the running
	// pods, and the core Agent's secret refresh picks it up without a restart.
	apiKeyRotationSecret = "otel-apikey-rotation"
	apiKeyRotationKey    = "api_key"
	apiKeyRotationMount  = "/etc/otel-apikey-rotation"

	// apiKeyHandleSecret is what the chart wires into DD_API_KEY. Its value is the
	// constant ENC[] handle, never a key, so the env var never has to change -- only
	// the file it points at does.
	apiKeyHandleSecret = "otel-apikey-handle"
	apiKeyHandleKey    = "api-key"

	// Two distinct, well-formed but fake API keys.
	apiKeyBeforeRotation = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	apiKeyAfterRotation  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// The core Agent serves its config API on this port and the otel-agent's configsync
	// polls it. Both default to disabled (agent_ipc.config_refresh_interval is 0), so
	// the test has to turn them on explicitly.
	apiKeyRotationIPCPort     = 5009
	apiKeyRotationSyncSeconds = 15

	// The forwarder's domain resolver logs this on every rotation, once per forwarder
	// wired to the config that changed. See
	// comp/forwarder/defaultforwarder/resolver.OnUpdateConfig.
	rotationLogMarker = "rotating API key for 'api_key'"

	// The converter names the extension it autoconfigures from the Agent's api_key.
	ddExtensionInstance = "datadog/dd-autoconfigured"
)

// apiKeyRotationTestSuite verifies that a rotated Datadog API key reaches the
// `datadog` extension's own forwarder, not just the exporter's (OTAGENT-1353).
type apiKeyRotationTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// apiKeyRotationHelmValues wires the rotation mechanics into the Helm release.
//
// The core Agent resolves ENC[file@...] through its secrets backend and re-resolves it
// every DD_SECRET_REFRESH_INTERVAL seconds; `api_key` is on the secrets allowlist
// (comp/core/secrets/impl.allowListPaths) so a change there is actually published. The
// otel-agent does not resolve secrets in connected mode by design -- it receives the
// already-resolved key over configsync (cmd/otel-agent/config/agent_config.go) -- which
// is precisely the rotation path this test exercises.
var apiKeyRotationHelmValues = fmt.Sprintf(`
datadog:
  # Point DD_API_KEY at the constant ENC[] handle instead of the framework's
  # plaintext credentials secret. This overrides the default values layer.
  apiKeyExistingSecret: %[1]s
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
  otelCollector:
    useStandaloneImage: false
agents:
  volumes:
    - name: %[2]s
      secret:
        secretName: %[2]s
  volumeMounts:
    - name: %[2]s
      mountPath: %[3]s
      readOnly: true
  containers:
    agent:
      env:
        - name: DD_SECRET_BACKEND_COMMAND
          value: /readsecret_multiple_providers.sh
        # Refresh often so the test does not wait minutes for the rotation.
        - name: DD_SECRET_REFRESH_INTERVAL
          value: "%[4]d"
        # Serve the config API that the otel-agent's configsync polls.
        - name: DD_AGENT_IPC_PORT
          value: "%[5]d"
    otelAgent:
      env:
        - name: DD_AGENT_IPC_PORT
          value: "%[5]d"
        - name: DD_AGENT_IPC_CONFIG_REFRESH_INTERVAL
          value: "%[4]d"
`, apiKeyHandleSecret, apiKeyRotationSecret, apiKeyRotationMount, apiKeyRotationSyncSeconds, apiKeyRotationIPCPort)

// apiKeyRotationSecrets creates the namespace and the two secrets the Helm release
// depends on, before the Agent is deployed.
func apiKeyRotationSecrets(e config.Env, kubeProvider *kubernetes.Provider) (*kubecomp.Workload, error) {
	workload := &kubecomp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", apiKeyRotationSecret, workload); err != nil {
		return nil, err
	}

	opts := []pulumi.ResourceOption{
		pulumi.Provider(kubeProvider),
		pulumi.Parent(workload),
		pulumi.DeletedWith(kubeProvider),
	}

	// The Helm component reconciles this namespace with a patch rather than creating
	// it, so creating it here first is safe.
	ns, err := corev1.NewNamespace(e.Ctx(), apiKeyRotationNamespace, &corev1.NamespaceArgs{
		Metadata: metav1pulumi.ObjectMetaArgs{Name: pulumi.String(apiKeyRotationNamespace)},
	}, opts...)
	if err != nil {
		return nil, err
	}
	nsOpts := append(opts, utils.PulumiDependsOn(ns))

	// The rotating value, mounted as a file.
	if _, err := corev1.NewSecret(e.Ctx(), apiKeyRotationSecret, &corev1.SecretArgs{
		Metadata: metav1pulumi.ObjectMetaArgs{
			Name:      pulumi.String(apiKeyRotationSecret),
			Namespace: pulumi.String(apiKeyRotationNamespace),
		},
		StringData: pulumi.StringMap{apiKeyRotationKey: pulumi.String(apiKeyBeforeRotation)},
	}, nsOpts...); err != nil {
		return nil, err
	}

	// The constant handle, consumed by the chart as DD_API_KEY.
	if _, err := corev1.NewSecret(e.Ctx(), apiKeyHandleSecret, &corev1.SecretArgs{
		Metadata: metav1pulumi.ObjectMetaArgs{
			Name:      pulumi.String(apiKeyHandleSecret),
			Namespace: pulumi.String(apiKeyRotationNamespace),
		},
		StringData: pulumi.StringMap{
			apiKeyHandleKey: pulumi.String(
				fmt.Sprintf("ENC[file@%s/%s]", apiKeyRotationMount, apiKeyRotationKey)),
		},
	}, nsOpts...); err != nil {
		return nil, err
	}
	return workload, nil
}

// apiKeyRotationSecretsHook adapts apiKeyRotationSecrets to the local provisioner's
// error-only pre-agent hook.
func apiKeyRotationSecretsHook(e config.Env, kubeProvider *kubernetes.Provider) error {
	_, err := apiKeyRotationSecrets(e, kubeProvider)
	return err
}

// apiKeyRotationProvisioner deploys DDOT in connected mode: a core Agent plus the
// otel-agent container, with the Agent's api_key supplied through the secrets backend.
//
// Connected mode is mandatory, not a preference. The converter refuses to autoconfigure
// the `datadog` extension when otel_standalone is set -- it and opamp are mutually
// exclusive (comp/otelcol/converter/impl/autoconfigure.go) -- so standalone has no
// extension to rotate at all.
func apiKeyRotationProvisioner() provisioners.TypedProvisioner[environments.Kubernetes] {
	agentOpts := []kubernetesagentparams.Option{
		kubernetesagentparams.WithHelmValues(apiKeyRotationHelmValues),
		kubernetesagentparams.WithOTelAgent(),
		kubernetesagentparams.WithOTelConfig(apiKeyRotationConfig),
	}

	if isKindLocal() {
		return provlocal.Provisioner(
			provlocal.WithPreAgentHook(apiKeyRotationSecretsHook),
			provlocal.WithAgentOptions(agentOpts...),
		)
	}
	return provkindvm.Provisioner(
		provkindvm.WithRunOptions(
			scenkindvm.WithPreAgentWorkloadApp(apiKeyRotationSecrets),
			scenkindvm.WithAgentOptions(agentOpts...),
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

// otelAgentPod returns the Agent pod carrying the otel-agent container.
func (s *apiKeyRotationTestSuite) otelAgentPod(ctx context.Context) k8scorev1.Pod {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().
		Pods(apiKeyRotationNamespace).
		List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector(
				"app", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
		})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), res.Items, "no agent pod found")
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
		assert.Contains(c, logs, ddExtensionInstance,
			"the datadog extension should have been autoconfigured from the Agent api_key")
	}, 3*time.Minute, 10*time.Second)

	require.Equal(s.T(), 0, strings.Count(s.otelAgentLogs(ctx, pod.Name), rotationLogMarker),
		"precondition: no rotation should have happened yet")

	s.T().Log("Rotating the API key in the Kubernetes secret")
	secrets := s.Env().KubernetesCluster.Client().CoreV1().Secrets(apiKeyRotationNamespace)
	sec, err := secrets.Get(ctx, apiKeyRotationSecret, metav1.GetOptions{})
	require.NoError(s.T(), err)
	sec.Data[apiKeyRotationKey] = []byte(apiKeyAfterRotation)
	_, err = secrets.Update(ctx, sec, metav1.UpdateOptions{})
	require.NoError(s.T(), err)

	// Three hops have to tick before the otel-agent sees anything: kubelet republishes
	// the mounted secret (up to ~1 minute), the core Agent's secret refresh re-resolves
	// the ENC[] handle, and the otel-agent's configsync then polls the core Agent. Allow
	// generously.
	s.T().Log("Waiting for both forwarders to rotate")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs := s.otelAgentLogs(ctx, pod.Name)
		assert.GreaterOrEqual(c, strings.Count(logs, rotationLogMarker), 2,
			"expected both the exporter's and the datadog extension's forwarder to rotate; "+
				"exactly one occurrence means the extension is still on the stale key")
	}, 6*time.Minute, 15*time.Second)
}
