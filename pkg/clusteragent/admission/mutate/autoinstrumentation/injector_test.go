// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestInjectorOptions(t *testing.T) {
	i := newInjector(time.Now(), "registry", injectorWithImageResolver(newNoOpImageResolver()), injectorWithImageTag("1"))
	require.Equal(t, "registry/apm-inject:1", i.image)
}

func TestInjectorLibRequirements(t *testing.T) {
	mutators := containerMutators{
		containerSecurityContext{
			&corev1.SecurityContext{
				AllowPrivilegeEscalation: pointer.Ptr(false),
			},
		},
	}
	i := newInjector(time.Now(), "registry",
		injectorWithImageResolver(newNoOpImageResolver()),
		injectorWithImageTag("1"),
		injectorWithLibRequirementOptions(libRequirementOptions{initContainerMutators: mutators}),
	)

	opts := i.requirements().libRequirementOptions
	require.Equal(t, 1, len(opts.initContainerMutators))

	container := corev1.Container{}
	err := opts.initContainerMutators[0].mutateContainer(&container)
	require.NoError(t, err)

	require.Equal(t, &corev1.SecurityContext{
		AllowPrivilegeEscalation: pointer.Ptr(false),
	}, container.SecurityContext)
}

func TestInjectorWithRemoteConfigImageResolver(t *testing.T) {
	testCases := []struct {
		name          string
		registry      string
		tag           string
		hasRemoteData bool
		expectedImage string
		description   string
	}{
		{
			name:          "datadog_registry_with_remote_config_during_init",
			registry:      "gcr.io/datadoghq",
			tag:           "0",
			hasRemoteData: false,
			expectedImage: "gcr.io/datadoghq/apm-inject:0",
			description:   "Should use digest from remote config for Datadog registry",
		},
		{
			name:          "datadog_registry_without_remote_config",
			registry:      "gcr.io/datadoghq",
			tag:           "0",
			hasRemoteData: false,
			expectedImage: "gcr.io/datadoghq/apm-inject:0",
			description:   "Should fallback to tag-based image when remote config unavailable",
		},
		{
			name:          "datadog_registry_unknown_tag_with_remote_config",
			registry:      "gcr.io/datadoghq",
			tag:           "unknown-tag",
			hasRemoteData: true,
			expectedImage: "gcr.io/datadoghq/apm-inject:unknown-tag",
			description:   "Should fallback to tag-based image when tag not found in remote config",
		},
		{
			name:          "custom_registry_fallback",
			registry:      "my-registry.com",
			tag:           "0",
			hasRemoteData: false,
			expectedImage: "my-registry.com/apm-inject:0",
			description:   "Should use tag-based image for non-Datadog registries",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var resolver ImageResolver
			if tc.hasRemoteData {
				mockClient := newMockRCClient("image_resolver_multi_repo.json")
				resolver = newRemoteConfigImageResolverWithRetryConfig(mockClient, 2, 1*time.Millisecond)
			} else {
				resolver = newNoOpImageResolver()
			}

			i := newInjector(time.Now(), tc.registry,
				injectorWithImageResolver(resolver),
				injectorWithImageTag(tc.tag),
			)

			assert.Equal(t, tc.expectedImage, i.image, tc.description)
		})
	}
}

func TestInjectorWithRemoteConfigImageResolverAfterInit(t *testing.T) {
	mockClient := newMockRCClient("image_resolver_multi_repo.json")
	resolver := newRemoteConfigImageResolverWithRetryConfig(mockClient, 2, 1*time.Millisecond)

	assert.Eventually(t, func() bool {
		_, ok := resolver.Resolve("gcr.io/datadoghq", "apm-inject", "0")
		return ok
	}, 100*time.Millisecond, 5*time.Millisecond, "Resolver should initialize")

	i := newInjector(time.Now(), "gcr.io/datadoghq",
		injectorWithImageResolver(resolver),
		injectorWithImageTag("0"),
	)

	assert.Equal(t, "gcr.io/datadoghq/apm-inject@sha256:inject456", i.image)
}
