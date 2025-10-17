// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gpu

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

const agentNamespace = "datadog"
const podSelectorField = "app"
const jobQueryInterval = 500 * time.Millisecond
const jobQueryTimeout = 120 * time.Second // Might take some time to create the container
const errMsgNoCudaCapableDevice = "error code no CUDA-capable device is detected"
const maxWorkloadRetries = 3

type agentComponent string

const (
	agentComponentSystemProbe agentComponent = "system-probe"
	agentComponentCoreAgent   agentComponent = "core-agent"
)

var agentComponentToSystemdService = map[agentComponent]string{
	agentComponentSystemProbe: "datadog-agent-sysprobe.service",
	agentComponentCoreAgent:   "datadog-agent.service",
}

var agentComponentToContainer = map[agentComponent]string{
	agentComponentSystemProbe: "system-probe",
	agentComponentCoreAgent:   "agent",
}

// suiteCapabilities is an interface that exposes the capabilities of the test suite,
// generalizing between different environments
type suiteCapabilities interface {
	FakeIntake() *components.FakeIntake
	Agent() agentclient.Agent
	QuerySysprobe(path string) (string, error)
	RunContainerWorkloadWithGPUs(image string, arguments ...string) (string, error)
	GetRestartCount(component agentComponent) int
}

// hostCapabilities is an implementation of suiteCapabilities for the Host environment
type hostCapabilities struct {
	suite *e2e.BaseSuite[environments.Host]
}

var _ suiteCapabilities = &hostCapabilities{}

// FakeIntake returns the FakeIntake component of the test environment
func (c *hostCapabilities) FakeIntake() *components.FakeIntake {
	return c.suite.Env().FakeIntake
}

// Agent returns the Agent client of the test environment, which communicates directly
// with the host-installed agent
func (c *hostCapabilities) Agent() agentclient.Agent {
	return c.suite.Env().Agent.Client
}

// QuerySysprobe sends a query to the sysprobe socket on the host using the
// UNIX socket present in the root filesystem.
func (c *hostCapabilities) QuerySysprobe(path string) (string, error) {
	sysprobeSocket := "/opt/datadog-agent/run/sysprobe.sock"
	cmd := fmt.Sprintf("sudo curl -s --unix-socket %s http://unix/%s", sysprobeSocket, path)
	return c.suite.Env().RemoteHost.Execute(cmd)
}

func (c *hostCapabilities) removeContainer(containerName string) error {
	_, err := c.suite.Env().RemoteHost.Execute(fmt.Sprintf("docker rm -f %s", containerName))
	return err
}

// RunContainerWorkloadWithGPUs runs a container workload with GPUs on the host using Docker
func (c *hostCapabilities) RunContainerWorkloadWithGPUs(image string, arguments ...string) (string, error) {
	containerName := strings.ToLower("workload-" + common.RandString(5))

	args := strings.Join(arguments, " ")
	cmd := fmt.Sprintf("sudo docker run --gpus all --name %s %s %s", containerName, image, args)

	var err error
	var out string
	for retries := range maxWorkloadRetries {
		out, err = c.suite.Env().RemoteHost.Execute(cmd)
		if err == nil {
			break
		}

		// Remove the container and try again
		if removeErr := c.removeContainer(containerName); removeErr != nil {
			return out, fmt.Errorf("error removing container for retry: %w", removeErr)
		}

		log.Printf("Workload container could not start, retrying (attempt %d of %d), error: %v", retries+1, maxWorkloadRetries, err)
	}

	if err != nil {
		return "", fmt.Errorf("could not run container workload with GPUs after %d retries: %w", maxWorkloadRetries, err)
	}

	c.suite.T().Cleanup(func() {
		// Cleanup the container
		_ = c.removeContainer(containerName)
	})
	containerIDCmd := fmt.Sprintf("docker inspect -f {{.Id}} %s", containerName)
	idOut, err := c.suite.Env().RemoteHost.Execute(containerIDCmd)

	return strings.TrimSpace(idOut), err
}

func (c *hostCapabilities) GetRestartCount(component agentComponent) int {
	service := agentComponentToSystemdService[component]
	out, err := c.suite.Env().RemoteHost.Execute(fmt.Sprintf("systemctl show -p NRestarts %s", service))
	c.suite.Require().NoError(err)
	c.suite.Require().NotEmpty(out)

	restartCount := strings.TrimPrefix(strings.TrimSpace(out), "NRestarts=")
	count, err := strconv.Atoi(restartCount)
	c.suite.Require().NoError(err)
	return count
}

