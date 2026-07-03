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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

//go:embed testdata/03-gateway.yaml
var egGatewayYAML string

//go:embed testdata/06-httproute.yaml
var egHTTPRouteYAML string

//go:embed testdata/07-sample-backend.yaml
var egSampleBackendYAML string

const (
	egNamespace      = "envoy-gateway-system"
	demoNamespace    = "eg-appsec-demo"
	sidecarContainer = "datadog-appsec"
	sidecarVolume    = "datadog-appsec-uds"
	envoyContainer   = "envoy"
	udsSocketDir     = "/var/run/datadog"
	extProcName      = "datadog-appsec-extproc"
	attackUserAgent  = "dd-test-scanner-log-block"
	gatewayName      = "appsec-gateway"
	egHelmVersion    = "v1.7.1"
	egFSGroup        = int64(65532)
)

var (
	backendGVR = schema.GroupVersionResource{
		Group: "gateway.envoyproxy.io", Version: "v1alpha1", Resource: "backends",
	}
	extProcGVR = schema.GroupVersionResource{
		Group: "gateway.envoyproxy.io", Version: "v1alpha1", Resource: "envoyextensionpolicies",
	}
	referenceGrantGVR = schema.GroupVersionResource{
		Group: "gateway.networking.k8s.io", Version: "v1beta1", Resource: "referencegrants",
	}
	gatewayGVR = schema.GroupVersionResource{
		Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways",
	}
)

type egAppSecSidecarSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestEnvoyGatewaySidecarUDS exercises the cluster-agent AppSec injector in
// SIDECAR mode with Envoy Gateway over a Unix Domain Socket.
//
// Manifests applied by the test (Gateway user's responsibility):
//   - GatewayClass (no parametersRef — no manual sidecar patch)
//   - Gateway
//   - HTTPRoute + sample backend
//
// Resources created / injected by the cluster-agent (what this suite asserts):
//   - Backend `datadog-appsec-extproc` (controller, in Gateway namespace)
//   - EnvoyExtensionPolicy `datadog-appsec-extproc` (controller, in Gateway namespace)
//   - `datadog-appsec` sidecar container + `datadog-appsec-uds` volume in the
//     EG data-plane pod (webhook, in envoy-gateway-system)
func TestEnvoyGatewaySidecarUDS(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &egAppSecSidecarSuite{}, e2e.WithProvisioner(Provisioner(ProvisionerOptions{
		AgentOptions: []kubernetesagentparams.Option{
			kubernetesagentparams.WithHelmValues(egAppSecHelmValues),
		},
		WorkloadFunc: deployEnvoyGatewayWorkloads,
	})))
}

// deployEnvoyGatewayWorkloads installs Envoy Gateway and applies only the
// user-side manifests (GatewayClass, Gateway, HTTPRoute, sample backend).
// The cluster-agent does the rest: it creates the Backend + EnvoyExtensionPolicy
// and its webhook injects the sidecar into EG data-plane pods.
func deployEnvoyGatewayWorkloads(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*kubeComp.Workload, error) {
	workload := &kubeComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "eg-appsec-sidecar-uds", workload, dependsOnAgent); err != nil {
		return nil, err
	}

	providerOpt := pulumi.Provider(kubeProvider)
	parentOpt := pulumi.Parent(workload)

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

	gcGroup, err := kubeYAML.NewConfigGroup(e.Ctx(), "eg-gatewayclass", &kubeYAML.ConfigGroupArgs{
		YAML: []string{egGatewayClassYAML},
	}, providerOpt, parentOpt, dependsOnEG)
	if err != nil {
		return nil, fmt.Errorf("applying GatewayClass: %w", err)
	}

	gwGroup, err := kubeYAML.NewConfigGroup(e.Ctx(), "eg-gateway", &kubeYAML.ConfigGroupArgs{
		YAML: []string{egGatewayYAML},
	}, providerOpt, parentOpt, pulumi.DependsOn([]pulumi.Resource{gcGroup}))
	if err != nil {
		return nil, fmt.Errorf("applying Gateway: %w", err)
	}

	_, err = kubeYAML.NewConfigGroup(e.Ctx(), "eg-workload", &kubeYAML.ConfigGroupArgs{
		YAML: []string{egHTTPRouteYAML, egSampleBackendYAML},
	}, providerOpt, parentOpt, pulumi.DependsOn([]pulumi.Resource{gwGroup}))
	if err != nil {
		return nil, fmt.Errorf("applying HTTPRoute and sample backend: %w", err)
	}

	return workload, nil
}

