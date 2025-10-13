package tests

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var workingDir = flag.String("workingDir", "", "run tests in the specified directory, relative to the root of the repository. Defaults to the root of the repository.")

func TestInvokes(t *testing.T) {
	// Set working directory
	rootPath, err := rootPath()
	require.NoError(t, err)
	if *workingDir == "" {
		*workingDir = rootPath
	}
	if !filepath.IsAbs(*workingDir) {
		*workingDir = filepath.Join(rootPath, *workingDir)
	}
	t.Logf("Running tests in %s", *workingDir)

	// Arrange
	t.Log("Creating temporary configuration file")
	tmpConfigFile, err := createTemporaryConfigurationFile()
	require.NoError(t, err, "Error writing temporary configuration")
	t.Cleanup(func() {
		t.Log("Cleaning up temporary configuration file")
		os.Remove(tmpConfigFile)
	})

	t.Log("setup test infra")
	err = setupTestInfra(tmpConfigFile)
	require.NoError(t, err, "Error setting up test infra")

	tmpConfig, err := LoadConfig(tmpConfigFile)
	require.NoError(t, err)

	require.NotEmpty(t, tmpConfig.ConfigParams.AWS.TeamTag)

	// Subtests

	t.Run("az.create-vm", func(t *testing.T) {
		t.Parallel()
		testAzureInvokeVM(t, tmpConfigFile, *workingDir)
	})

	t.Run("aws.create-vm", func(t *testing.T) {
		t.Parallel()
		testAwsInvokeVM(t, tmpConfigFile, *workingDir)
	})

	t.Run("gcp.create-vm", func(t *testing.T) {
		t.Parallel()
		testGcpInvokeVM(t, tmpConfigFile, *workingDir)
	})

	t.Run("aws.invoke-docker-vm", func(t *testing.T) {
		t.Parallel()
		testInvokeDockerVM(t, tmpConfigFile, *workingDir)
	})

	t.Run("aws.invoke-kind", func(t *testing.T) {
		t.Parallel()
		testInvokeKind(t, tmpConfigFile, *workingDir)
	})

	t.Run("invoke-kind-operator", func(t *testing.T) {
		t.Parallel()
		testInvokeKindOperator(t, tmpConfigFile, *workingDir)
	})
}

func testAzureInvokeVM(t *testing.T, tmpConfigFile string, workingDirectory string) {
	t.Helper()

	stackName := fmt.Sprintf("az-invoke-vm-%s", os.Getenv("CI_JOB_ID"))
	stackName = sanitizeStackName(stackName)

	t.Log("creating vm")
	createCmd := exec.Command("invoke", "az.create-vm", "--no-interactive", "--stack-name", stackName, "--config-path", tmpConfigFile, "--no-add-known-host")
	createCmd.Dir = workingDirectory
	createOutput, err := createCmd.Output()
	assert.NoError(t, err, "Error found creating vm: %s", string(createOutput))

	t.Log("destroying vm")
	destroyCmd := exec.Command("invoke", "az.destroy-vm", "--no-clean-known-hosts", "--stack-name", stackName, "--config-path", tmpConfigFile)
	destroyCmd.Dir = workingDirectory
	destroyOutput, err := destroyCmd.Output()
	require.NoError(t, err, "Error found destroying stack: %s", string(destroyOutput))
}

func testAwsInvokeVM(t *testing.T, tmpConfigFile string, workingDirectory string) {
	t.Helper()

	stackName := fmt.Sprintf("aws-invoke-vm-%s", os.Getenv("CI_JOB_ID"))
	stackName = sanitizeStackName(stackName)

	t.Log("creating vm")
	createCmd := exec.Command("invoke", "aws.create-vm", "--no-interactive", "--stack-name", stackName, "--config-path", tmpConfigFile, "--use-fakeintake", "--no-add-known-host")
	createCmd.Dir = workingDirectory
	createOutput, err := createCmd.Output()
	assert.NoError(t, err, "Error found creating vm: %s", string(createOutput))

	t.Log("destroying vm")
	destroyCmd := exec.Command("invoke", "aws.destroy-vm", "--no-clean-known-hosts", "--stack-name", stackName, "--config-path", tmpConfigFile)
	destroyCmd.Dir = workingDirectory
	destroyOutput, err := destroyCmd.Output()
	require.NoError(t, err, "Error found destroying stack: %s", string(destroyOutput))
}

