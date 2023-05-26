// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailerfactory

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func TestDefaultSourceAndService(t *testing.T) {
	makeContainer := func(shortName, name string) *workloadmeta.Container {
		return &workloadmeta.Container{
			EntityMeta: workloadmeta.EntityMeta{
				Name: name,
			},
			Image: workloadmeta.ContainerImage{
				ShortName: shortName,
			},
		}
	}

	t.Run("both already set", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				Source:     "src",
				Service:    "svc",
			}), containersorpods.LogContainers, nil, nil)
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
			// getContainer
			func(containerID string) (*workloadmeta.Container, error) {
				require.Equal(t, "abc123", containerID)
				return makeContainer("img", "cname"), nil
			},
			// getServiceNameFromTags
			func(containerID, containerName string) string {
				require.Equal(t, "abc123", containerID)
				require.Equal(t, "cname", containerName)
				return "fromtags"
			})
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
			// getContainer
			func(containerID string) (*workloadmeta.Container, error) {
				require.Equal(t, "abc123", containerID)
				return nil, errors.New("uhoh")
			},
			// getServiceNameFromTags
			func(containerID, containerName string) string {
				require.Equal(t, "abc123", containerID)
				require.Equal(t, "cname", containerName)
				return "fromtags"
			})
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
			// getContainer
			func(containerID string) (*workloadmeta.Container, error) {
				require.Equal(t, "abc123", containerID)
				return makeContainer("img", "cname"), nil
			},
			// getServiceNameFromTags
			func(containerID, containerName string) string {
				require.Equal(t, "abc123", containerID)
				require.Equal(t, "cname", containerName)
				return ""
			})
		require.Equal(t, "src", source)
		require.Equal(t, "img", service)
	})

	t.Run("source from shortName", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				// Source not set
				Service: "svc",
			}), containersorpods.LogContainers,
			// getContainer
			func(containerID string) (*workloadmeta.Container, error) {
				require.Equal(t, "abc123", containerID)
				return makeContainer("img", "cname"), nil
			},
			nil)
		require.Equal(t, "img", source)
		require.Equal(t, "svc", service)
	})

	t.Run("source from shortName/inspect fails", func(t *testing.T) {
		source, service := defaultSourceAndServiceInner(
			sources.NewLogSource("test", &config.LogsConfig{
				Identifier: "abc123",
				// Source not set
				Service: "svc",
			}), containersorpods.LogContainers,
			// getContainer
			func(containerID string) (*workloadmeta.Container, error) {
				require.Equal(t, "abc123", containerID)
				return nil, errors.New("uhoh")
			},
			nil)
		require.Equal(t, "", source)
		require.Equal(t, "svc", service)
	})
}
