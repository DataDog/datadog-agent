// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloads provides functions to deploy standard test workload
// applications to a Kubernetes cluster outside of a Pulumi context.
//
// Each workload is defined as an embedded YAML manifest (Go template) under a
// subdirectory of this package. The Deploy function renders and applies the
// selected manifests via the Kubernetes Go client and waits for all deployments
// to become available.
//
// Usage in SetupSuite:
//
//	workloads.Deploy(s.T(), s.Env(),
//	    workloads.WithNginx(),
//	    workloads.WithRedis(),
//	    workloads.WithTracegen(),
//	)
//
// Or to deploy the full standard set used by the containers test suite:
//
//	workloads.DeployTestWorkload(s.T(), s.Env())
package workloads

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apimachineryYAML "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

//go:embed nginx/manifest.yaml
var nginxManifest string

//go:embed redis/manifest.yaml
var redisManifest string

//go:embed tracegen/manifest.yaml
var tracegenManifest string

//go:embed prometheus/manifest.yaml
var prometheusManifest string

//go:embed dogstatsd/manifest.yaml
var dogstatsdManifest string

//go:embed cpustress/manifest.yaml
var cpustressManifest string

//go:embed etcd/manifest.yaml
var etcdManifest string

//go:embed mutated/manifest.yaml
var mutatedManifest string

//go:embed argorollout/manifest.yaml
var argoRolloutManifest string

type deployParams struct {
	nginx               bool
	nginxPort           int
	redis               bool
	tracegen            bool
	prometheus          bool
	dogstatsd           *dogstatsdConfig
	dogstatsdStandalone bool
	cpustress           bool
	etcd                bool
	mutated             bool
	argoRollout         bool
}

type dogstatsdConfig struct {
	port   int
	socket string
}

// Option configures which workloads to deploy.
type Option func(*deployParams)

// WithNginx deploys the nginx workload (Deployment, Service, ConfigMap, PDB).
func WithNginx() Option {
	return func(p *deployParams) {
		p.nginx = true
		p.nginxPort = 80
	}
}

// WithRedis deploys the redis workload (Deployment, Service, PDB).
func WithRedis() Option { return func(p *deployParams) { p.redis = true } }

// WithTracegen deploys the tracegen workload (UDS and TCP Deployments).
func WithTracegen() Option { return func(p *deployParams) { p.tracegen = true } }

// WithPrometheus deploys the Prometheus metrics app workload.
func WithPrometheus() Option { return func(p *deployParams) { p.prometheus = true } }

// WithCPUStress deploys the stress-ng CPU load workload.
func WithCPUStress() Option { return func(p *deployParams) { p.cpustress = true } }

// WithEtcd deploys the etcd workload used for service discovery config testing.
func WithEtcd() Option { return func(p *deployParams) { p.etcd = true } }

// WithMutated deploys workloads used for admission controller mutation testing.
func WithMutated() Option { return func(p *deployParams) { p.mutated = true } }

// WithDogstatsdStandalone deploys DogStatsD client workloads targeting the
// dogstatsd-standalone DaemonSet (port 8128, socket
// /run/datadog/dsd-standalone.socket) into the workload-dogstatsd-standalone
// namespace. Use alongside scenkind.WithDeployDogstatsd() which deploys the
// standalone DaemonSet itself.
func WithDogstatsdStandalone() Option {
	return func(p *deployParams) { p.dogstatsdStandalone = true }
}

// WithArgoRolloutNginx deploys an nginx Rollout workload in the
// workload-argo-rollout-nginx namespace. Requires ArgoRollout to be installed.
func WithArgoRolloutNginx() Option { return func(p *deployParams) { p.argoRollout = true } }

// WithDogstatsd deploys DogStatsD client workloads targeting the agent via UDS
// and UDP. The agent socket path defaults to /var/run/datadog/dsd.socket.
func WithDogstatsd() Option {
	return func(p *deployParams) {
		p.dogstatsd = &dogstatsdConfig{
			port:   8125,
			socket: "/var/run/datadog/dsd.socket",
		}
	}
}

// DefaultTestWorkloadOptions returns the full set of standard test workload
// options used by the containers test suite. Use this with provisioner
// WithWorkloads to declare workloads at provisioner construction time:
//
//	provkind.WithWorkloads(workloads.DefaultTestWorkloadOptions()...)
func DefaultTestWorkloadOptions() []Option {
	return []Option{
		WithNginx(),
		WithRedis(),
		WithTracegen(),
		WithPrometheus(),
		WithCPUStress(),
		WithEtcd(),
		WithMutated(),
		WithDogstatsd(),
	}
}

