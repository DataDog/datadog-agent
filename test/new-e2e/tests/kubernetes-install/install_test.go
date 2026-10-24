// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2026-present Datadog, Inc.

package kubernetesinstall

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	eks "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	eksprovisioner "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/eks"
)

type installSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	instructions installCase
	apiKey       string
}

func TestKubernetesInstall(t *testing.T) {
	// This suite is invoked only by the authenticated web-ui downstream pipeline.
	data, err := os.ReadFile(os.Getenv("WEBUI_INSTALL_BUNDLE"))
	require.NoError(t, err, "an explicit frontend bundle is required")
	b, err := parseBundle(data, os.Getenv("WEBUI_INSTALL_SOURCE_SHA"))
	require.NoError(t, err)
	cleanupOnly := os.Getenv("E2E_TEARDOWN_ONLY") == "true"
	apiKey := os.Getenv("WEBUI_INSTALL_API_KEY")
	if !cleanupOnly {
		require.NoError(t, verifyKey(b, apiKey))
		for _, tool := range []string{"helm", "kubectl", "aws"} {
			_, err := exec.LookPath(tool)
			require.NoError(t, err, "install required tools in the CI image before provisioning")
		}
	}
	require.NotEqual(t, "true", os.Getenv("E2E_DEV_MODE"), "fresh-install tests must destroy their infrastructure")
	require.NotEqual(t, "true", os.Getenv("REMOTE_STACK_CLEANING"), "wait for destruction before starting the next case")
	// Deliberately sequential: no reuse, no in-place retries, one cluster at a time.
	for _, c := range b.Cases {
		ok := t.Run(c.Method, func(t *testing.T) {
			e2e.Run(t, &installSuite{instructions: c, apiKey: apiKey},
				e2e.WithStackName(c.ClusterName),
				e2e.WithProvisioner(eksprovisioner.Provisioner(eksprovisioner.WithRunOptions(
					eks.WithName(c.ClusterName), eks.WithoutAgent(), eks.WithoutFakeIntake(),
					eks.WithEKSOptions(eks.WithLinuxNodeGroup(), eks.WithoutFargate(), eks.WithInternetAccess()),
				))),
			)
		})
		if !ok && !cleanupOnly {
			break
		}
	}
}

func (s *installSuite) TestCopiedInstructions() {
	t := s.T()
	ctx := context.Background()
	client := s.Env().KubernetesCluster.Client()
	defer s.collectEvidence()
	namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	for _, ns := range namespaces.Items {
		require.NotEqual(t, "datadog", ns.Name, "cluster is not fresh")
	}
	workloads, err := client.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	for _, ds := range workloads.Items {
		require.Equal(t, "kube-system", ds.Namespace, "unexpected preinstalled workload")
	}

	dir := t.TempDir()
	s.writeInstallerKubeconfig(dir)
	file := "datadog-agent.yaml"
	if s.instructions.Method == "helm" {
		file = "datadog-values.yaml"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(s.instructions.Config), 0600))
	for _, text := range []string{s.instructions.Setup, s.instructions.Install} {
		cmds, err := commands(text)
		require.NoError(t, err)
		for _, args := range cmds {
			s.writeInstallerKubeconfig(dir)
			for i := range args {
				args[i] = strings.ReplaceAll(args[i], keyMarker, s.apiKey)
			}
			commandCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			cmd := exec.CommandContext(commandCtx, args[0], args[1:]...)
			cmd.Dir = dir
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "KUBECONFIG=" + filepath.Join(dir, "kubeconfig")}
			// Never attach stdout/stderr: Helm failures can echo rendered credentials.
			err = cmd.Run()
			cancel()
			require.True(t, err == nil, "copied %s command failed (output suppressed to protect secrets)", args[0])
		}
	}
	var healthySince time.Time
	s.EventuallyWithT(func(c *assert.CollectT) {
		healthy := false
		defer func() {
			if !healthy {
				healthySince = time.Time{}
			}
		}()
		deployments, err := client.AppsV1().Deployments("datadog").List(ctx, metav1.ListOptions{})
		require.NoError(c, err)
		expected := 1
		if s.instructions.Method == "operator" {
			expected = 2
		}
		require.GreaterOrEqual(c, len(deployments.Items), expected)
		for _, d := range deployments.Items {
			require.Greater(c, d.Status.AvailableReplicas, int32(0), "%s is unavailable", d.Name)
			require.Equal(c, *d.Spec.Replicas, d.Status.UpdatedReplicas)
		}
		daemonsets, err := client.AppsV1().DaemonSets("datadog").List(ctx, metav1.ListOptions{})
		require.NoError(c, err)
		require.NotEmpty(c, daemonsets.Items)
		for _, d := range daemonsets.Items {
			require.Greater(c, d.Status.DesiredNumberScheduled, int32(0))
			require.Equal(c, d.Status.DesiredNumberScheduled, d.Status.NumberReady)
		}
		pods, err := client.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{})
		require.NoError(c, err)
		nodeCount, clusterCount := 0, 0
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil {
				continue
			}
			require.NotEmpty(c, pod.Status.ContainerStatuses)
			for _, container := range pod.Status.ContainerStatuses {
				require.True(c, container.Ready, "%s/%s is not ready", pod.Name, container.Name)
				require.Zero(c, container.RestartCount, "%s/%s restarted", pod.Name, container.Name)
			}
			role := pod.Labels["app.kubernetes.io/component"]
			if role != "agent" && role != "cluster-agent" {
				continue
			}
			s.checkEnrollment(c, pod)
			if role == "agent" {
				nodeCount++
				s.checkNodeStatus(c, pod)
			} else {
				clusterCount++
				s.checkIdentityRBAC(c, pod)
			}
		}
		require.Greater(c, nodeCount, 0)
		require.Greater(c, clusterCount, 0)
		healthy = true
		if healthySince.IsZero() {
			healthySince = time.Now()
		}
		require.GreaterOrEqual(c, time.Since(healthySince), time.Minute, "waiting for stable health")
	}, 15*time.Minute, 15*time.Second)
}