// TestWebhookInjectsSidecar verifies that the cluster-agent admission webhook
// injected the Datadog AppSec sidecar into the EG data-plane pod.
//
// Assertions (all produced by the cluster-agent webhook, NOT by any manual manifest):
//   - pod has container named "datadog-appsec"
//   - pod has volume named "datadog-appsec-uds"
//   - "envoy" container has a VolumeMount for "datadog-appsec-uds" at /var/run/datadog
//   - pod.spec.securityContext.fsGroup == 65532
func (s *egAppSecSidecarSuite) TestWebhookInjectsSidecar() {
	k8s := s.Env().KubernetesCluster.Client()

	var pod *corev1.Pod
	s.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods(egNamespace).List(
			context.Background(),
			metav1.ListOptions{
				LabelSelector: "gateway.envoyproxy.io/owning-gateway-name=" + gatewayName,
			},
		)
		require.NoError(c, err, "listing EG data-plane pods")
		for i := range pods.Items {
			if pods.Items[i].Status.Phase == corev1.PodRunning {
				pod = &pods.Items[i]
				return
			}
		}
		require.Fail(c, "no running EG data-plane pod yet")
	}, 5*time.Minute, 15*time.Second)

	require.NotNil(s.T(), pod)

	containerNames := make([]string, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		containerNames = append(containerNames, c.Name)
	}
	require.Contains(s.T(), containerNames, sidecarContainer,
		"cluster-agent webhook must inject %q container; got %v", sidecarContainer, containerNames)

	volumeNames := make([]string, 0, len(pod.Spec.Volumes))
	for _, v := range pod.Spec.Volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	require.Contains(s.T(), volumeNames, sidecarVolume,
		"cluster-agent webhook must add %q volume; got %v", sidecarVolume, volumeNames)

	var envoyMountFound bool
	for _, c := range pod.Spec.Containers {
		if c.Name != envoyContainer {
			continue
		}
		for _, vm := range c.VolumeMounts {
			if vm.Name == sidecarVolume && vm.MountPath == udsSocketDir {
				envoyMountFound = true
			}
		}
	}
	require.True(s.T(), envoyMountFound,
		"cluster-agent webhook must mount %q at %q in the envoy container", sidecarVolume, udsSocketDir)

	require.NotNil(s.T(), pod.Spec.SecurityContext,
		"cluster-agent webhook must set pod security context")
	require.NotNil(s.T(), pod.Spec.SecurityContext.FSGroup,
		"cluster-agent webhook must set fsGroup")
	require.Equal(s.T(), egFSGroup, *pod.Spec.SecurityContext.FSGroup,
		"cluster-agent webhook must set fsGroup=%d for shared emptyDir", egFSGroup)
}

// TestControllerCreatesExtProcResources verifies that the cluster-agent controller
// created the Backend and EnvoyExtensionPolicy in the Gateway namespace, and did
// NOT create a ReferenceGrant (sidecar mode does not need cross-namespace access).
func (s *egAppSecSidecarSuite) TestControllerCreatesExtProcResources() {
	kc := s.Env().KubernetesCluster.KubernetesClient
	dynClient, err := dynamic.NewForConfig(kc.K8sConfig)
	require.NoError(s.T(), err, "creating dynamic client")

	var backendObj *unstructured.Unstructured
	s.EventuallyWithT(func(c *assert.CollectT) {
		obj, err := dynClient.Resource(backendGVR).Namespace(demoNamespace).Get(
			context.Background(), extProcName, metav1.GetOptions{},
		)
		require.NoError(c, err, "cluster-agent controller must create Backend %q in %s", extProcName, demoNamespace)
		backendObj = obj
	}, 5*time.Minute, 15*time.Second)

	require.NotNil(s.T(), backendObj)

	endpoints, found, err := unstructured.NestedSlice(backendObj.Object, "spec", "endpoints")
	require.NoError(s.T(), err)
	require.True(s.T(), found && len(endpoints) > 0, "Backend must have at least one endpoint")
	ep0, ok := endpoints[0].(map[string]interface{})
	require.True(s.T(), ok, "endpoint must be a map")
	unixPath, _, _ := unstructured.NestedString(ep0, "unix", "path")
	require.NotEmpty(s.T(), unixPath, "Backend endpoint must have a unix socket path")

	s.EventuallyWithT(func(c *assert.CollectT) {
		_, err := dynClient.Resource(extProcGVR).Namespace(demoNamespace).Get(
			context.Background(), extProcName, metav1.GetOptions{},
		)
		require.NoError(c, err, "cluster-agent controller must create EnvoyExtensionPolicy %q in %s", extProcName, demoNamespace)
	}, 3*time.Minute, 10*time.Second)

	_, err = dynClient.Resource(referenceGrantGVR).Namespace(demoNamespace).Get(
		context.Background(), extProcName, metav1.GetOptions{},
	)
	require.True(s.T(), errors.IsNotFound(err),
		"sidecar mode must NOT create a ReferenceGrant; found one in %s", demoNamespace)
}

