// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/imageresolver"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// mockRCClient is a lightweight mock that implements RemoteConfigClient
type mockRCClient struct {
	configs     map[string]state.RawConfig
	subscribers map[string]func(map[string]state.RawConfig, func(string, state.ApplyStatus))

	// For async testing
	blockGetConfigs bool
	configsReady    chan struct{}
	mu              sync.Mutex
}

// loadTestConfigFile loads a test data file and converts it to the format returned by rcClient.GetConfigs()
func loadTestConfigFile(filename string) (map[string]state.RawConfig, error) {
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		return nil, fmt.Errorf("failed to read test data file %s: %v", filename, err)
	}

	var repoConfigs map[string]imageresolver.RepositoryConfig
	if err := json.Unmarshal(data, &repoConfigs); err != nil {
		return nil, fmt.Errorf("failed to parse test data: %v", err)
	}

	// Convert each repository config to RawConfig format (as rcClient.GetConfigs() would return)
	rawConfigs := make(map[string]state.RawConfig)
	for configName, repoConfig := range repoConfigs {
		configJSON, err := json.Marshal(repoConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config %s: %v", configName, err)
		}
		rawConfigs[configName] = state.RawConfig{Config: configJSON}
	}

	return rawConfigs, nil
}

func newMockRCClient(filename string) *mockRCClient {
	rawConfigs, err := loadTestConfigFile(filename)
	if err != nil {
		panic(err)
	}

	return &mockRCClient{
		configs:         rawConfigs,
		subscribers:     make(map[string]func(map[string]state.RawConfig, func(string, state.ApplyStatus))),
		blockGetConfigs: false,
		configsReady:    make(chan struct{}),
	}
}

func (m *mockRCClient) Subscribe(product string, _ func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
	log.Debugf("Would subscribe called with product on RCClient: %s", product)
}

func (m *mockRCClient) GetConfigs(_ string) map[string]state.RawConfig {
	m.mu.Lock()
	shouldBlock := m.blockGetConfigs
	channel := m.configsReady
	m.mu.Unlock()

	if shouldBlock {
		<-channel
	}

	return m.configs
}

func TestInjectorOptions(t *testing.T) {
	i := newInjector(time.Now(), "registry", injectorWithImageTag("1", imageresolver.NewNoOpImageResolver()))
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
		injectorWithImageTag("1", imageresolver.NewNoOpImageResolver()),
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
			var resolver imageresolver.ImageResolver
			if tc.hasRemoteData {
				mockClient := newMockRCClient("image_resolver_multi_repo.json")
				resolver = imageresolver.NewImageResolver(*imageresolver.NewImageResolverConfig(config.NewMock(t), mockClient))
			} else {
				resolver = imageresolver.NewNoOpImageResolver()
			}

			i := newInjector(time.Now(), tc.registry,
				injectorWithImageTag(tc.tag, resolver),
			)

			assert.Equal(t, tc.expectedImage, i.image, tc.description)
		})
	}
}

func TestInjectorWithRemoteConfigImageResolverAfterInit(t *testing.T) {
	mockClient := newMockRCClient("image_resolver_multi_repo.json")
	resolver := imageresolver.NewImageResolver(*imageresolver.NewImageResolverConfig(config.NewMock(t), mockClient))

	assert.Eventually(t, func() bool {
		_, ok := resolver.Resolve("gcr.io/datadoghq", "apm-inject", "0")
		return ok
	}, 100*time.Millisecond, 5*time.Millisecond, "Resolver should initialize")

	i := newInjector(time.Now(), "gcr.io/datadoghq",
		injectorWithImageTag("0", resolver),
	)

	assert.Equal(t, "gcr.io/datadoghq/apm-inject@sha256:9876543210fedcba9876543210fedcba9876543210fedcba9876543210fedcba", i.image)
}
