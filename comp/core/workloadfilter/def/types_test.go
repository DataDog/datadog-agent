// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package workloadfilter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestCreateProcess(t *testing.T) {
	t.Run("nil process input", func(t *testing.T) {
		result := CreateProcess(nil, nil)
		assert.Nil(t, result)
	})

	t.Run("process with all fields populated", func(t *testing.T) {
		process := &workloadmeta.Process{
			Comm:    "test-proc",
			Cmdline: []string{"test-process", "--config", "/etc/config.yaml", "--verbose"},
		}

		result := CreateProcess(process, nil)

		assert.NotNil(t, result)
		assert.NotNil(t, result.FilterProcess)
		assert.Equal(t, "test-proc", result.FilterProcess.Comm)
		assert.Equal(t, []string{"test-process", "--config", "/etc/config.yaml", "--verbose"}, result.FilterProcess.Cmdline)
		assert.Nil(t, result.Owner)
	})

	t.Run("process with empty fields", func(t *testing.T) {
		process := &workloadmeta.Process{}

		result := CreateProcess(process, nil)

		assert.NotNil(t, result)
		assert.NotNil(t, result.FilterProcess)
		assert.Empty(t, result.FilterProcess.Comm)
		assert.Empty(t, result.FilterProcess.Cmdline)
		assert.Nil(t, result.Owner)
	})

	t.Run("process with container owner", func(t *testing.T) {
		container := CreateContainer(
			&workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					ID: "container123",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: "test-container",
				},
				Image: workloadmeta.ContainerImage{
					RawName: "nginx:latest",
				},
			},
			nil,
		)

		process := &workloadmeta.Process{
			Comm:        "nginx",
			Cmdline:     []string{"nginx", "-g", "daemon off;"},
			ContainerID: "container123",
		}

		result := CreateProcess(process, container)

		assert.NotNil(t, result)
		assert.NotNil(t, result.FilterProcess)
		assert.Equal(t, "nginx", result.FilterProcess.Comm)
		assert.Equal(t, []string{"nginx", "-g", "daemon off;"}, result.FilterProcess.Cmdline)
		assert.Equal(t, container, result.Owner)

		// Verify owner relationship is set in the FilterProcess
		assert.NotNil(t, result.FilterProcess.Owner)
		containerOwner := result.FilterProcess.GetContainer()
		assert.NotNil(t, containerOwner)
		assert.Equal(t, "test-container", containerOwner.Name)
		assert.Equal(t, "container123", containerOwner.Id)
		assert.Equal(t, "nginx:latest", containerOwner.Image)
	})
}