// TestTrafficBlocking sends a benign request and an attack request through the
// Envoy Gateway.  HTTP 403 on the attack proves the injected sidecar processes
// traffic over the UDS.
//
// fakeintake WAF telemetry: not asserted here.  The injected sidecar env has
// DD_APM_TRACING_ENABLED=false (configured via the cluster-agent injector with
// the sidecar image's default), so AppSec events do not flow through APM traces
// to fakeintake in this configuration.  The HTTP 403 + cluster-agent injection
// assertions (TestWebhookInjectsSidecar, TestControllerCreatesExtProcResources)
// are the authoritative proof.  Adding fakeintake WAF telemetry is tracked as
// a follow-up once the sidecar→node-agent→fakeintake path is wired in CI.
func (s *egAppSecSidecarSuite) TestTrafficBlocking() {
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	var gatewaySvcIP string
	s.EventuallyWithT(func(c *assert.CollectT) {
		svcs, err := k8s.CoreV1().Services(egNamespace).List(
			context.Background(),
			metav1.ListOptions{
				LabelSelector: "gateway.envoyproxy.io/owning-gateway-name=" + gatewayName,
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
		require.Fail(c, "no running sample-backend pod yet")
	}, 5*time.Minute, 15*time.Second)

	gatewayURL := fmt.Sprintf("http://%s:80/", gatewaySvcIP)

	s.EventuallyWithT(func(c *assert.CollectT) {
		stdout, _, err := kc.PodExec(
			demoNamespace, backendPodName, "backend",
			[]string{"curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "10", gatewayURL},
		)
		require.NoError(c, err, "curl benign request")
		require.Equal(c, "200", strings.TrimSpace(stdout),
			"benign request must return 200, got %q", strings.TrimSpace(stdout))
	}, 3*time.Minute, 10*time.Second)

	s.EventuallyWithT(func(c *assert.CollectT) {
		stdout, _, err := kc.PodExec(
			demoNamespace, backendPodName, "backend",
			[]string{
				"curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "10",
				"-H", "User-Agent: " + attackUserAgent,
				gatewayURL + "?x=1",
			},
		)
		require.NoError(c, err, "curl attack request")
		require.Equal(c, "403", strings.TrimSpace(stdout),
			"attack (User-Agent: %s) must be blocked with 403; got %q", attackUserAgent, strings.TrimSpace(stdout))
	}, 3*time.Minute, 10*time.Second)
}

// TestReconcileDoesNotStripSidecar verifies the reconcile-loop guard: a no-op
// Gateway annotation must not cause EG to re-roll the data-plane pod or strip
// the injected sidecar and volume.
func (s *egAppSecSidecarSuite) TestReconcileDoesNotStripSidecar() {
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	var podNameBefore string
	s.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods(egNamespace).List(
			context.Background(),
			metav1.ListOptions{
				LabelSelector: "gateway.envoyproxy.io/owning-gateway-name=" + gatewayName,
			},
		)
		require.NoError(c, err)
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodRunning {
				podNameBefore = p.Name
				return
			}
		}
		require.Fail(c, "no running EG data-plane pod")
	}, 3*time.Minute, 10*time.Second)

	require.NotEmpty(s.T(), podNameBefore)

	dynClient, err := dynamic.NewForConfig(kc.K8sConfig)
	require.NoError(s.T(), err)

	patchData, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]string{
				"appsec.datadoghq.com/noop-reconcile": strconv.FormatInt(time.Now().Unix(), 10),
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
	require.NoError(s.T(), err, "patching Gateway with no-op annotation")

	// Instead of a fixed wait, poll over a bounded window and assert a re-roll is never observed:
	// a running data-plane pod whose name differs from podNameBefore means the no-op reconcile
	// re-rolled it. This fails fast on a transient re-roll rather than racing a single snapshot.
	require.Never(s.T(), func() bool {
		pods, err := k8s.CoreV1().Pods(egNamespace).List(
			context.Background(),
			metav1.ListOptions{
				LabelSelector: "gateway.envoyproxy.io/owning-gateway-name=" + gatewayName,
			},
		)
		if err != nil {
			return false
		}
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodRunning && p.Name != podNameBefore {
				return true
			}
		}
		return false
	}, 30*time.Second, 5*time.Second, "EG data-plane pod must not be re-rolled by a no-op reconcile")

	pods, err := k8s.CoreV1().Pods(egNamespace).List(
		context.Background(),
		metav1.ListOptions{
			LabelSelector: "gateway.envoyproxy.io/owning-gateway-name=" + gatewayName,
		},
	)
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), pods.Items)

	var podNameAfter string
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			podNameAfter = p.Name
			break
		}
	}
	require.Equal(s.T(), podNameBefore, podNameAfter,
		"EG data-plane pod must NOT be re-rolled after no-op reconcile")

	var finalPod *corev1.Pod
	for i := range pods.Items {
		if pods.Items[i].Name == podNameAfter {
			finalPod = &pods.Items[i]
			break
		}
	}
	require.NotNil(s.T(), finalPod)

	containerNames := make([]string, 0, len(finalPod.Spec.Containers))
	for _, c := range finalPod.Spec.Containers {
		containerNames = append(containerNames, c.Name)
	}
	require.Contains(s.T(), containerNames, sidecarContainer,
		"injected sidecar must survive no-op reconcile; found %v", containerNames)

	volumeNames := make([]string, 0, len(finalPod.Spec.Volumes))
	for _, v := range finalPod.Spec.Volumes {
		volumeNames = append(volumeNames, v.Name)
	}
	require.Contains(s.T(), volumeNames, sidecarVolume,
		"injected volume must survive no-op reconcile; found %v", volumeNames)
}
