// +build linux

package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestAgentConfigYamlEnc(t *testing.T) {
	secretScriptBuilder.Do(func() { require.NoError(t, setupSecretScript()) })

	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	defer restoreGlobalConfig()
	// Secrets settings are initialized only once by initConfig in the agent package so we have to setup them
	config.Datadog.Set("secret_backend_timeout", 15)
	config.Datadog.Set("secret_backend_output_max_size", 1024)

	assert := assert.New(t)

	agentConfig, err := NewAgentConfig(
		"test",
		"./testdata/TestDDAgentConfigYamlEnc.yaml",
		"",
	)
	assert.NoError(err)

	ep := agentConfig.APIEndpoints[0]
	assert.Equal("secret_my_api_key", ep.APIKey)
}

func TestAgentConfigYamlEnc2(t *testing.T) {
	secretScriptBuilder.Do(func() { require.NoError(t, setupSecretScript()) })

	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	defer restoreGlobalConfig()
	// Secrets settings are initialized only once by initConfig in the agent package so we have to setup them
	config.Datadog.Set("secret_backend_timeout", 15)
	config.Datadog.Set("secret_backend_output_max_size", 1024)
	assert := assert.New(t)
	agentConfig, err := NewAgentConfig(
		"test",
		"./testdata/TestDDAgentConfigYamlEnc2.yaml",
		"",
	)
	assert.NoError(err)

	ep := agentConfig.APIEndpoints[0]
	assert.Equal("secret_encrypted_key", ep.APIKey)
	assert.Equal("secret_burrito.com", ep.Endpoint.String())
}