func (s *installSuite) collectEvidence() {
	t := s.T()
	client := s.Env().KubernetesCluster.Client()
	// Save safe diagnostics before teardown, on failure as well as success.
	pods, err := client.CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	images := map[string]any{}
	for _, pod := range pods.Items {
		for _, container := range pod.Status.ContainerStatuses {
			reason := ""
			if container.State.Waiting != nil {
				reason = container.State.Waiting.Reason
			}
			if container.State.Terminated != nil {
				reason = container.State.Terminated.Reason
			}
			images[pod.Name+"/"+container.Name] = map[string]any{"image": container.Image, "imageID": container.ImageID, "chart": pod.Labels["helm.sh/chart"], "ready": container.Ready, "restarts": container.RestartCount, "reason": reason}
		}
	}
	version, err := client.Discovery().ServerVersion()
	require.NoError(t, err)
	evidence, err := json.MarshalIndent(map[string]any{"sourceSHA": os.Getenv("WEBUI_INSTALL_SOURCE_SHA"), "method": s.instructions.Method, "clusterName": s.instructions.ClusterName, "images": images, "kubernetesVersion": version.GitVersion, "passed": !t.Failed()}, "", "  ")
	require.NoError(t, err)
	output := os.Getenv("WEBUI_INSTALL_EVIDENCE")
	require.NotEmpty(t, output)
	require.NoError(t, os.MkdirAll(output, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(output, s.instructions.Method+".json"), evidence, 0600))
}

