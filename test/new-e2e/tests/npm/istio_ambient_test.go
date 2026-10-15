// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package npm

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/cilium"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes/istio"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	proveks "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/eks"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

// istioAmbientHelmValues turns on netns container attribution in system-probe
const istioAmbientHelmValues = `
agents:
  containers:
    systemProbe:
      env:
        - name: DD_NETWORK_CONFIG_ENABLE_NETNS_CONTAINER_ATTRIBUTION
          value: "true"
`

// istioAmbientSuite checks that sockets ztunnel opens inside an enrolled pod's network namespace
// are attributed to that pod's container, on any cluster running Istio ambient.
type istioAmbientSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestEKSIstioAmbientSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &istioAmbientSuite{},
		e2e.WithStackName("stack-eks-istio-ambient"),
		e2e.WithProvisioner(proveks.Provisioner(proveks.WithRunOptions(
			eks.WithEKSOptions(eks.WithLinuxNodeGroup()),
			eks.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(systemProbeConfigNPMHelmValues),
				kubernetesagentparams.WithHelmValues(istioAmbientHelmValues),
				kubernetesagentparams.WithTimeout(600),
			),
			eks.WithAgentDependentWorkloadApp(istioAmbientWorkload),
		))),
	)
}

func TestKindCiliumIstioAmbientSuite(t *testing.T) {
	t.Parallel()
	// Istio ambient needs Cilium to leave the CNI config to istio-cni too, and to keep socket
	// load balancing out of pod namespaces so ztunnel sees the service address
	ciliumHelmValues := cilium.HelmValues{
		"kubeProxyReplacement": pulumi.BoolPtr(true),
		"ipam":                 pulumi.Map{"method": pulumi.StringPtr("kubernetes")},
		"socketLB":             pulumi.Map{"hostNamespaceOnly": pulumi.BoolPtr(true)},
		"cni":                  pulumi.Map{"exclusive": pulumi.BoolPtr(false)},
	}
	e2e.Run(t, &istioAmbientSuite{},
		e2e.WithStackName("stack-kind-cilium-istio-ambient"),
		e2e.WithProvisioner(provkindvm.Provisioner(provkindvm.WithRunOptions(
			scenkindvm.WithName("cilium-istio-ambient"),
			scenkindvm.WithCiliumOptions(cilium.WithHelmValues(ciliumHelmValues), cilium.WithVersion("1.17.4")),
			scenkindvm.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(systemProbeConfigNPMHelmValues),
				kubernetesagentparams.WithHelmValues(istioAmbientHelmValues),
			),
			scenkindvm.WithAgentDependentWorkloadApp(istioAmbientWorkload),
		))),
	)
}

