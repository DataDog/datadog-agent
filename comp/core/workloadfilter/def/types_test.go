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
		result := CreateProcess(nil)
		assert.Nil(t, result)
	})

	t.Run("process with all fields populated", func(t *testing.T) {
		process := &workloadmeta.Process{
			Name:    "test-proc",
			Cmdline: []string{"test-process", "--config", "/etc/config.yaml", "--verbose"},
		}

		result := CreateProcess(process)

		assert.NotNil(t, result)
		assert.NotNil(t, result.FilterProcess)
		assert.Equal(t, "test-proc", result.FilterProcess.Name)
		assert.Equal(t, "test-process --config /etc/config.yaml --verbose", result.FilterProcess.Cmdline)
		assert.Equal(t, []string{"test-process", "--config", "/etc/config.yaml", "--verbose"}, result.FilterProcess.Args)
	})

	t.Run("process with empty fields", func(t *testing.T) {
		process := &workloadmeta.Process{}

		result := CreateProcess(process)

		assert.NotNil(t, result)
		assert.NotNil(t, result.FilterProcess)
		assert.Empty(t, result.FilterProcess.Name)
		assert.Empty(t, result.FilterProcess.Cmdline)
		assert.Empty(t, result.FilterProcess.Args)
	})
}
