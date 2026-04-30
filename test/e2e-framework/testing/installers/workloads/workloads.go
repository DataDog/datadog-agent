// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloads provides functions to deploy standard test workload
// applications to a Kubernetes cluster outside of a Pulumi context.
//
// Each workload is defined as an embedded YAML manifest (Go template) under a
// subdirectory of this package. The Deploy function renders and applies the
// selected manifests via kubectl and waits for all deployments to become
// available.
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
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
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

type deployParams struct {
	nginx        bool
	nginxPort    int
	redis        bool
	tracegen     bool
	prometheus   bool
	dogstatsd    *dogstatsdConfig
	cpustress    bool
	etcd         bool
	mutated      bool
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

// DeployTestWorkload deploys the full set of standard test workloads used by
// the containers test suite: nginx, redis, tracegen, prometheus, cpustress,
// etcd, mutated (admission controller), and dogstatsd clients.
func DeployTestWorkload(t *testing.T, env *environments.Kubernetes) {
	t.Helper()
	Deploy(t, env,
		WithNginx(),
		WithRedis(),
		WithTracegen(),
		WithPrometheus(),
		WithCPUStress(),
		WithEtcd(),
		WithMutated(),
		WithDogstatsd(),
	)
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

	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	require.NoError(t, err)
	defer os.Remove(kubeconfigFile.Name())
	_, err = kubeconfigFile.WriteString(env.KubernetesCluster.KubeConfig)
	require.NoError(t, err)
	require.NoError(t, kubeconfigFile.Close())

	kubeconfig := kubeconfigFile.Name()
	version := apps.Version

	if p.nginx {
		ns := "workload-nginx"
		applyManifest(t, kubeconfig, render(t, nginxManifest, map[string]any{
			"Version":   version,
			"Namespace": ns,
			"NginxPort": p.nginxPort,
		}))
		waitForDeployments(t, kubeconfig, ns, 5*time.Minute)
	}

	if p.redis {
		ns := "workload-redis"
		applyManifest(t, kubeconfig, render(t, redisManifest, map[string]any{
			"Version":   version,
			"Namespace": ns,
		}))
		waitForDeployments(t, kubeconfig, ns, 5*time.Minute)
	}

	if p.tracegen {
		ns := "workload-tracegen"
		applyManifest(t, kubeconfig, render(t, tracegenManifest, map[string]any{
			"Version":   version,
			"Namespace": ns,
		}))
		waitForDeployments(t, kubeconfig, ns, 5*time.Minute)
	}

	if p.prometheus {
		ns := "workload-prometheus"
		applyManifest(t, kubeconfig, render(t, prometheusManifest, map[string]any{
			"Version":   version,
			"Namespace": ns,
		}))
		waitForDeployments(t, kubeconfig, ns, 5*time.Minute)
	}

	if p.dogstatsd != nil {
		ns := "workload-dogstatsd"
		applyManifest(t, kubeconfig, render(t, dogstatsdManifest, map[string]any{
			"Version":      version,
			"Namespace":    ns,
			"StatsdPort":   p.dogstatsd.port,
			"StatsdSocket": p.dogstatsd.socket,
		}))
		waitForDeployments(t, kubeconfig, ns, 5*time.Minute)
	}

	if p.cpustress {
		ns := "workload-cpustress"
		applyManifest(t, kubeconfig, render(t, cpustressManifest, map[string]any{
			"Version":   version,
			"Namespace": ns,
		}))
		waitForDeployments(t, kubeconfig, ns, 5*time.Minute)
	}

	if p.etcd {
		applyManifest(t, kubeconfig, render(t, etcdManifest, map[string]any{
			"Version":   version,
			"Namespace": "etcd",
		}))
		waitForDeployments(t, kubeconfig, "etcd", 5*time.Minute)
	}

	if p.mutated {
		applyManifest(t, kubeconfig, render(t, mutatedManifest, map[string]any{
			"Version":             version,
			"NamespaceWithoutLib": "workload-mutated",
			"NamespaceWithLib":    "workload-mutated-lib-injection",
		}))
		waitForDeployments(t, kubeconfig, "workload-mutated", 10*time.Minute)
		waitForDeployments(t, kubeconfig, "workload-mutated-lib-injection", 10*time.Minute)
	}
}

// applyManifest runs kubectl apply with the manifest piped to stdin.
func applyManifest(t *testing.T, kubeconfigPath, manifest string) {
	t.Helper()
	cmd := exec.Command("kubectl", "apply", "-f", "-", "--kubeconfig", kubeconfigPath)
	cmd.Stdin = bytes.NewBufferString(manifest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("kubectl apply output:\n%s", string(output))
	}
	require.NoError(t, err, "kubectl apply failed")
	t.Logf("kubectl apply:\n%s", string(output))
}

// waitForDeployments waits for all deployments in the given namespace to have
// the Available condition. Uses kubectl wait so that it works with any cluster.
func waitForDeployments(t *testing.T, kubeconfigPath, namespace string, timeout time.Duration) {
	t.Helper()
	cmd := exec.Command("kubectl", "wait",
		"--for=condition=available",
		"deployment", "--all",
		"-n", namespace,
		fmt.Sprintf("--timeout=%ds", int(timeout.Seconds())),
		"--kubeconfig", kubeconfigPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("kubectl wait output:\n%s", string(output))
	}
	require.NoError(t, err, "deployments in namespace %s not ready after %s", namespace, timeout)
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
