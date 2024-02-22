// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockedContainer struct {
	containerd.Container
	mockID    func() string
	mockImage func() (containerd.Image, error)
}

func (m *mockedContainer) ID() string {
	return m.mockID()
}

// Image is from the containerd.Container interface
func (m *mockedContainer) Image(context.Context) (containerd.Image, error) {
	return m.mockImage()
}

type mockedImage struct {
	containerd.Image
	mockName   func() string
	mockConfig func() (ocispec.Descriptor, error)
}

func (m *mockedImage) Config(_ context.Context) (ocispec.Descriptor, error) {
	return m.mockConfig()
}

func TestBuildWorkloadMetaContainer(t *testing.T) {
	namespace := "default"
	containerID := "10"
	labels := map[string]string{
		"some_label": "some_val",
	}
	imgName := "datadog/agent:7"
	envVarStrs := []string{
		"test_env=test_val",
	}
	envVars := map[string]string{}
	for _, s := range envVarStrs {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) < 2 {
			continue
		}
		envVars[parts[0]] = parts[1]
	}
	hostName := "test_hostname"
	createdAt, err := time.Parse("2006-01-02", "2021-10-11")
	assert.NoError(t, err)

	image := &mockedImage{
		mockConfig: func() (ocispec.Descriptor, error) {
			return ocispec.Descriptor{Digest: "my_image_id"}, nil
		},
	}
	container := mockedContainer{
		mockID: func() string {
			return containerID
		},
		mockImage: func() (containerd.Image, error) {
			return image, nil
		},
	}

	client := fake.MockedContainerdClient{
		MockInfo: func(namespace string, ctn containerd.Container) (containers.Container, error) {
			return containers.Container{
				Labels:    labels,
				CreatedAt: createdAt,
				Image:     imgName,
				Runtime: containers.RuntimeInfo{
					Name: "io.containerd.kata-qemu.v2",
				},
			}, nil
		},
		MockSpec: func(namespace string, ctn containers.Container) (*oci.Spec, error) {
			return &oci.Spec{Hostname: hostName, Process: &specs.Process{Env: envVarStrs}}, nil
		},
		MockStatus: func(namespace string, ctn containerd.Container) (containerd.ProcessStatus, error) {
			return containerd.Running, nil
		},
		MockTaskPids: func(namespace string, ctn containerd.Container) ([]containerd.ProcessInfo, error) {
			return nil, nil
		},
	}

	// Create a workload meta global store containing image metadata
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(context.Background()),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))
	imageMetadata := &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   "my_image_id",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "datadog/agent",
		},
		RepoDigests: []string{
			"gcr.io/datadoghq/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
		},
	}
	workloadmetaStore.Set(imageMetadata)

	result, err := buildWorkloadMetaContainer(namespace, &container, &client, workloadmetaStore)
	assert.NoError(t, err)

	expected := workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "", // Not available
			Labels: labels,
		},
		Image: workloadmeta.ContainerImage{
			RawName:    "datadog/agent:7",
			Name:       "datadog/agent",
			ShortName:  "agent",
			Tag:        "7",
			ID:         "my_image_id",
			RepoDigest: "sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
		},
		EnvVars:       envVars,
		Ports:         nil, // Not available
		Runtime:       workloadmeta.ContainerRuntimeContainerd,
		RuntimeFlavor: workloadmeta.ContainerRuntimeFlavorKata,
		State: workloadmeta.ContainerState{
			Running:    true,
			Status:     workloadmeta.ContainerStatusRunning,
			StartedAt:  createdAt,
			CreatedAt:  createdAt,
			FinishedAt: time.Time{}, // Not available
		},
		NetworkIPs: make(map[string]string), // Not available
		Hostname:   hostName,
		PID:        0, // Not available
	}
	assert.Equal(t, expected, result)
}

func TestExtractRuntimeFlavor(t *testing.T) {
	tests := []struct {
		name     string
		runtime  string
		expected workloadmeta.ContainerRuntimeFlavor
	}{
		{
			name:     "kata",
			runtime:  "io.containerd.kata.v2",
			expected: workloadmeta.ContainerRuntimeFlavorKata,
		},
		{
			name:     "kata-qemu",
			runtime:  "io.containerd.kata-qemu.v2",
			expected: workloadmeta.ContainerRuntimeFlavorKata,
		},
		{
			name:     "non-kata",
			runtime:  "io.containerd.runc.v2",
			expected: workloadmeta.ContainerRuntimeFlavorDefault,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRuntimeFlavor(tt.runtime)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRepoDigest(t *testing.T) {
	for _, tc := range []struct {
		repoDigest string
		registry   string
		repository string
		digest     string
	}{
		{
			repoDigest: "727006795293.dkr.ecr.us-east-1.amazonaws.com/spidly@sha256:fce79f86f7a3b9c660112da8484a8f5858a7da9e703892ba04c6f045da714300",
			registry:   "727006795293.dkr.ecr.us-east-1.amazonaws.com",
			repository: "spidly",
			digest:     "sha256:fce79f86f7a3b9c660112da8484a8f5858a7da9e703892ba04c6f045da714300",
		},
		{
			repoDigest: "docker.io/library/docker@sha256:b813c414ee36b8a2c44b45295698df6bdc3bdee4a435481dbb892e1b44e09d3b",
			registry:   "docker.io",
			repository: "library/docker",
			digest:     "sha256:b813c414ee36b8a2c44b45295698df6bdc3bdee4a435481dbb892e1b44e09d3b",
		},
		{
			repoDigest: "eu.gcr.io/datadog-staging/logs-event-store-api@sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
			registry:   "eu.gcr.io",
			repository: "datadog-staging/logs-event-store-api",
			digest:     "sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
		},
		{
			repoDigest: "registry.ddbuild.io/apm-integrations-testing/handmade/postgres",
			registry:   "registry.ddbuild.io",
			repository: "apm-integrations-testing/handmade/postgres",
			digest:     "",
		},
		{
			repoDigest: "registry.ddbuild.io/apm-integrations-testing/handmade/postgres@sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
			registry:   "registry.ddbuild.io",
			repository: "apm-integrations-testing/handmade/postgres",
			digest:     "sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
		},
		{
			repoDigest: "@sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
			registry:   "",
			repository: "",
			digest:     "sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
		},
		{
			repoDigest: "docker.io@sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
			registry:   "docker.io",
			repository: "",
			digest:     "sha256:747bd4fc36f3f263b5dcb9df907b98489a4cb46d636c223dc29f1fb7f9405070",
		},
	} {
		registry, repository, digest := parseRepoDigest(tc.repoDigest)
		assert.Equal(t, tc.registry, registry)
		assert.Equal(t, tc.repository, repository)
		assert.Equal(t, tc.digest, digest)
	}
}
