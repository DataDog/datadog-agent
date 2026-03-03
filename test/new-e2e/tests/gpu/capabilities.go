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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/k8s"
)

const agentNamespace = "datadog"
const jobNamespace = "default"
const podSelectorField = "app"
const jobQueryInterval = 500 * time.Millisecond
const jobQueryTimeout = 120 * time.Second // Might take some time to create the container
const errMsgNoCudaCapableDevice = "error code no CUDA-capable device is detected"
const maxWorkloadRetries = 3
const nvidiaRuntimeClassName = "nvidia"

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
	CheckWorkloadErrors(containerID string) error
	ExpectedWorkloadTags() []string
}

// hostCapabilities is an implementation of suiteCapabilities for the Host environment
type hostCapabilities struct {
	suite             *e2e.BaseSuite[environments.Host]
	containerIDToName map[string]string // maps container ID to container name
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
	_, err := c.suite.Env().RemoteHost.Execute("docker rm -f " + containerName)
	return err
}

// RunContainerWorkloadWithGPUs runs a container workload with GPUs on the host using Docker
func (c *hostCapabilities) RunContainerWorkloadWithGPUs(image string, arguments ...string) (string, error) {
	if c.containerIDToName == nil {
		c.containerIDToName = make(map[string]string)
	}

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
	containerIDCmd := "docker inspect -f {{.Id}} " + containerName
	idOut, err := c.suite.Env().RemoteHost.Execute(containerIDCmd)
	if err != nil {
		return "", err
	}

	containerID := strings.TrimSpace(idOut)
	c.containerIDToName[containerID] = containerName
	return containerID, nil
}

func (c *hostCapabilities) GetRestartCount(component agentComponent) int {
	service := agentComponentToSystemdService[component]
	out, err := c.suite.Env().RemoteHost.Execute("systemctl show -p NRestarts " + service)
	c.suite.Require().NoError(err)
	c.suite.Require().NotEmpty(out)

	restartCount := strings.TrimPrefix(strings.TrimSpace(out), "NRestarts=")
	count, err := strconv.Atoi(restartCount)
	c.suite.Require().NoError(err)
	return count
}

// CheckWorkloadErrors checks if a workload container has any errors.
// For host environments, it checks the Docker container exit code.
func (c *hostCapabilities) CheckWorkloadErrors(containerID string) error {
	containerName, found := c.containerIDToName[containerID]
	if !found {
		// Container ID not found in map, can't check status
		return nil
	}

	// Check container exit code using docker inspect
	exitCodeCmd := "docker inspect -f '{{.State.ExitCode}}' " + containerName
	exitCodeOut, err := c.suite.Env().RemoteHost.Execute(exitCodeCmd)
	if err != nil {
		return fmt.Errorf("error inspecting container %s: %w", containerName, err)
	}

	exitCodeStr := strings.TrimSpace(exitCodeOut)
	exitCode, err := strconv.Atoi(exitCodeStr)
	if err != nil {
		return fmt.Errorf("error parsing exit code for container %s: %w", containerName, err)
	}

	if exitCode != 0 {
		// Get container status for more details
		statusCmd := "docker inspect -f '{{.State.Status}}' " + containerName
		statusOut, _ := c.suite.Env().RemoteHost.Execute(statusCmd)
		status := strings.TrimSpace(statusOut)

		return fmt.Errorf("workload container %s exited with code %d (status: %s)", containerName, exitCode, status)
	}

	return nil
}

// ExpectedWorkloadTags returns tags that are expected to be present on workloads
func (c *hostCapabilities) ExpectedWorkloadTags() []string {
	return []string{"container_id", "container_name", "short_image"}
}

// kubernetesCapabilities is an implementation of suiteCapabilities for the Kubernetes environment
type kubernetesCapabilities struct {
	suite          *e2e.BaseSuite[environments.Kubernetes]
	containerToJob map[string]string // maps container ID to job name
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
	if c.containerToJob == nil {
		c.containerToJob = make(map[string]string)
	}

	jobName := strings.ToLower("workload-" + common.RandString(5))
	runtimeClassName := nvidiaRuntimeClassName

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
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									"nvidia.com/gpu": resource.MustParse("1"),
								},
							},
						},
					},
					RestartPolicy:    corev1.RestartPolicyNever,
					RuntimeClassName: &runtimeClassName,
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
				if len(pod.Status.ContainerStatuses) > 0 {
					containerID := pod.Status.ContainerStatuses[0].ContainerID
					c.containerToJob[containerID] = jobName
					return containerID, nil
				}
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

// CheckWorkloadErrors checks if a workload container has any errors.
// For Kubernetes environments, it checks the Job status and returns an error if the job failed.
func (c *kubernetesCapabilities) CheckWorkloadErrors(containerID string) error {
	jobName, found := c.containerToJob[containerID]
	if !found {
		// Container ID not found in map, can't check status
		return nil
	}

	return k8s.CheckJobErrors(context.Background(), c.suite.Env().KubernetesCluster.Client(), jobNamespace, jobName)
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

// ExpectedWorkloadTags returns tags that are expected to be present on workloads
func (c *kubernetesCapabilities) ExpectedWorkloadTags() []string {
	// Kubernetes tag support not added yet
	return nil
}
