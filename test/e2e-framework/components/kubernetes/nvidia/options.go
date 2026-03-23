// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package nvidia

// defaultGpuOperatorVersion is the default version of the Nvidia GPU operator to install
const defaultGpuOperatorVersion = "v24.9.2"

// defaultKindNodeImage is the default image to use for the kind nodes.
const defaultKindNodeImage = "kindest/node"

// defaultCudaSanityCheckImage is a Docker image that contains a CUDA sample to
// validate the GPU setup with the default CUDA installation. Note that the CUDA
// version in this image must be equal or less than the one installed in the
// AMI.
const defaultCudaSanityCheckImage = "docker.io/nvidia/cuda:12.6.3-base-ubuntu22.04"

// KindClusterOptions contains the options for creating a kind cluster with the Nvidia GPU operator
type KindClusterOptions struct {
	// kubeVersion is the version of Kubernetes to install in the kind cluster
	kubeVersion string

	// gpuOperatorVersion is the version of the Nvidia GPU operator to install
	gpuOperatorVersion string

	// kindImage is the image to use for the kind nodes
	kindImage string

	// cudaSanityCheckImage is a Docker image to use when performing sanity checks for validation of the GPU setup in containers
	cudaSanityCheckImage string
}

// KindClusterOption is a function that modifies a KindClusterOptions
type KindClusterOption func(*KindClusterOptions)

// WithGPUOperatorVersion sets the version of the Nvidia GPU operator to install
func WithKubeVersion(version string) KindClusterOption {
	return func(o *KindClusterOptions) {
		o.kubeVersion = version
	}
}

// WithGPUOperatorVersion sets the version of the Nvidia GPU operator to install
func WithGPUOperatorVersion(version string) KindClusterOption {
	return func(o *KindClusterOptions) {
		o.gpuOperatorVersion = version
	}
}

// WithKindImage sets the image to use for the kind nodes. The version used by this image will
// be the one defined by kubernetes.GetKindVersionConfig based on the kubernetes version used.
func WithKindImage(image string) KindClusterOption {
	return func(o *KindClusterOptions) {
		o.kindImage = image
	}
}

// WithCudaSanityCheckImage sets the image to use for the CUDA sanity check commands. Note that
// the CUDA version in this image must be equal or less than the one installed in the AMI.
func WithCudaSanityCheckImage(image string) KindClusterOption {
	return func(o *KindClusterOptions) {
		o.cudaSanityCheckImage = image
	}
}

// NewKindClusterOptions creates a new KindClusterOptions with the given options, or defaults
func NewKindClusterOptions(opts ...KindClusterOption) *KindClusterOptions {
	o := &KindClusterOptions{
		gpuOperatorVersion:   defaultGpuOperatorVersion,
		kindImage:            defaultKindNodeImage,
		cudaSanityCheckImage: defaultCudaSanityCheckImage,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
