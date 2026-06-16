// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package appsecinjection contains E2E tests for the AppSec Injection Proxy
// feature in SIDECAR mode with Envoy Gateway over a Unix Domain Socket.
package appsecinjection

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	kubeYAML "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

//go:embed testdata/helm_values.yaml
var egAppSecHelmValues string

//go:embed testdata/01-gatewayclass.yaml
var egGatewayClassYAML string

//go:embed testdata/02-envoyproxy.yaml
var egEnvoyProxyYAML string

//go:embed testdata/03-gateway.yaml
var egGatewayYAML string

//go:embed testdata/04-backend-uds.yaml
var egBackendUDSYAML string

//go:embed testdata/05-envoyextensionpolicy.yaml
var egExtProcPolicyYAML string

//go:embed testdata/06-httproute.yaml
var egHTTPRouteYAML string

//go:embed testdata/07-sample-backend.yaml
var egSampleBackendYAML string

const (
	egNamespace      = "envoy-gateway-system"
	demoNamespace    = "eg-appsec-demo"
	sidecarContainer = "datadog-appsec"
	sidecarVolume    = "datadog-appsec-uds"
	attackUserAgent  = "dd-test-scanner-log-block"
	gatewayName      = "appsec-gateway"
	egHelmVersion    = "v1.7.1"
)

type egAppSecSidecarSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestEnvoyGatewaySidecarUDS is the entry point for the suite.
// It provisions a Kubernetes cluster with:
//   - Datadog DaemonSet + cluster-agent with AppSec injector in sidecar mode
//   - Envoy Gateway v1.7.1 (OCI Helm chart)
//   - GatewayClass / EnvoyProxy (with Datadog sidecar patch) / Gateway / Backend / ExtProc / HTTPRoute
func TestEnvoyGatewaySidecarUDS(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &egAppSecSidecarSuite{}, e2e.WithProvisioner(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(egAppSecHelmValues),
		},
		WorkloadFunc: deployEnvoyGatewayWorkloads,
	})))
}

// deployEnvoyGatewayWorkloads is the AgentDependentWorkloadAppFunc that:
//  1. Installs Envoy Gateway via the OCI Helm chart with Backend extension APIs enabled
//  2. Applies all EG AppSec UDS manifests in order
//
// It is intentionally kept as a simple, sequential Pulumi program.
func deployEnvoyGatewayWorkloads(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*kubeComp.Workload, error) {
	workload := &kubeComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "eg-appsec-sidecar-uds", workload, dependsOnAgent); err != nil {
		return nil, err
	}

	providerOpt := pulumi.Provider(kubeProvider)
	parentOpt := pulumi.Parent(workload)

	// 1. Install Envoy Gateway via its OCI Helm chart.
	//    extensionApis.enableBackend is passed as a Helm value so the Backend
	//    CRD is activated before we apply 04-backend-uds.yaml.
	egRelease, err := helmv3.NewRelease(e.Ctx(), "envoy-gateway", &helmv3.ReleaseArgs{
		Chart:           pulumi.String("oci://docker.io/envoyproxy/gateway-helm"),
		Version:         pulumi.StringPtr(egHelmVersion),
		Namespace:       pulumi.String(egNamespace),
		CreateNamespace: pulumi.Bool(true),
		Timeout:         pulumi.IntPtr(300),
		Values: pulumi.Map{
			"config": pulumi.Map{
				"envoyGateway": pulumi.Map{
					"extensionApis": pulumi.Map{
						"enableBackend": pulumi.Bool(true),
					},
				},
			},
		},
	}, providerOpt, parentOpt)
	if err != nil {
		return nil, fmt.Errorf("installing envoy-gateway helm chart: %w", err)
	}

	dependsOnEG := pulumi.DependsOn([]pulumi.Resource{egRelease})

	// 2. Apply the AppSec UDS manifests in dependency order.
	//    Each ConfigGroup is a separate Pulumi resource so errors are localized.
	gatewayClassGroup, err := kubeYAML.NewConfigGroup(e.Ctx(), "eg-gatewayclass", &kubeYAML.ConfigGroupArgs{
		YAML: []string{egGatewayClassYAML},
	}, providerOpt, parentOpt, dependsOnEG)
	if err != nil {
		return nil, fmt.Errorf("applying gatewayclass: %w", err)
	}

	dependsOnGC := pulumi.DependsOn([]pulumi.Resource{gatewayClassGroup})

	envoyProxyGroup, err := kubeYAML.NewConfigGroup(e.Ctx(), "eg-envoyproxy", &kubeYAML.ConfigGroupArgs{
		YAML: []string{egEnvoyProxyYAML},
	}, providerOpt, parentOpt, dependsOnGC)
	if err != nil {
		return nil, fmt.Errorf("applying envoyproxy: %w", err)
	}

	gatewayGroup, err := kubeYAML.NewConfigGroup(e.Ctx(), "eg-gateway", &kubeYAML.ConfigGroupArgs{
		YAML: []string{egGatewayYAML},
	}, providerOpt, parentOpt, pulumi.DependsOn([]pulumi.Resource{envoyProxyGroup}))
	if err != nil {
		return nil, fmt.Errorf("applying gateway: %w", err)
	}

	backendGroup, err := kubeYAML.NewConfigGroup(e.Ctx(), "eg-backend-uds", &kubeYAML.ConfigGroupArgs{
		YAML: []string{egBackendUDSYAML},
	}, providerOpt, parentOpt, pulumi.DependsOn([]pulumi.Resource{gatewayGroup}))
	if err != nil {
		return nil, fmt.Errorf("applying backend-uds: %w", err)
	}

	extProcGroup, err := kubeYAML.NewConfigGroup(e.Ctx(), "eg-extproc-policy", &kubeYAML.ConfigGroupArgs{
		YAML: []string{egExtProcPolicyYAML},
	}, providerOpt, parentOpt, pulumi.DependsOn([]pulumi.Resource{backendGroup}))
	if err != nil {
		return nil, fmt.Errorf("applying envoyextensionpolicy: %w", err)
	}

	_, err = kubeYAML.NewConfigGroup(e.Ctx(), "eg-httproute-and-backend", &kubeYAML.ConfigGroupArgs{
		YAML: []string{egHTTPRouteYAML, egSampleBackendYAML},
	}, providerOpt, parentOpt, pulumi.DependsOn([]pulumi.Resource{extProcGroup}))
	if err != nil {
		return nil, fmt.Errorf("applying httproute and sample-backend: %w", err)
	}

	return workload, nil
}