// Resolve the framework's AWS exec credential before each command. Copied commands receive only
// a short-lived Kubernetes token, not the runner's AWS/SSM/Pulumi credentials.
func (s *installSuite) writeInstallerKubeconfig(dir string) {
	t := s.T()
	config, err := clientcmd.Load([]byte(s.Env().KubernetesCluster.KubeConfig))
	require.NoError(t, err)
	for _, auth := range config.AuthInfos {
		require.NotNil(t, auth.Exec)
		require.Equal(t, "aws", filepath.Base(auth.Exec.Command))
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		output, err := exec.CommandContext(ctx, auth.Exec.Command, auth.Exec.Args...).Output()
		cancel()
		require.True(t, err == nil, "could not obtain a short-lived EKS token")
		var credential struct {
			Status struct {
				Token string `json:"token"`
			} `json:"status"`
		}
		require.NoError(t, json.Unmarshal(output, &credential))
		require.NotEmpty(t, credential.Status.Token)
		auth.Exec = nil
		auth.Token = credential.Status.Token
	}
	data, err := clientcmd.Write(*config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kubeconfig"), data, 0600))
}

func (s *installSuite) checkEnrollment(c *assert.CollectT, pod corev1.Pod) {
	enrolled, persisted := false, false
	for _, container := range pod.Spec.Containers {
		logs, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").GetLogs(pod.Name, &corev1.PodLogOptions{Container: container.Name}).DoRaw(context.Background())
		require.True(c, err == nil, "unable to inspect enrollment logs")
		text := string(logs)
		enrolled = enrolled || strings.Contains(text, "Self-enrollment successful")
		if match := regexp.MustCompile(`Private Runner identity successfully persisted to (/[A-Za-z0-9_./-]+)`).FindStringSubmatch(text); match != nil {
			_, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", pod.Name, container.Name, []string{"test", "-s", match[1]})
			persisted = persisted || err == nil
		}
		persisted = persisted || strings.Contains(text, "Created PAR identity in K8s secret:")
	}
	require.True(c, enrolled, "%s has not freshly enrolled", pod.Name)
	require.True(c, persisted, "%s has not persisted its identity", pod.Name)
}

func (s *installSuite) checkNodeStatus(c *assert.CollectT, pod corev1.Pod) {
	stdout, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", pod.Name, "agent", []string{"agent", "status", "-j"})
	require.True(c, err == nil, "Agent status unavailable")
	var status struct {
		Runner struct {
			Checks map[string]map[string]struct {
				LastError string
				TotalRuns int
			}
		} `json:"runnerStats"`
		Forwarder struct {
			APIKeyStatus map[string]string
			Transactions struct{ Success int }
		} `json:"forwarderStats"`
		Logs struct {
			StatusMetrics map[string]string `json:"metrics"`
		} `json:"logsStats"`
	}
	require.True(c, json.Unmarshal([]byte(stdout), &status) == nil, "invalid Agent status JSON")
	require.NotEmpty(c, status.Runner.Checks["kubelet"], "kubelet check missing")
	for _, check := range status.Runner.Checks["kubelet"] {
		require.True(c, check.LastError == "", "kubelet collection failed")
		require.Greater(c, check.TotalRuns, 0)
	}
	require.NotEmpty(c, status.Forwarder.APIKeyStatus)
	for _, value := range status.Forwarder.APIKeyStatus {
		require.True(c, value == "API Key valid", "API key is not valid")
	}
	require.Greater(c, status.Forwarder.Transactions.Success, 0, "no successful telemetry delivery")
	require.NotEmpty(c, status.Logs.StatusMetrics["LogsSent"])
	require.NotEqual(c, "0", status.Logs.StatusMetrics["LogsSent"], "no logs delivered")
	runtimeConfig, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", pod.Name, "agent", []string{"agent", "config", "--all"})
	require.True(c, err == nil, "runtime config unavailable")
	var config map[string]any
	require.True(c, yaml.Unmarshal([]byte(runtimeConfig), &config) == nil, "invalid runtime config")
	require.True(c, config["kubelet_tls_verify"] == true, "runtime kubelet TLS verification is disabled")
	if s.instructions.Method == "operator" {
		par, ok := config["private_action_runner"].(map[string]any)
		require.True(c, ok && par["split_enabled"] == true, "Operator node PAR is not in split mode")
	}
}

func (s *installSuite) checkIdentityRBAC(c *assert.CollectT, pod corev1.Pod) {
	client := s.Env().KubernetesCluster.Client()
	secrets, err := client.CoreV1().Secrets("datadog").List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/component=private-action-runner"})
	require.NoError(c, err)
	require.NotEmpty(c, secrets.Items, "Cluster Agent has no persisted PAR identity secret")
	for _, secret := range secrets.Items {
		require.True(c, len(secret.Data["urn"]) > 0 && len(secret.Data["private_key"]) > 0, "incomplete PAR identity")
		for _, verb := range []string{"get", "create", "update"} {
			name := secret.Name
			if verb == "create" {
				name = ""
			}
			review, err := client.AuthorizationV1().SubjectAccessReviews().Create(context.Background(), &authorizationv1.SubjectAccessReview{Spec: authorizationv1.SubjectAccessReviewSpec{
				User:               "system:serviceaccount:datadog:" + pod.Spec.ServiceAccountName,
				ResourceAttributes: &authorizationv1.ResourceAttributes{Namespace: "datadog", Verb: verb, Resource: "secrets", Name: name},
			}}, metav1.CreateOptions{})
			require.NoError(c, err)
			require.True(c, review.Status.Allowed, "Cluster Agent lacks %s permission for its PAR identity", verb)
		}
	}
}