func testGcpInvokeVM(t *testing.T, tmpConfigFile string, workingDirectory string) {
	t.Helper()

	stackName := fmt.Sprintf("gcp-invoke-vm-%s", os.Getenv("CI_JOB_ID"))
	stackName = sanitizeStackName(stackName)

	t.Log("creating vm")
	createCmd := exec.Command("invoke", "gcp.create-vm", "--no-interactive", "--stack-name", stackName, "--config-path", tmpConfigFile, "--use-fakeintake", "--no-add-known-host")
	createCmd.Dir = workingDirectory
	createOutput, err := createCmd.Output()
	assert.NoError(t, err, "Error found creating vm: %s", string(createOutput))

	t.Log("destroying vm")
	destroyCmd := exec.Command("invoke", "gcp.destroy-vm", "--no-clean-known-hosts", "--stack-name", stackName, "--config-path", tmpConfigFile)
	destroyCmd.Dir = workingDirectory
	destroyOutput, err := destroyCmd.Output()
	require.NoError(t, err, "Error found destroying stack: %s", string(destroyOutput))
}

func testInvokeDockerVM(t *testing.T, tmpConfigFile string, workingDirectory string) {
	t.Helper()
	stackName := fmt.Sprintf("invoke-docker-vm-%s", os.Getenv("CI_JOB_ID"))
	stackName = sanitizeStackName(stackName)
	t.Log("creating vm with docker")
	var stdOut, stdErr bytes.Buffer

	createCmd := exec.Command("invoke", "aws.create-docker", "--no-interactive", "--stack-name", stackName, "--config-path", tmpConfigFile, "--use-fakeintake", "--use-loadBalancer")
	createCmd.Dir = workingDirectory
	createCmd.Stdout = &stdOut
	createCmd.Stderr = &stdErr
	err := createCmd.Run()
	assert.NoError(t, err, "Error found creating docker vm.\n   stdout: %s\n   stderr: %s", stdOut.String(), stdErr.String())

	stdOut.Reset()
	stdErr.Reset()

	t.Log("destroying vm with docker")
	destroyCmd := exec.Command("invoke", "destroy-docker", "--stack-name", stackName, "--config-path", tmpConfigFile)
	destroyCmd.Dir = workingDirectory
	destroyCmd.Stdout = &stdOut
	destroyCmd.Stderr = &stdErr
	err = destroyCmd.Run()
	require.NoError(t, err, "Error found destroying stack.\n   stdout: %s\n   stderr: %s", stdOut.String(), stdErr.String())
}

func testInvokeKind(t *testing.T, tmpConfigFile string, workingDirectory string) {
	t.Helper()
	stackParts := []string{"invoke", "kind"}
	if os.Getenv("CI") == "true" {
		stackParts = append(stackParts, os.Getenv("CI_JOB_ID"))
	}
	stackName := strings.Join(stackParts, "-")
	stackName = sanitizeStackName(stackName)
	t.Log("creating kind cluster")
	createCmd := exec.Command("invoke", "create-kind", "--no-interactive", "--stack-name", stackName, "--config-path", tmpConfigFile, "--use-fakeintake", "--use-loadBalancer")
	createCmd.Dir = workingDirectory
	createOutput, err := createCmd.Output()
	assert.NoError(t, err, "Error found creating kind cluster: %s", string(createOutput))

	t.Log("destroying kind cluster")
	destroyCmd := exec.Command("invoke", "destroy-kind", "--stack-name", stackName, "--config-path", tmpConfigFile)
	destroyCmd.Dir = workingDirectory
	destroyOutput, err := destroyCmd.Output()
	require.NoError(t, err, "Error found destroying kind cluster: %s", string(destroyOutput))
}