// istioAmbientWorkload installs Istio ambient and the test workloads. The workloads start after
// the agent, so system-probe sees every app container start.
func istioAmbientWorkload(e config.Env, kubeProvider *kubernetes.Provider, dependsOnAgent pulumi.ResourceOption) (*kubeComp.Workload, error) {
	workload := &kubeComp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "istio-ambient-workload", workload); err != nil {
		return nil, err
	}
	// everything depends on the agent so teardown removes it first: a namespace deleted after the
	// cluster agent hangs on its stale external metrics APIService
	opts := []pulumi.ResourceOption{pulumi.Provider(kubeProvider), pulumi.Parent(workload), dependsOnAgent}

	mesh, err := istio.NewAmbientHelmInstallation(e, "", opts...)
	if err != nil {
		return nil, err
	}
	appOpts := append(opts, utils.PulumiDependsOn(mesh.Ztunnel))

	ambient := map[string]string{"istio.io/dataplane-mode": "ambient"}
	namespaces := map[string]map[string]string{
		"clients": ambient,
		"direct":  ambient,
		"waypointed": {
			"istio.io/dataplane-mode": "ambient",
			"istio.io/use-waypoint":   "waypoint",
		},
	}
	nsDeps := map[string]pulumi.ResourceOption{}
	for name, labels := range namespaces {
		ns, err := corev1.NewNamespace(e.Ctx(), "ambient-"+name, &corev1.NamespaceArgs{
			Metadata: metav1.ObjectMetaArgs{Name: pulumi.String(name), Labels: pulumi.ToStringMap(labels)},
		}, appOpts...)
		if err != nil {
			return nil, err
		}
		deps := []pulumi.Resource{ns}
		if e.ImagePullRegistry() != "" {
			secret, err := utils.NewImagePullSecret(e, name, append(appOpts, utils.PulumiDependsOn(ns))...)
			if err != nil {
				return nil, err
			}
			deps = append(deps, secret)
		}
		nsDeps[name] = utils.PulumiDependsOn(deps...)
	}

	if _, err := istio.NewWaypoint(e, "waypointed", append(appOpts, nsDeps["waypointed"])...); err != nil {
		return nil, err
	}

	httpbin := apps.Image(e, "apps-go-httpbin")
	tools := apps.Image(e, "apps-npm-tools")
	loop := func(targets ...string) []string {
		urls := make([]string, 0, len(targets))
		for _, t := range targets {
			urls = append(urls, "http://"+t+":8080/")
		}
		return []string{"sh", "-c", fmt.Sprintf("while true; do for u in %s; do curl -s -o /dev/null --max-time 2 $u || true; done; sleep 1; done", strings.Join(urls, " "))}
	}
	bothServers := loop("direct-server.direct", "waypoint-server.waypointed")
	sleep := []string{"sh", "-c", "sleep infinity"}

	deployments := []ambientDeployment{
		{namespace: "direct", name: "direct-server", role: "server", containers: []ambientContainer{{name: "server", image: httpbin, port: 8080}}},
		{namespace: "waypointed", name: "waypoint-server", role: "server", containers: []ambientContainer{{name: "server", image: httpbin, port: 8080}}},
		{namespace: "clients", name: "client", role: "client", containers: []ambientContainer{{name: "client", image: tools, command: bothServers}}},
		// vault-agent is on the default ignore list, so the pod still has one app container
		{namespace: "clients", name: "client-vault", role: "client", containers: []ambientContainer{
			{name: "client", image: tools, command: bothServers},
			{name: "vault-agent", image: tools, command: sleep},
		}},
		// two app containers: the pod is ambiguous, so its traffic stays on ztunnel
		{namespace: "clients", name: "client-two-apps", role: "client", containers: []ambientContainer{
			{name: "client", image: tools, command: bothServers},
			{name: "sql-proxy", image: tools, command: loop("direct-server.direct")},
		}},
	}
	for _, d := range deployments {
		if err := d.create(e, append(appOpts, nsDeps[d.namespace])...); err != nil {
			return nil, err
		}
	}

	return workload, nil
}

type ambientContainer struct {
	name    string
	image   string
	command []string
	port    int
}

type ambientDeployment struct {
	namespace  string
	name       string
	role       string
	containers []ambientContainer
}

func (d ambientDeployment) create(e config.Env, opts ...pulumi.ResourceOption) error {
	labels := pulumi.StringMap{"app": pulumi.String(d.name), "cnm-e2e/role": pulumi.String(d.role)}
	var containers corev1.ContainerArray
	var ports corev1.ServicePortArray
	for _, c := range d.containers {
		container := &corev1.ContainerArgs{Name: pulumi.String(c.name), Image: pulumi.String(c.image)}
		if len(c.command) > 0 {
			container.Command = pulumi.ToStringArray(c.command)
		}
		if c.port != 0 {
			container.Ports = corev1.ContainerPortArray{&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(c.port)}}
			ports = append(ports, &corev1.ServicePortArgs{Name: pulumi.String("http"), Port: pulumi.Int(c.port), TargetPort: pulumi.Int(c.port)})
		}
		containers = append(containers, container)
	}
	spec := &corev1.PodSpecArgs{Containers: containers}
	if e.ImagePullRegistry() != "" {
		spec.ImagePullSecrets = corev1.LocalObjectReferenceArray{corev1.LocalObjectReferenceArgs{Name: pulumi.String(utils.DefaultImagePullSecretName)}}
	}
	if d.role == "client" {
		// prefer another node than the servers, so traffic crosses nodes where the cluster has room
		spec.Affinity = &corev1.AffinityArgs{PodAntiAffinity: &corev1.PodAntiAffinityArgs{
			PreferredDuringSchedulingIgnoredDuringExecution: corev1.WeightedPodAffinityTermArray{&corev1.WeightedPodAffinityTermArgs{
				Weight: pulumi.Int(100),
				PodAffinityTerm: &corev1.PodAffinityTermArgs{
					TopologyKey:       pulumi.String("kubernetes.io/hostname"),
					NamespaceSelector: &metav1.LabelSelectorArgs{},
					LabelSelector:     &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"cnm-e2e/role": pulumi.String("server")}},
				},
			}},
		}}
	}

	deploy, err := appsv1.NewDeployment(e.Ctx(), d.namespace+"-"+d.name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(d.name), Namespace: pulumi.String(d.namespace)},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String(d.name)}},
			Template: &corev1.PodTemplateSpecArgs{Metadata: &metav1.ObjectMetaArgs{Labels: labels}, Spec: spec},
		},
	}, opts...)
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		return nil
	}
	_, err = corev1.NewService(e.Ctx(), d.namespace+"-"+d.name, &corev1.ServiceArgs{
		Metadata: metav1.ObjectMetaArgs{Name: pulumi.String(d.name), Namespace: pulumi.String(d.namespace)},
		Spec:     &corev1.ServiceSpecArgs{Selector: pulumi.StringMap{"app": pulumi.String(d.name)}, Ports: ports},
	}, append(opts, utils.PulumiDependsOn(deploy))...)
	return err
}