// TestSidecarPodInjected verifies that the Envoy Gateway data-plane pod
// contains the Datadog AppSec sidecar container and the shared UDS emptyDir volume.
func (s *egAppSecSidecarSuite) TestSidecarPodInjected() {
	k8s := s.Env().KubernetesCluster.Client()

	var dataPlanePod *corev1.Pod

	s.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods(egNamespace).List(
			context.Background(),
			metav1.ListOptions{
				LabelSelector: fmt.Sprintf("gateway.envoyproxy.io/owning-gateway-name=%s", gatewayName),
			},
		)
		require.NoError(c, err, "listing EG data-plane pods")
		require.NotEmpty(c, pods.Items, "expected at least one EG data-plane pod")

		for i := range pods.Items {
			if pods.Items[i].Status.Phase == corev1.PodRunning {
				dataPlanePod = &pods.Items[i]
				return
			}
		}
		require.Fail(c, "no running EG data-plane pod found yet")
	}, 5*time.Minute, 15*time.Second)

	require.NotNil(s.T(), dataPlanePod)

	containerNames := make([]string, 0, len(dataPlanePod.Spec.Containers))
	for _, c := range dataPlanePod.Spec.Containers {
		containerNames = append(containerNames, c.Name)
	}
	require.Contains(s.T(), containerNames, sidecarContainer,
		"EG data-plane pod must contain the %q sidecar; found containers: %v", sidecarContainer, containerNames)

	volumeNames := make([]string, 0, len(dataPlanePod.Spec.Volumes))
	for _, v := range dataPlanePod.Spec.Volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	require.Contains(s.T(), volumeNames, sidecarVolume,
		"EG data-plane pod must have volume %q; found volumes: %v", sidecarVolume, volumeNames)
}

// TestTrafficBlocking sends a benign HTTP request and an attack request
// through the Envoy Gateway.  The benign request must return 200; the attack
// request must return 403 — proving that the Datadog ext_proc sidecar is
// actively processing traffic over the Unix Domain Socket.
func (s *egAppSecSidecarSuite) TestTrafficBlocking() {
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	// Resolve the Gateway service ClusterIP.  The service is labeled with the
	// owning gateway name in envoy-gateway-system.
	var gatewaySvcIP string
	s.EventuallyWithT(func(c *assert.CollectT) {
		svcs, err := k8s.CoreV1().Services(egNamespace).List(
			context.Background(),
			metav1.ListOptions{
				LabelSelector: fmt.Sprintf("gateway.envoyproxy.io/owning-gateway-name=%s", gatewayName),
			},
		)
		require.NoError(c, err, "listing Gateway services")
		require.NotEmpty(c, svcs.Items, "expected Gateway service to exist")
		ip := svcs.Items[0].Spec.ClusterIP
		require.NotEmpty(c, ip, "Gateway service ClusterIP must be set")
		gatewaySvcIP = ip
	}, 5*time.Minute, 15*time.Second)

	var backendPodName string
	s.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods(demoNamespace).List(
			context.Background(),
			metav1.ListOptions{LabelSelector: "app=sample-backend"},
		)
		require.NoError(c, err, "listing sample-backend pods")
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodRunning {
				backendPodName = p.Name
				return
			}
		}
		require.Fail(c, "no running sample-backend pod found yet")
	}, 5*time.Minute, 15*time.Second)

	gatewayURL := fmt.Sprintf("http://%s:80/", gatewaySvcIP)

	s.EventuallyWithT(func(c *assert.CollectT) {
		stdout, _, err := kc.PodExec(
			demoNamespace, backendPodName, "backend",
			[]string{"curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "10", gatewayURL},
		)
		require.NoError(c, err, "curl benign request exec")
		require.Equal(c, "200", strings.TrimSpace(stdout),
			"benign request through EG must return 200, got %q", strings.TrimSpace(stdout))
	}, 3*time.Minute, 10*time.Second)

	s.EventuallyWithT(func(c *assert.CollectT) {
		stdout, _, err := kc.PodExec(
			demoNamespace, backendPodName, "backend",
			[]string{
				"curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "10",
				"-H", fmt.Sprintf("User-Agent: %s", attackUserAgent),
				gatewayURL + "?x=1",
			},
		)
		require.NoError(c, err, "curl attack request exec")
		require.Equal(c, "403", strings.TrimSpace(stdout),
			"attack request (User-Agent: %s) must be blocked with 403; got %q",
			attackUserAgent, strings.TrimSpace(stdout))
	}, 3*time.Minute, 10*time.Second)
}