// DeployTestWorkload deploys the full set of standard test workloads used by
// the containers test suite: nginx, redis, tracegen, prometheus, cpustress,
// etcd, mutated (admission controller), and dogstatsd clients.
func DeployTestWorkload(t *testing.T, env *environments.Kubernetes) {
	t.Helper()
	Deploy(t, env, DefaultTestWorkloadOptions()...)
}

// k8sClients bundles the clients needed to apply manifests and wait for
// workloads. Built once from the cluster kubeconfig and shared across all
// manifest applications within a single Deploy call.
type k8sClients struct {
	dynamic    dynamic.Interface
	typed      kubernetes.Interface
	restMapper meta.RESTMapper
	// hpaAPIVersion is the apiVersion to use for HorizontalPodAutoscaler
	// manifests on this cluster. autoscaling/v2 is GA from k8s 1.23 onward;
	// older clusters (1.19, 1.22) only expose autoscaling/v2beta2. Set once
	// per Deploy from the server version and used by manifest templates.
	hpaAPIVersion string
}

func newK8sClients(kubeconfig string) (*k8sClients, error) {
	rc, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("parsing kubeconfig: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(rc)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	typedClient, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return nil, fmt.Errorf("creating typed client: %w", err)
	}

	dc, err := discovery.NewDiscoveryClientForConfig(rc)
	if err != nil {
		return nil, fmt.Errorf("creating discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	hpaAPIVersion, err := resolveHPAAPIVersion(typedClient)
	if err != nil {
		return nil, fmt.Errorf("resolving HPA API version: %w", err)
	}

	return &k8sClients{
		dynamic:       dynClient,
		typed:         typedClient,
		restMapper:    mapper,
		hpaAPIVersion: hpaAPIVersion,
	}, nil
}

// resolveHPAAPIVersion picks the HorizontalPodAutoscaler apiVersion supported
// by the target cluster: autoscaling/v2 for k8s >= 1.23, autoscaling/v2beta2
// for older clusters (1.19, 1.22). Mirrors the version-conditional logic in
// the Pulumi nginx/redis app definitions.
func resolveHPAAPIVersion(typedClient kubernetes.Interface) (string, error) {
	v, err := typedClient.Discovery().ServerVersion()
	if err != nil {
		return "", fmt.Errorf("fetching server version: %w", err)
	}
	major, err := strconv.Atoi(strings.TrimSuffix(v.Major, "+"))
	if err != nil {
		return "", fmt.Errorf("parsing server major version %q: %w", v.Major, err)
	}
	minor, err := strconv.Atoi(strings.TrimSuffix(v.Minor, "+"))
	if err != nil {
		return "", fmt.Errorf("parsing server minor version %q: %w", v.Minor, err)
	}
	if major > 1 || (major == 1 && minor >= 23) {
		return "autoscaling/v2", nil
	}
	return "autoscaling/v2beta2", nil
}

// Deploy applies the selected workload manifests to the cluster and waits for
// all deployments to become available.
func Deploy(t *testing.T, env *environments.Kubernetes, opts ...Option) {
	t.Helper()
	require.NotNil(t, env.KubernetesCluster, "workloads.Deploy: KubernetesCluster is nil, infrastructure must be provisioned first")

	p := &deployParams{}
	for _, opt := range opts {
		opt(p)
	}

	clients, err := newK8sClients(env.KubernetesCluster.KubeConfig)
	require.NoError(t, err, "workloads.Deploy: failed to build k8s clients")

	ctx := context.Background()
	version := apps.Version
	imageRegistry, _ := runner.GetProfile().ParamStore().GetWithDefault(parameters.ImagePullRegistry, "")

	if p.nginx {
		ns := "workload-nginx"
		pullSecret, err := ensureImagePullSecret(ctx, clients, ns)
		require.NoError(t, err, "workloads.Deploy: failed to create pull secret in %s", ns)
		applyManifest(t, clients, render(t, nginxManifest, map[string]any{
			"Version":         version,
			"Namespace":       ns,
			"NginxPort":       p.nginxPort,
			"HPAAPIVersion":   clients.hpaAPIVersion,
			"ImagePullSecret": pullSecret,
		}))
		waitForDeployments(t, clients.typed, ns, 5*time.Minute)
	}

	if p.redis {
		ns := "workload-redis"
		pullSecret, err := ensureImagePullSecret(ctx, clients, ns)
		require.NoError(t, err, "workloads.Deploy: failed to create pull secret in %s", ns)
		applyManifest(t, clients, render(t, redisManifest, map[string]any{
			"Version":         version,
			"Namespace":       ns,
			"HPAAPIVersion":   clients.hpaAPIVersion,
			"ImagePullSecret": pullSecret,
		}))
		waitForDeployments(t, clients.typed, ns, 5*time.Minute)
	}

	if p.tracegen {
		ns := "workload-tracegen"
		pullSecret, err := ensureImagePullSecret(ctx, clients, ns)
		require.NoError(t, err, "workloads.Deploy: failed to create pull secret in %s", ns)
		applyManifest(t, clients, render(t, tracegenManifest, map[string]any{
			"Version":         version,
			"Namespace":       ns,
			"ImagePullSecret": pullSecret,
		}))
		waitForDeployments(t, clients.typed, ns, 5*time.Minute)
	}

	if p.prometheus {
		ns := "workload-prometheus"
		pullSecret, err := ensureImagePullSecret(ctx, clients, ns)
		require.NoError(t, err, "workloads.Deploy: failed to create pull secret in %s", ns)
		applyManifest(t, clients, render(t, prometheusManifest, map[string]any{
			"Version":         version,
			"Namespace":       ns,
			"ImagePullSecret": pullSecret,
		}))
		waitForDeployments(t, clients.typed, ns, 5*time.Minute)
	}

	if p.dogstatsd != nil {
		ns := "workload-dogstatsd"
		pullSecret, err := ensureImagePullSecret(ctx, clients, ns)
		require.NoError(t, err, "workloads.Deploy: failed to create pull secret in %s", ns)
		applyManifest(t, clients, render(t, dogstatsdManifest, map[string]any{
			"Version":         version,
			"Namespace":       ns,
			"StatsdPort":      p.dogstatsd.port,
			"StatsdSocket":    p.dogstatsd.socket,
			"ImagePullSecret": pullSecret,
		}))
		waitForDeployments(t, clients.typed, ns, 5*time.Minute)
	}

	if p.dogstatsdStandalone {
		// Deploy clients that target the dogstatsd-standalone DaemonSet.
		// Port 8128 and socket /run/datadog/dsd-standalone.socket match the
		// constants in components/datadog/dogstatsd-standalone/k8s.go.
		ns := "workload-dogstatsd-standalone"
		pullSecret, err := ensureImagePullSecret(ctx, clients, ns)
		require.NoError(t, err, "workloads.Deploy: failed to create pull secret in %s", ns)
		applyManifest(t, clients, render(t, dogstatsdManifest, map[string]any{
			"Version":         version,
			"Namespace":       ns,
			"StatsdPort":      8128,
			"StatsdSocket":    "/run/datadog/dsd-standalone.socket",
			"ImagePullSecret": pullSecret,
		}))
		waitForDeployments(t, clients.typed, ns, 5*time.Minute)
	}

	if p.cpustress {
		ns := "workload-cpustress"
		applyManifest(t, clients, render(t, cpustressManifest, map[string]any{
			"Version":   version,
			"Namespace": ns,
		}))
		waitForDeployments(t, clients.typed, ns, 5*time.Minute)
	}

	if p.etcd {
		ns := "etcd"
		pullSecret, err := ensureImagePullSecret(ctx, clients, ns)
		require.NoError(t, err, "workloads.Deploy: failed to create pull secret in %s", ns)
		applyManifest(t, clients, render(t, etcdManifest, map[string]any{
			"Version":   version,
			"Namespace": ns,
			// Route through ECR pull-through cache in CI to avoid Quay.io rate limits.
			// Mirrors etcd/k8s.go: imageRegistry + "/quay/coreos/etcd:v3.5.1"
			"EtcdImage":       resolveImage(imageRegistry, "quay.io/coreos/etcd:v3.5.1"),
			"ImagePullSecret": pullSecret,
		}))
		waitForDeployments(t, clients.typed, ns, 5*time.Minute)
	}

	if p.mutated {
		pullSecret1, err := ensureImagePullSecret(ctx, clients, "workload-mutated")
		require.NoError(t, err, "workloads.Deploy: failed to create pull secret in workload-mutated")
		_, err = ensureImagePullSecret(ctx, clients, "workload-mutated-lib-injection")
		require.NoError(t, err, "workloads.Deploy: failed to create pull secret in workload-mutated-lib-injection")
		applyManifest(t, clients, render(t, mutatedManifest, map[string]any{
			"Version":             version,
			"NamespaceWithoutLib": "workload-mutated",
			"NamespaceWithLib":    "workload-mutated-lib-injection",
			// Route through ECR pull-through cache in CI to avoid Docker Hub rate limits.
			// Mirrors mutatedbyadmissioncontroller/k8s.go: imageRegistry + "/dockerhub/library/python:3.12-slim"
			"PythonImage":     resolveImage(imageRegistry, "python:3.12-slim"),
			"ImagePullSecret": pullSecret1,
		}))
		waitForDeployments(t, clients.typed, "workload-mutated", 10*time.Minute)
		waitForDeployments(t, clients.typed, "workload-mutated-lib-injection", 10*time.Minute)
	}

	if p.argoRollout {
		ns := "workload-argo-rollout-nginx"
		pullSecret, err := ensureImagePullSecret(ctx, clients, ns)
		require.NoError(t, err, "workloads.Deploy: failed to create pull secret in %s", ns)
		applyManifest(t, clients, render(t, argoRolloutManifest, map[string]any{
			"Version":         version,
			"Namespace":       ns,
			"NginxPort":       80,
			"ImagePullSecret": pullSecret,
		}))
		waitForPods(t, clients.typed, ns, 5*time.Minute)
	}
}

// applyManifest applies a (potentially multi-document) YAML manifest to the
// cluster using server-side apply via the dynamic client. No kubectl binary
// required — all k8s API calls go through the Go client library.
func applyManifest(t *testing.T, clients *k8sClients, manifest string) {
	t.Helper()
	ctx := context.Background()

	// Split on YAML document separator; each segment is applied independently.
	for doc := range strings.SplitSeq(manifest, "\n---") {
		doc = strings.TrimSpace(doc)
		// Skip empty docs and bare separator lines
		if doc == "" || doc == "---" {
			continue
		}
		// Strip a leading "---" that may appear at the very start of a document
		doc = strings.TrimPrefix(doc, "---")
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		// Convert YAML → JSON so we can unmarshal into unstructured
		jsonData, err := apimachineryYAML.ToJSON([]byte(doc))
		require.NoError(t, err, "workloads: failed to convert YAML to JSON")

		var obj unstructured.Unstructured
		require.NoError(t, json.Unmarshal(jsonData, &obj.Object), "workloads: failed to unmarshal manifest document")

		if obj.GetKind() == "" {
			continue
		}

		// Resolve GVK → GVR via the REST mapper (talks to the API server)
		gvk := obj.GroupVersionKind()
		mapping, err := clients.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		require.NoError(t, err, "workloads: failed to map GVK %s to GVR", gvk)

		// Choose namespaced or cluster-scoped resource interface
		var dr dynamic.ResourceInterface
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			dr = clients.dynamic.Resource(mapping.Resource).Namespace(obj.GetNamespace())
		} else {
			dr = clients.dynamic.Resource(mapping.Resource)
		}

		// Server-side apply — Force resolves conflicts; FieldManager identifies this installer
		_, err = dr.Apply(ctx, obj.GetName(), &obj, metav1.ApplyOptions{
			FieldManager: "e2e-workloads",
			Force:        true,
		})
		require.NoError(t, err, "workloads: failed to apply %s %s/%s", gvk.Kind, obj.GetNamespace(), obj.GetName())
		t.Logf("workloads: applied %s %s/%s", gvk.Kind, obj.GetNamespace(), obj.GetName())
	}
}

// waitForDeployments polls until every Deployment in the given namespace has
// the Available condition set to True, or until timeout expires.
func waitForDeployments(t *testing.T, k8sClient kubernetes.Interface, namespace string, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		list, err := k8sClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Logf("workloads: listing deployments in %s: %v (retrying)", namespace, err)
			time.Sleep(5 * time.Second)
			continue
		}

		if len(list.Items) == 0 {
			time.Sleep(5 * time.Second)
			continue
		}

		allAvailable := true
		for i := range list.Items {
			if !deploymentAvailable(&list.Items[i]) {
				allAvailable = false
				break
			}
		}
		if allAvailable {
			t.Logf("workloads: all %d deployment(s) in %s are available", len(list.Items), namespace)
			return
		}
		time.Sleep(5 * time.Second)
	}

	require.Fail(t, fmt.Sprintf("workloads: deployments in namespace %s not available after %s", namespace, timeout))
}