// kubernetesCapabilities is an implementation of suiteCapabilities for the Kubernetes environment
type kubernetesCapabilities struct {
	suite *e2e.BaseSuite[environments.Kubernetes]
}

var _ suiteCapabilities = &kubernetesCapabilities{}

// FakeIntake returns the FakeIntake component of the test environment
func (c *kubernetesCapabilities) FakeIntake() *components.FakeIntake {
	return c.suite.Env().FakeIntake
}

// Agent returns the Agent client of the test environment, which communicates with the agent
// running in a Kubernetes pod. It selects the first pod that runs the node agent, as in our e2e tests
// we only have one node agent per cluster.
func (c *kubernetesCapabilities) Agent() agentclient.Agent {
	linuxAgent := c.suite.Env().Agent.LinuxNodeAgent
	client, err := client.NewK8sAgentClient(c.suite, client.AgentSelectorAnyPod(linuxAgent), c.suite.Env().KubernetesCluster.KubernetesClient)
	c.suite.Require().NoError(err)
	return client
}

// QuerySysprobe sends a query to the sysprobe socket on the host using the
// UNIX socket present in the agent container, shared with the sysprobe container.
func (c *kubernetesCapabilities) QuerySysprobe(path string) (string, error) {
	pods, err := c.suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector(podSelectorField, c.suite.Env().Agent.LinuxNodeAgent.LabelSelectors[podSelectorField]).String(),
		Limit:         1,
	})
	if err != nil {
		panic(err)
	}

	if len(pods.Items) != 1 {
		panic("Expected to find a single pod")
	}

	pod := pods.Items[0]

	cmd := []string{"curl", "-s", "--unix-socket", "/var/run/sysprobe/sysprobe.sock", "http://unix/" + path}
	stdout, stderr, err := c.suite.Env().KubernetesCluster.KubernetesClient.PodExec(agentNamespace, pod.Name, "agent", cmd)
	return stdout + " " + stderr, err
}

// RunContainerWorkloadWithGPUs runs a container workload with GPUs on the Kubernetes cluster
// using a Kubernetes Job.
func (c *kubernetesCapabilities) RunContainerWorkloadWithGPUs(image string, arguments ...string) (string, error) {
	jobName := strings.ToLower("workload-" + common.RandString(5))
	jobNamespace := "default"

	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: jobNamespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "workload",
							Image:   image,
							Command: arguments,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}

	_, err := c.suite.Env().KubernetesCluster.Client().BatchV1().Jobs(jobNamespace).Create(context.Background(), jobSpec, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("error starting container workload with GPUs: %w", err)
	}

	// Now let's find the container ID
	var pods *corev1.PodList // keep the list here so that we can return a good error message on timeout
	maxTime := time.Now().Add(jobQueryTimeout)
	for time.Now().Before(maxTime) {
		pods, err = c.suite.Env().KubernetesCluster.Client().CoreV1().Pods(jobNamespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("job-name", jobName).String(), // job-name is the label automatically assigned by k8s to the pod running this job
			Limit:         1,
		})
		if err != nil {
			return "", fmt.Errorf("error listing pods for job %s: %w", jobName, err)
		}

		if len(pods.Items) > 0 {
			pod := pods.Items[0]
			if pod.Status.Phase != corev1.PodPending {
				return pod.Status.ContainerStatuses[0].ContainerID, nil
			}
		}

		time.Sleep(jobQueryInterval)
	}

	// Timed out, check what happened
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("Could not find a pod that matched job-name: %s", jobName)
	}

	pod := pods.Items[0]
	return "", fmt.Errorf("Pod %s found but is not running, status: %s %s (%s)", pod.Name, pod.Status.Phase, pod.Status.Message, pod.Status.Reason)
}

func (c *kubernetesCapabilities) GetRestartCount(component agentComponent) int {
	container := agentComponentToContainer[component]

	ctx := context.Background()
	linuxPods, err := c.suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", c.suite.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	c.suite.Require().NoError(err)

	restartCount := 0
	for _, pod := range linuxPods.Items {
		for _, containerStatus := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			if containerStatus.Name == container {
				restartCount += int(containerStatus.RestartCount)
			}
		}
	}

	return restartCount
}