// TestReconcileDoesNotStripSidecar verifies the reconcile-loop guard:
// a no-op annotation update on the Gateway must NOT cause Envoy Gateway to
// re-roll the data-plane pod or remove the Datadog sidecar.
func (s *egAppSecSidecarSuite) TestReconcileDoesNotStripSidecar() {
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	// Record the current data-plane pod name.
	var podNameBefore string
	s.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods(egNamespace).List(
			context.Background(),
			metav1.ListOptions{
				LabelSelector: fmt.Sprintf("gateway.envoyproxy.io/owning-gateway-name=%s", gatewayName),
			},
		)
		require.NoError(c, err, "listing EG data-plane pods before reconcile")
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodRunning {
				podNameBefore = p.Name
				return
			}
		}
		require.Fail(c, "no running EG data-plane pod found")
	}, 3*time.Minute, 10*time.Second)

	require.NotEmpty(s.T(), podNameBefore)

	// Trigger a no-op reconcile by annotating the Gateway resource.
	dynClient, err := dynamic.NewForConfig(kc.K8sConfig)
	require.NoError(s.T(), err, "creating dynamic client")

	gatewayGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gateways",
	}
	patchData, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]string{
				"appsec.datadoghq.com/noop-reconcile": fmt.Sprintf("%d", time.Now().Unix()),
			},
		},
	})
	require.NoError(s.T(), err)

	_, err = dynClient.Resource(gatewayGVR).Namespace(demoNamespace).Patch(
		context.Background(),
		gatewayName,
		types.MergePatchType,
		patchData,
		metav1.PatchOptions{},
	)
	require.NoError(s.T(), err, "annotating Gateway for no-op reconcile")

	// Wait one reconcile interval — 30 s is conservative.
	time.Sleep(30 * time.Second)

	// Assert pod name is unchanged.
	pods, err := k8s.CoreV1().Pods(egNamespace).List(
		context.Background(),
		metav1.ListOptions{
			LabelSelector: fmt.Sprintf("gateway.envoyproxy.io/owning-gateway-name=%s", gatewayName),
		},
	)
	require.NoError(s.T(), err, "listing EG data-plane pods after reconcile")
	require.NotEmpty(s.T(), pods.Items, "expected EG data-plane pod to still exist")

	var podNameAfter string
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			podNameAfter = p.Name
			break
		}
	}
	require.Equal(s.T(), podNameBefore, podNameAfter,
		"EG data-plane pod must NOT be re-rolled after a no-op reconcile: before=%q after=%q",
		podNameBefore, podNameAfter)

	// Re-verify the sidecar is still present after the reconcile.
	var finalPod *corev1.Pod
	for i := range pods.Items {
		if pods.Items[i].Name == podNameAfter {
			finalPod = &pods.Items[i]
			break
		}
	}
	require.NotNil(s.T(), finalPod, "could not find pod %q after reconcile", podNameAfter)

	containerNames := make([]string, 0, len(finalPod.Spec.Containers))
	for _, c := range finalPod.Spec.Containers {
		containerNames = append(containerNames, c.Name)
	}
	require.Contains(s.T(), containerNames, sidecarContainer,
		"sidecar %q must still be present after no-op reconcile; found: %v", sidecarContainer, containerNames)

	volumeNames := make([]string, 0, len(finalPod.Spec.Volumes))
	for _, v := range finalPod.Spec.Volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	require.Contains(s.T(), volumeNames, sidecarVolume,
		"volume %q must still be present after no-op reconcile; found: %v", sidecarVolume, volumeNames)
}
