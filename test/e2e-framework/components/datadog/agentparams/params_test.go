// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agentparams

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/stretchr/testify/assert"
)

func TestParams(t *testing.T) {
	t.Run("parseVersion should correctly parse stable version", func(t *testing.T) {
		version, err := parseVersion("7.43")
		assert.NoError(t, err)
		assert.Equal(t, version, PackageVersion{
			Major:   "7",
			Minor:   "43",
			Channel: StableChannel,
		})
	})
	t.Run("parseVersion should correctly parse rc version", func(t *testing.T) {
		version, err := parseVersion("7.45~rc.1")
		assert.NoError(t, err)
		assert.Equal(t, version, PackageVersion{
			Major:   "7",
			Minor:   "45~rc.1",
			Channel: BetaChannel,
		})
	})
	t.Run("parsePipelineVersion should correctly parse a pipeline ID and format the agent version pipeline", func(t *testing.T) {
		p := &Params{}
		options := []Option{WithPipeline("16362517")}
		result, err := common.ApplyOption(p, options)
		assert.NoError(t, err)
		assert.Equal(t, result.Version, PackageVersion{
			PipelineID: "16362517",
		})
	})
	t.Run("WithIntegration should correctly add conf.d/integration/conf.yaml to the path", func(t *testing.T) {
		p := &Params{
			Integrations: make(map[string]*FileDefinition),
			Files:        make(map[string]*FileDefinition),
		}
		options := []Option{WithIntegration("http_check", "some_config")}
		result, err := common.ApplyOption(p, options)
		assert.NoError(t, err)

		for filePath, definition := range result.Integrations {
			assert.Contains(t, filePath, "conf.d")
			assert.Contains(t, filePath, "http_check")
			assert.Contains(t, filePath, "conf.yaml")
			assert.Equal(t, definition.Content, "some_config")
		}
	})
}
