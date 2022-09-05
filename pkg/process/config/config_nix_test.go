// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && secrets
// +build linux,secrets

package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	// This test calls ContainerProvider behind the scene, need to initialize the linux provider
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var secretScriptBuilder sync.Once

func setupSecretScript() error {
	script := "./testdata/secret"
	goCmd, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("Couldn't find golang binary in path")
	}

	buildCmd := exec.Command(goCmd, "build", "-o", script, fmt.Sprintf("%s.go", script))
	if err := buildCmd.Start(); err != nil {
		return fmt.Errorf("Couldn't build script %v: %s", script, err)
	}
	if err := buildCmd.Wait(); err != nil {
		return fmt.Errorf("Couldn't wait the end of the build for script %v: %s", script, err)
	}

	// Permissions required for the secret script
	err = os.Chmod(script, 0700)
	if err != nil {
		return err
	}

	return os.Chown(script, os.Geteuid(), os.Getgid())
}

// TestAgentConfigYamlEnc tests the secrets feature on the file TestDDAgentConfigYamlEnc
func TestAgentConfigYamlEnc(t *testing.T) {
	secretScriptBuilder.Do(func() { require.NoError(t, setupSecretScript()) })

	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	defer restoreGlobalConfig()
	// Secrets settings are initialized only once by initConfig in the agent package so we have to setup them
	config.InitConfig(config.Datadog)
	config.Datadog.Set("secret_backend_timeout", 15)
	config.Datadog.Set("secret_backend_output_max_size", 1024)

	_ = loadAgentConfigForTest(t, "./testdata/TestDDAgentConfigYamlEnc.yaml", "")
	assert.Equal(t, "secret-my_api_key", config.Datadog.GetString("api_key"))
}

// TestAgentConfigYamlEnc2 tests the secrets feature on the file TestDDAgentConfigYamlEnc2
func TestAgentConfigYamlEnc2(t *testing.T) {
	secretScriptBuilder.Do(func() { require.NoError(t, setupSecretScript()) })

	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	defer restoreGlobalConfig()
	// Secrets settings are initialized only once by initConfig in the agent package so we have to setup them
	config.InitConfig(config.Datadog)
	config.Datadog.Set("secret_backend_timeout", 15)
	config.Datadog.Set("secret_backend_output_max_size", 1024)
	_ = loadAgentConfigForTest(t, "./testdata/TestDDAgentConfigYamlEnc2.yaml", "")

	assert.Equal(t, "secret-encrypted_key", config.Datadog.GetString("api_key"))
	assert.Equal(t, "secret-burrito.com", config.Datadog.GetString("process_config.process_dd_url"))
}

func TestAgentEncryptedVariablesSecrets(t *testing.T) {
	secretScriptBuilder.Do(func() { require.NoError(t, setupSecretScript()) })

	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	defer restoreGlobalConfig()

	// Secrets settings are initialized only once by initConfig in the agent package so we have to setup them
	config.InitConfig(config.Datadog)
	config.Datadog.Set("secret_backend_timeout", 15)
	config.Datadog.Set("secret_backend_output_max_size", 1024)

	t.Setenv("DD_API_KEY", "ENC[my_api_key]")
	t.Setenv("DD_HOSTNAME", "ENC[my-host]") // Valid hostnames do not use underscores

	agentConfig := loadAgentConfigForTest(t, "./testdata/TestEnvSiteConfig-Enc.yaml", "")

	assert.Equal(t, "secret-my_api_key", config.Datadog.Get("api_key"))
	assert.Equal(t, "secret-my-host", agentConfig.HostName)
}