func deploymentAvailable(d *appsv1.Deployment) bool {
	for _, c := range d.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// waitForPods polls until every Pod in the given namespace has the Ready
// condition set to True. Used for CRD-based workloads (e.g. ArgoRollout) that
// don't expose a Deployment.
func waitForPods(t *testing.T, k8sClient kubernetes.Interface, namespace string, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		list, err := k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Logf("workloads: listing pods in %s: %v (retrying)", namespace, err)
			time.Sleep(5 * time.Second)
			continue
		}

		if len(list.Items) == 0 {
			time.Sleep(5 * time.Second)
			continue
		}

		allReady := true
		for i := range list.Items {
			if !podReady(&list.Items[i]) {
				allReady = false
				break
			}
		}
		if allReady {
			t.Logf("workloads: all %d pod(s) in %s are ready", len(list.Items), namespace)
			return
		}
		time.Sleep(5 * time.Second)
	}

	require.Fail(t, fmt.Sprintf("workloads: pods in namespace %s not ready after %s", namespace, timeout))
}

func podReady(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// resolveImage returns the image reference routed through the ECR pull-through
// cache when ImagePullRegistry is configured, mirroring the logic in the
// Pulumi app components (e.g. etcd/k8s.go, mutatedbyadmissioncontroller/k8s.go).
//
// The mapping mirrors the ECR pull-through cache path conventions:
//   - quay.io/foo/bar:tag  → <registry>/quay/foo/bar:tag
//   - python:3.12-slim     → <registry>/dockerhub/library/python:3.12-slim
func resolveImage(imageRegistry, image string) string {
	if imageRegistry == "" {
		return image
	}
	primaryRegistry := strings.SplitN(imageRegistry, ",", 2)[0]
	if path, ok := strings.CutPrefix(image, "quay.io/"); ok {
		return primaryRegistry + "/quay/" + path
	}
	// Docker Hub official image (no registry prefix)
	return primaryRegistry + "/dockerhub/library/" + image
}

// render executes a text/template with the given variables.
func render(t *testing.T, tmplStr string, vars map[string]any) string {
	t.Helper()
	tmpl, err := template.New("").Parse(tmplStr)
	require.NoError(t, err, "failed to parse manifest template")
	var buf bytes.Buffer
	require.NoError(t, tmpl.Execute(&buf, vars), "failed to render manifest template")
	return buf.String()
}

const workloadPullSecretName = "registry-credentials"

// ensureImagePullSecret pre-creates the namespace and inserts a
// kubernetes.io/dockerconfigjson secret named "registry-credentials" into it
// when ImagePullRegistry is configured. The namespace is created before the
// secret so the secret does not race with pod scheduling when the manifest is
// applied immediately after. Returns the secret name, or "" if no registry
// credentials are configured.
func ensureImagePullSecret(ctx context.Context, clients *k8sClients, namespace string) (string, error) {
	profile := runner.GetProfile()
	registryStr, _ := profile.ParamStore().GetWithDefault(parameters.ImagePullRegistry, "")
	if registryStr == "" {
		return "", nil
	}

	// Pre-create namespace before inserting the pull secret
	_, err := clients.typed.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return "", fmt.Errorf("creating namespace %s: %w", namespace, err)
	}

	usernameStr, _ := profile.ParamStore().GetWithDefault(parameters.ImagePullUsername, "")
	passwordStr, _ := profile.ParamStore().GetWithDefault(parameters.ImagePullPassword, "")
	if usernameStr == "" || passwordStr == "" {
		return "", fmt.Errorf("image_pull_registry is set but image_pull_username or image_pull_password is missing")
	}

	registries := strings.Split(registryStr, ",")
	usernames := strings.Split(usernameStr, ",")
	passwords := strings.Split(passwordStr, ",")

	if len(registries) != len(usernames) || len(registries) != len(passwords) {
		return "", fmt.Errorf("image_pull_registry, image_pull_username, and image_pull_password must have the same number of comma-separated entries")
	}

	authMap := make(map[string]map[string]string, len(registries))
	for i := range registries {
		pwd := strings.TrimSpace(passwords[i])
		if strings.HasPrefix(pwd, "b64=") {
			decoded, err := base64.StdEncoding.DecodeString(pwd[4:])
			if err != nil {
				return "", fmt.Errorf("failed to base64-decode password for registry %s: %w", registries[i], err)
			}
			pwd = string(decoded)
		}
		reg := strings.TrimSpace(registries[i])
		usr := strings.TrimSpace(usernames[i])
		authMap[reg] = map[string]string{
			"username": usr,
			"password": pwd,
			"auth":     base64.StdEncoding.EncodeToString([]byte(usr + ":" + pwd)),
		}
	}

	dockerConfigJSON, err := json.Marshal(map[string]map[string]map[string]string{"auths": authMap})
	if err != nil {
		return "", fmt.Errorf("marshaling dockerconfigjson: %w", err)
	}

	_, err = clients.typed.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: workloadPullSecretName, Namespace: namespace},
		StringData: map[string]string{
			".dockerconfigjson": string(dockerConfigJSON),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return "", fmt.Errorf("creating pull secret in %s: %w", namespace, err)
	}

	return workloadPullSecretName, nil
}