func testInvokeKindOperator(t *testing.T, tmpConfigFile string, workingDirectory string) {
	t.Helper()
	stackName := "invoke-kind-with-operator"
	if os.Getenv("CI") == "true" {
		stackName = fmt.Sprintf("%s-%s", stackName, os.Getenv("CI_JOB_ID"))
	}
	stackName = sanitizeStackName(stackName)
	t.Log("creating kind cluster with operator")
	createCmd := exec.Command("invoke", "aws.create-kind", "--install-agent-with-operator", "true", "--no-interactive", "--stack-name", stackName, "--config-path", tmpConfigFile, "--use-fakeintake", "--use-loadBalancer")
	createCmd.Dir = workingDirectory
	createOutput, err := createCmd.Output()
	assert.NoError(t, err, "Error found creating kind cluster: %s; %s", string(createOutput), err)

	t.Log("destroying kind cluster with operator")
	destroyCmd := exec.Command("invoke", "destroy-kind", "--stack-name", stackName, "--config-path", tmpConfigFile)
	destroyCmd.Dir = workingDirectory
	destroyOutput, err := destroyCmd.Output()
	require.NoError(t, err, "Error found destroying kind cluster: %s", string(destroyOutput))
}

//go:embed testfixture/config.yaml
var testInfraTestConfig string

func createTemporaryConfigurationFile() (string, error) {
	tmpConfigFile := filepath.Join(os.TempDir(), "test-infra-test.yaml")

	isCI, err := strconv.ParseBool(os.Getenv("CI"))
	account := "agent-qa"
	keyPairName := os.Getenv("E2E_KEY_PAIR_NAME")
	publicKeyPath := os.Getenv("E2E_PUBLIC_KEY_PATH")
	if err != nil || !isCI {
		// load local config
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		localConfig, err := LoadConfig(filepath.Join(homeDir, ".test_infra_config.yaml"))
		if err != nil {
			return "", err
		}
		account = localConfig.ConfigParams.AWS.Account
		keyPairName = localConfig.ConfigParams.AWS.KeyPairName
		publicKeyPath = localConfig.ConfigParams.AWS.PublicKeyPath
	}
	testInfraTestConfig = strings.ReplaceAll(testInfraTestConfig, "KEY_PAIR_NAME", keyPairName)
	testInfraTestConfig = strings.ReplaceAll(testInfraTestConfig, "PUBLIC_KEY_PATH", publicKeyPath)
	testInfraTestConfig = strings.ReplaceAll(testInfraTestConfig, "ACCOUNT", account)
	err = os.WriteFile(tmpConfigFile, []byte(testInfraTestConfig), 0644)
	return tmpConfigFile, err
}

func setupTestInfra(tmpConfigFile string) error {
	var setupStdout, setupStderr bytes.Buffer

	setupCmd := exec.Command("invoke", "setup", "--no-interactive", "--config-path", tmpConfigFile)
	setupCmd.Stdout = &setupStdout
	setupCmd.Stderr = &setupStderr

	setupCmd.Dir = "../"
	err := setupCmd.Run()
	if err != nil {
		return fmt.Errorf("stdout: %s\n%s, %v", setupStdout.String(), setupStderr.String(), err)
	}
	return nil
}

func rootPath() (string, error) {
	path, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(path)), nil
}

func sanitizeStackName(s string) string {
	return strings.Map(
		func(r rune) rune {
			// valid values are alphanumeric and hyphen
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || unicode.IsDigit(r) || r == '-' {
				return r
			}
			// drop invalid runes
			return -1
		},
		s,
	)
}
