// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package mokka deploys Mokka (NVIDIA nvml-mock, https://github.com/NVIDIA/k8s-test-infra)
// on a Kubernetes cluster, so that CPU-only nodes expose simulated NVIDIA GPUs.
//
// Mokka stages a mock driver (libnvidia-ml, /dev/nvidiaN, sysfs) on each selected node and
// injects it, through an NRI plugin, into containers that are allocated GPUs or whose pod
// carries the [DevicesAnnotation]. On top of it this package deploys the NVIDIA device plugin,
// so that nodes advertise nvidia.com/gpu and workloads can request GPUs.
//
// Mokka does not execute CUDA: it covers what goes through NVML and the Kubernetes API
// (inventory, NVML metrics, GPU allocation, tagging, fault injection), not eBPF/CUDA tracing.
package mokka

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
)

// Component is a Mokka installation: the nvml-mock Helm release and the NVIDIA device plugin.
// Depend on it (e.g. with utils.PulumiDependsOn) before deploying the Agent or GPU workloads:
// the NRI plugin only injects the mock driver into containers created after it is running.
type Component struct {
	pulumi.ResourceState

	Release      *helm.Release
	DevicePlugin *appsv1.DaemonSet
}

// NewInstallation installs Mokka and the NVIDIA device plugin on the cluster behind kubeProvider.
func NewInstallation(e config.Env, params *Params, kubeProvider *kubernetes.Provider, opts ...pulumi.ResourceOption) (*Component, error) {
	comp := &Component{}
	if err := e.Ctx().RegisterComponentResource("dd:mokka", params.Name, comp, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(comp), pulumi.Provider(kubeProvider), pulumi.DeletedWith(kubeProvider))

	chart := params.ChartRepository + "/nvml-mock"
	var err error
	comp.Release, err = helm.NewRelease(e.Ctx(), e.CommonNamer().ResourceName(params.Name, "nvml-mock"), &helm.ReleaseArgs{
		Name:      pulumi.String(params.Name),
		Namespace: pulumi.String(params.Namespace),
		// Let Helm create the namespace: several installations (GPU pools) can share it.
		CreateNamespace: pulumi.Bool(true),
		Chart:           pulumi.String(chart),
		RepositoryOpts:  utils.HelmOCIRepositoryOpts(e, chart),
		Version:         pulumi.String(params.ChartVersion),
		Values:          chartValues(params),
		// The node pods restart containerd and stage the mock driver before becoming ready.
		Timeout: pulumi.Int(600),
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("mokka: nvml-mock release: %w", err)
	}

	comp.DevicePlugin, err = newDevicePlugin(e, params, append(opts, pulumi.DependsOn([]pulumi.Resource{comp.Release}))...)
	if err != nil {
		return nil, fmt.Errorf("mokka: device plugin: %w", err)
	}

	if err := e.Ctx().RegisterResourceOutputs(comp, pulumi.Map{
		"releaseStatus": comp.Release.Status,
	}); err != nil {
		return nil, err
	}
	return comp, nil
}

func chartValues(params *Params) pulumi.Map {
	values := pulumi.Map{
		"gpu": pulumi.Map{
			"profile": pulumi.String(params.Profile),
			"count":   pulumi.Int(params.GPUCount),
		},
		"nodeAgent": pulumi.Map{
			"kernelLog": pulumi.Map{"enabled": pulumi.Bool(params.KernelLog)},
		},
	}
	if len(params.NodeSelector) > 0 {
		values["nodeSelector"] = pulumi.ToStringMap(params.NodeSelector)
	}
	for k, v := range params.HelmValues {
		values[k] = v
	}
	return values
}

// newDevicePlugin deploys the NVIDIA device plugin on the mock driver. It mirrors
// tests/e2e/device-plugin-mock.yaml from NVIDIA/k8s-test-infra, but selects nodes with the
// same node selector as Mokka instead of the mokka.nvidia.com/type label, which is only set
// when the Mokka control plane is enabled.
func newDevicePlugin(e config.Env, params *Params, opts ...pulumi.ResourceOption) (*appsv1.DaemonSet, error) {
	name := params.Name + "-device-plugin"
	labels := pulumi.StringMap{"app": pulumi.String(name)}

	var nodeSelector pulumi.StringMapInput
	if len(params.NodeSelector) > 0 {
		nodeSelector = pulumi.ToStringMap(params.NodeSelector)
	}

	return appsv1.NewDaemonSet(e.Ctx(), e.CommonNamer().ResourceName(name), &appsv1.DaemonSetArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(params.Namespace),
		},
		Spec: appsv1.DaemonSetSpecArgs{
			Selector: metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: corev1.PodTemplateSpecArgs{
				Metadata: metav1.ObjectMetaArgs{Labels: labels},
				Spec: corev1.PodSpecArgs{
					NodeSelector: nodeSelector,
					// Mokka itself tolerates every taint, the device plugin must follow it.
					Tolerations: corev1.TolerationArray{corev1.TolerationArgs{Operator: pulumi.String("Exists")}},
					Containers: corev1.ContainerArray{corev1.ContainerArgs{
						Name:  pulumi.String("nvidia-device-plugin"),
						Image: pulumi.String(params.DevicePluginImage),
						Args: pulumi.StringArray{
							pulumi.String("--nvidia-driver-root=" + MockDriverRoot),
							pulumi.String("--driver-root-ctr-path=" + MockDriverRoot),
							pulumi.String("--device-discovery-strategy=nvml"),
							// The Mokka NRI plugin recognises a GPU allocation from the /dev/nvidiaN
							// device in the container, which kind nodes only get through device specs.
							pulumi.String("--pass-device-specs=true"),
						},
						SecurityContext: corev1.SecurityContextArgs{Privileged: pulumi.Bool(true)},
						VolumeMounts: corev1.VolumeMountArray{
							corev1.VolumeMountArgs{Name: pulumi.String("mock-root"), MountPath: pulumi.String("/var/lib/nvml-mock"), ReadOnly: pulumi.Bool(true)},
							corev1.VolumeMountArgs{Name: pulumi.String("device-plugins"), MountPath: pulumi.String("/var/lib/kubelet/device-plugins")},
						},
					}},
					Volumes: corev1.VolumeArray{
						corev1.VolumeArgs{Name: pulumi.String("mock-root"), HostPath: corev1.HostPathVolumeSourceArgs{Path: pulumi.String("/var/lib/nvml-mock")}},
						corev1.VolumeArgs{Name: pulumi.String("device-plugins"), HostPath: corev1.HostPathVolumeSourceArgs{Path: pulumi.String("/var/lib/kubelet/device-plugins")}},
					},
				},
			},
		},
	}, opts...)
}

// AgentHelmValues returns the Datadog Helm chart values needed for the node Agent to see
// the mock GPUs. Pass it to kubernetesagentparams.WithHelmValues.
//
//   - The devices annotation makes Mokka inject every mock GPU into the Agent pod, and points
//     LD_LIBRARY_PATH at the mock libnvidia-ml, so no NVML library path override is needed.
//   - gpuMonitoring.runtimeClassName must be empty: kind nodes have no "nvidia" RuntimeClass.
//   - It only sets maps, never lists, because Helm replaces lists when merging value files.
func AgentHelmValues() string {
	return fmt.Sprintf(`datadog:
  gpuMonitoring:
    enabled: true
    runtimeClassName: ""
agents:
  podAnnotations:
    %s: "true"
`, DevicesAnnotation)
}