// BeforeTest will be called before each test
func (s *istioAmbientSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	if !s.BaseSuite.IsDevMode() {
		s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// AfterTest will be called after each test
func (s *istioAmbientSuite) AfterTest(suiteName, testName string) {
	test1HostFakeIntakeNPMDumpInfo(s.T(), s.Env().FakeIntake)
	s.BaseSuite.AfterTest(suiteName, testName)
}

// TestZtunnelSocketsInAppPodsAreAttributedToTheApp checks that every connection ztunnel opens or
// accepts inside a pod with one app container is attributed to that container, while the payload
// still maps ztunnel's PID to the ztunnel container.
func (s *istioAmbientSuite) TestZtunnelSocketsInAppPodsAreAttributedToTheApp() {
	ztunnel := s.ztunnelContainers()
	for _, tc := range []struct{ namespace, app, container string }{
		{"clients", "client", "client"},
		{"clients", "client-vault", "client"},
		{"direct", "direct-server", "server"},
		{"waypointed", "waypoint-server", "server"},
	} {
		s.Run(tc.app, func() {
			pod := s.pod(tc.namespace, tc.app)
			appID := pod.containers[tc.container]
			s.EventuallyWithT(func(c *assert.CollectT) {
				var moved, kept int
				owners := map[string]int{} // every connection from the pod, by its process's container
				for _, conn := range s.connectionsFrom(c, pod.ip) {
					owners[containerName(conn.pidContainer, pod, ztunnel)]++
					if ztunnel[conn.container] {
						kept++
					}
					if ztunnel[conn.pidContainer] && conn.container == appID {
						moved++
					}
				}
				assert.Zerof(c, kept, "%d connections from %s still attributed to ztunnel", kept, tc.app)
				assert.Positivef(c, moved, "no ztunnel connection from %s was attributed to %s; connections by owning container: %v", tc.app, tc.container, owners)
			}, 4*time.Minute, 10*time.Second)
		})
	}
}

// TestZtunnelSocketsInMultiAppPodsStayOnZtunnel checks that a pod with two app containers keeps
// today's attribution, because there's no single owner to move the socket to.
func (s *istioAmbientSuite) TestZtunnelSocketsInMultiAppPodsStayOnZtunnel() {
	ztunnel := s.ztunnelContainers()
	pod := s.pod("clients", "client-two-apps")
	s.EventuallyWithT(func(c *assert.CollectT) {
		var kept int
		moved := map[string]int{} // reattributed connections by the container they were moved to
		for _, conn := range s.connectionsFrom(c, pod.ip) {
			if !ztunnel[conn.pidContainer] {
				continue
			}
			if ztunnel[conn.container] {
				kept++
			} else {
				moved[containerName(conn.container, pod, ztunnel)]++
			}
		}
		assert.Emptyf(c, moved, "ztunnel connections from client-two-apps were reattributed (pod containers %v)", pod.containers)
		assert.Positive(c, kept, "no ztunnel connection from client-two-apps")
	}, 4*time.Minute, 10*time.Second)
}

// TestZtunnelOwnTrafficStaysOnZtunnel checks that sockets in ztunnel's own namespace, such as xDS
// to istiod, keep the ztunnel container.
func (s *istioAmbientSuite) TestZtunnelOwnTrafficStaysOnZtunnel() {
	ztunnel := s.ztunnelContainers()
	ips := s.ztunnelPodIPs()
	s.EventuallyWithT(func(c *assert.CollectT) {
		var own int
		for ip := range ips {
			for _, conn := range s.connectionsFrom(c, ip) {
				if !ztunnel[conn.pidContainer] {
					continue
				}
				assert.Equalf(c, conn.pidContainer, conn.container, "connection from ztunnel's own namespace was reattributed: %+v", conn)
				own++
			}
		}
		assert.Positive(c, own, "no connection from ztunnel's own namespace")
	}, 4*time.Minute, 10*time.Second)
}

type ambientConn struct {
	container    string // Laddr.ContainerId
	pidContainer string // ContainerForPid[Pid]: the owning process's container
	raddr        string
}

func (s *istioAmbientSuite) connectionsFrom(c *assert.CollectT, ip string) []ambientConn {
	cnx, err := s.Env().FakeIntake.Client().GetConnections()
	if !assert.NoError(c, err, "error getting connections") {
		return nil
	}
	var conns []ambientConn
	cnx.ForeachConnection(func(conn *agentmodel.Connection, cc *agentmodel.CollectorConnections, _ string) {
		if conn.Laddr == nil || conn.Laddr.Ip != ip {
			return
		}
		ac := ambientConn{container: conn.Laddr.ContainerId, pidContainer: cc.ContainerForPid[conn.Pid]}
		if conn.Raddr != nil {
			ac.raddr = fmt.Sprintf("%s:%d", conn.Raddr.Ip, conn.Raddr.Port)
		}
		conns = append(conns, ac)
	})
	return conns
}

type ambientPod struct {
	ip         string
	containers map[string]string // container name to runtime-less container ID
}

func (s *istioAmbientSuite) pod(namespace, app string) ambientPod {
	var pod ambientPod
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(namespace).List(context.Background(), k8smetav1.ListOptions{LabelSelector: "app=" + app})
		require.NoError(c, err)
		require.Len(c, pods.Items, 1, "want one %s pod", app)
		p := pods.Items[0]
		require.NotEmpty(c, p.Status.PodIP, "%s has no IP yet", app)
		pod = ambientPod{ip: p.Status.PodIP, containers: map[string]string{}}
		for _, st := range p.Status.ContainerStatuses {
			require.Truef(c, st.Ready, "%s/%s not ready", app, st.Name)
			pod.containers[st.Name] = trimContainerRuntime(st.ContainerID)
		}
	}, 5*time.Minute, 5*time.Second, "pod %s/%s not running", namespace, app)
	return pod
}

