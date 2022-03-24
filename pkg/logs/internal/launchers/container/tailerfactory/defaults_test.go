// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package tailerfactory

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/require"
)

func TestDefaultSourceAndService(t *testing.T) {
	makeContainerJSON := func(image, name string) types.ContainerJSON {
		return types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				Name:  name,
				Image: image,
			},
		}
	}

	t.Run("both already set", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				Source:     "src",
				Service:    "svc",
			}), containersorpods.LogContainers, nil, nil, nil)
		require.Equal(t, "src", source)
		require.Equal(t, "svc", service)
	})

	t.Run("service from tags", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				Source:     "src",
				// Service not set
			}), containersorpods.LogContainers,
			// inspectContainer
			func(containerID string) (types.ContainerJSON, error) {
				require.Equal(t, "abc123", containerID)
				return makeContainerJSON("img", "cname"), nil
			},
			// getServiceNameFromTags
			func(containerID, containerName string) string {
				require.Equal(t, "abc123", containerID)
				require.Equal(t, "cname", containerName)
				return "fromtags"
			},
			nil)
		require.Equal(t, "src", source)
		require.Equal(t, "fromtags", service)
	})

	t.Run("service from tags/inspect fails", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				Source:     "src",
				// Service not set
			}), containersorpods.LogContainers,
			// inspectContainer
			func(containerID string) (types.ContainerJSON, error) {
				require.Equal(t, "abc123", containerID)
				return types.ContainerJSON{}, errors.New("uhoh")
			},
			// getServiceNameFromTags
			func(containerID, containerName string) string {
				require.Equal(t, "abc123", containerID)
				require.Equal(t, "cname", containerName)
				return "fromtags"
			},
			nil)
		require.Equal(t, "src", source)
		require.Equal(t, "", service)
	})

	t.Run("service from shortName", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				Source:     "src",
				// Service not set
			}), containersorpods.LogContainers,
			// inspectContainer
			func(containerID string) (types.ContainerJSON, error) {
				require.Equal(t, "abc123", containerID)
				return makeContainerJSON("img", "cname"), nil
			},
			// getServiceNameFromTags
			func(containerID, containerName string) string {
				require.Equal(t, "abc123", containerID)
				require.Equal(t, "cname", containerName)
				return ""
			},
			// resolveImageName
			func(imageName string) (string, error) {
				require.Equal(t, "img", imageName)
				return "fromimg", nil
			})
		require.Equal(t, "src", source)
		require.Equal(t, "fromimg", service)
	})

	t.Run("service from shortName/resolve fails", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				Source:     "src",
				// Service not set
			}), containersorpods.LogContainers,
			// inspectContainer
			func(containerID string) (types.ContainerJSON, error) {
				require.Equal(t, "abc123", containerID)
				return makeContainerJSON("img", "cname"), nil
			},
			// getServiceNameFromTags
			func(containerID, containerName string) string {
				require.Equal(t, "abc123", containerID)
				require.Equal(t, "cname", containerName)
				return ""
			},
			// resolveImageName
			func(imageName string) (string, error) {
				require.Equal(t, "img", imageName)
				return "", errors.New("uhoh")
			})
		require.Equal(t, "src", source)
		require.Equal(t, "", service)
	})

	t.Run("service from shortName/split fails", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				Source:     "src",
				// Service not set
			}), containersorpods.LogContainers,
			// inspectContainer
			func(containerID string) (types.ContainerJSON, error) {
				require.Equal(t, "abc123", containerID)
				return makeContainerJSON("img", "cname"), nil
			},
			// getServiceNameFromTags
			func(containerID, containerName string) string {
				require.Equal(t, "abc123", containerID)
				require.Equal(t, "cname", containerName)
				return ""
			},
			// resolveImageName
			func(imageName string) (string, error) {
				require.Equal(t, "img", imageName)
				return "sha256:99999", nil // SplitImageName will fail on a sha256 name
			})
		require.Equal(t, "src", source)
		require.Equal(t, "", service)
	})

	t.Run("source from shortName", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				// Source not set
				Service: "svc",
			}), containersorpods.LogContainers,
			// inspectContainer
			func(containerID string) (types.ContainerJSON, error) {
				require.Equal(t, "abc123", containerID)
				return makeContainerJSON("img", "cname"), nil
			},
			nil,
			// resolveImageName
			func(imageName string) (string, error) {
				require.Equal(t, "img", imageName)
				return "fromimg", nil
			})
		require.Equal(t, "fromimg", source)
		require.Equal(t, "svc", service)
	})

	t.Run("source from shortName/inspect fails", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				// Source not set
				Service: "svc",
			}), containersorpods.LogContainers,
			// inspectContainer
			func(containerID string) (types.ContainerJSON, error) {
				require.Equal(t, "abc123", containerID)
				return types.ContainerJSON{}, errors.New("uhoh")
			},
			nil,
			nil)
		require.Equal(t, "", source)
		require.Equal(t, "svc", service)
	})

	t.Run("source from shortName/resolve fails", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				// Source not set
				Service: "svc",
			}), containersorpods.LogContainers,
			// inspectContainer
			func(containerID string) (types.ContainerJSON, error) {
				require.Equal(t, "abc123", containerID)
				return makeContainerJSON("img", "cname"), nil
			},
			nil,
			// resolveImageName
			func(imageName string) (string, error) {
				require.Equal(t, "img", imageName)
				return "", errors.New("uhoh")
			})
		require.Equal(t, "", source)
		require.Equal(t, "svc", service)
	})

	t.Run("source from shortName/split fails", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				// Source not set
				Service: "svc",
			}), containersorpods.LogContainers,
			// inspectContainer
			func(containerID string) (types.ContainerJSON, error) {
				require.Equal(t, "abc123", containerID)
				return makeContainerJSON("img", "cname"), nil
			},
			nil,
			// resolveImageName
			func(imageName string) (string, error) {
				require.Equal(t, "img", imageName)
				return "sha256:99999", nil // SplitImageName will fail on a sha256 name
			})
		require.Equal(t, "", source)
		require.Equal(t, "svc", service)
	})
}
