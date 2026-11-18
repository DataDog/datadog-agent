// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package mokka

import (
	"errors"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
)

const (
	// DefaultChartVersion is the pinned version of the nvml-mock Helm chart.
	DefaultChartVersion = "0.7.0"
	// DefaultDevicePluginImage is the NVIDIA device plugin image deployed on top of the mock driver.
	DefaultDevicePluginImage = "nvcr.io/nvidia/k8s-device-plugin:v0.18.2"
	// DefaultNamespace is the namespace Mokka is installed in. The Mokka NRI plugin never
	// injects the mock driver into pods of its own namespace, so workloads must live elsewhere.
	DefaultNamespace = "mokka"
	// DefaultProfile is the GPU profile simulated by default.
	DefaultProfile = "t4"

	// DevicesAnnotation makes Mokka inject every mock GPU of the node into a pod that
	// does not request GPUs, e.g. the Datadog Agent DaemonSet.
	DevicesAnnotation = "nvml-mock.nvidia.com/devices"
	// MockDriverRoot is the host directory holding the mock driver tree.
	MockDriverRoot = "/var/lib/nvml-mock/driver"
)

// Params holds the configuration of a Mokka installation.
type Params struct {
	// Name is used to name the Pulumi resources and the Helm release, so that
	// several installations (e.g. one per node pool) can coexist.
	Name              string
	Namespace         string
	ChartVersion      string
	Profile           string
	GPUCount          int
	NodeSelector      map[string]string
	KernelLog         bool
	DevicePluginImage string
	HelmValues        pulumi.Map
}

// Option configures a Mokka installation.
type Option = func(*Params) error

// NewParams returns the Mokka parameters with defaults applied.
func NewParams(options ...Option) (*Params, error) {
	params := &Params{
		Name:              "mokka",
		Namespace:         DefaultNamespace,
		ChartVersion:      DefaultChartVersion,
		Profile:           DefaultProfile,
		GPUCount:          1,
		DevicePluginImage: DefaultDevicePluginImage,
	}
	params, err := common.ApplyOption(params, options)
	if err != nil {
		return nil, err
	}
	if params.GPUCount < 1 {
		return nil, errors.New("mokka: GPU count must be at least 1")
	}
	return params, nil
}

// WithName sets the name of the installation. Use a different name for each
// installation when deploying several GPU pools in the same cluster.
func WithName(name string) Option {
	return func(p *Params) error {
		p.Name = name
		return nil
	}
}

// WithNamespace sets the namespace Mokka is installed in.
func WithNamespace(namespace string) Option {
	return func(p *Params) error {
		p.Namespace = namespace
		return nil
	}
}

// WithChartVersion overrides the pinned nvml-mock chart version.
func WithChartVersion(version string) Option {
	return func(p *Params) error {
		p.ChartVersion = version
		return nil
	}
}

// WithProfile sets the simulated GPU model, e.g. "t4", "h100", "gb200".
func WithProfile(profile string) Option {
	return func(p *Params) error {
		p.Profile = profile
		return nil
	}
}

// WithGPUCount sets the number of mock GPUs per node.
func WithGPUCount(count int) Option {
	return func(p *Params) error {
		p.GPUCount = count
		return nil
	}
}

// WithNodeSelector restricts the mock GPUs (and the device plugin) to the matching nodes.
func WithNodeSelector(selector map[string]string) Option {
	return func(p *Params) error {
		p.NodeSelector = selector
		return nil
	}
}

// WithKernelLog makes Mokka write injected Xid errors to the node kernel log.
func WithKernelLog() Option {
	return func(p *Params) error {
		p.KernelLog = true
		return nil
	}
}

// WithDevicePluginImage overrides the NVIDIA device plugin image.
func WithDevicePluginImage(image string) Option {
	return func(p *Params) error {
		p.DevicePluginImage = image
		return nil
	}
}

// WithHelmValues sets extra nvml-mock chart values. They take precedence over the
// values derived from the other options.
func WithHelmValues(values pulumi.Map) Option {
	return func(p *Params) error {
		p.HelmValues = values
		return nil
	}
}