func (s *istioAmbientSuite) ztunnelPods(c *assert.CollectT) []ambientPod {
	pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("istio-system").List(context.Background(), k8smetav1.ListOptions{LabelSelector: "app=ztunnel"})
	require.NoError(c, err)
	require.NotEmpty(c, pods.Items, "no ztunnel pods")
	var out []ambientPod
	for _, p := range pods.Items {
		ap := ambientPod{ip: p.Status.PodIP, containers: map[string]string{}}
		for _, st := range p.Status.ContainerStatuses {
			require.NotEmpty(c, st.ContainerID, "ztunnel pod %s has no container ID yet", p.Name)
			ap.containers[st.Name] = trimContainerRuntime(st.ContainerID)
		}
		out = append(out, ap)
	}
	return out
}

func (s *istioAmbientSuite) ztunnelContainers() map[string]bool {
	ids := map[string]bool{}
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		for _, p := range s.ztunnelPods(c) {
			for _, id := range p.containers {
				ids[id] = true
			}
		}
	}, 2*time.Minute, 5*time.Second, "could not list ztunnel containers")
	return ids
}

func (s *istioAmbientSuite) ztunnelPodIPs() map[string]bool {
	ips := map[string]bool{}
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		for _, p := range s.ztunnelPods(c) {
			require.NotEmpty(c, p.ip)
			ips[p.ip] = true
		}
	}, 2*time.Minute, 5*time.Second, "could not list ztunnel pod IPs")
	return ips
}

// containerName names a container ID for failure messages: the pod's container name, ztunnel, or
// the ID's prefix when it's neither
func containerName(id string, pod ambientPod, ztunnel map[string]bool) string {
	for name, cid := range pod.containers {
		if cid == id {
			return name
		}
	}
	if ztunnel[id] {
		return "ztunnel"
	}
	if id == "" {
		return "<none>"
	}
	return "unknown:" + id[:min(12, len(id))]
}

// trimContainerRuntime strips the runtime prefix, as in containerd://<id>
func trimContainerRuntime(id string) string {
	if i := strings.Index(id, "://"); i >= 0 {
		return id[i+3:]
	}
	return id
}
