// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_script

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// credsWith constructs a PrivateCredentials whose configFileLocation token holds the given
// YAML string, matching how the credential resolver loads script config files.
func credsWith(yamlContent string) *privateconnection.PrivateCredentials {
	return &privateconnection.PrivateCredentials{
		Type: privateconnection.TokenAuthType,
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: "configFileLocation", Value: yamlContent},
		},
	}
}

// TestParseCredentials_ValidConfig verifies that a correct YAML config with the expected
// schemaId is parsed into a ScriptBundleConfig with the right scripts and commands.
func TestParseCredentials_ValidConfig(t *testing.T) {
	yaml := `
schemaId: script-credentials-v1
runPredefinedScript:
  deploy:
    command: ["./deploy.sh", "--env", "prod"]
    allowedEnvVars: ["DEPLOY_TOKEN"]
  lint:
    command: ["golangci-lint", "run"]
`
	creds := credsWith(yaml)
	cfg, err := parseCredentials(creds)

	require.NoError(t, err)
	assert.Equal(t, schemaIdV1, cfg.SchemaId)
	require.Contains(t, cfg.RunPredefinedScript, "deploy")
	assert.Equal(t, []string{"./deploy.sh", "--env", "prod"}, cfg.RunPredefinedScript["deploy"].Command)
	assert.Equal(t, []string{"DEPLOY_TOKEN"}, cfg.RunPredefinedScript["deploy"].AllowedEnvVars)
	require.Contains(t, cfg.RunPredefinedScript, "lint")
}

// TestParseCredentials_WrongSchemaId verifies that a YAML file with an unrecognised schemaId
// is rejected so that version mismatches are surfaced early (before script execution).
func TestParseCredentials_WrongSchemaId(t *testing.T) {
	yaml := `
schemaId: script-credentials-v99
runPredefinedScript: {}
`
	_, err := parseCredentials(credsWith(yaml))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected schemaId")
	assert.Contains(t, err.Error(), "script-credentials-v99")
}

// TestParseCredentials_InvalidYAML verifies that a non-YAML configFileLocation value
// returns a parse error rather than silently producing an empty/zero config.
func TestParseCredentials_InvalidYAML(t *testing.T) {
	_, err := parseCredentials(credsWith("{not: [valid: yaml"))

	require.Error(t, err)
}

// TestParseCredentials_EmptyConfigFileLocation verifies that an absent configFileLocation
// token results in a schema-ID validation error (the empty string is not the expected ID).
func TestParseCredentials_EmptyConfigFileLocation(t *testing.T) {
	creds := &privateconnection.PrivateCredentials{
		Type:   privateconnection.TokenAuthType,
		Tokens: []privateconnection.PrivateCredentialsToken{},
	}
	_, err := parseCredentials(creds)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected schemaId")
}
