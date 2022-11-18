// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python
// +build python

package integrations

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestInstallCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "install", "foo==1.0", "-v"},
		install,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"foo==1.0"}, cliParams.args)
			require.Equal(t, 1, cliParams.verbose)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
			require.Equal(t, true, coreParams.ConfigMissingOK)
		})
}

func TestRemoveCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "remove", "foo"},
		remove,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"foo"}, cliParams.args)
			require.Equal(t, 0, cliParams.verbose)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
			require.Equal(t, true, coreParams.ConfigMissingOK)
		})
}

func TestFreezeCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "freeze"},
		list,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{}, cliParams.args)
			require.Equal(t, 0, cliParams.verbose)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
			require.Equal(t, true, coreParams.ConfigMissingOK)
		})
}

func TestShowCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "show", "foo"},
		show,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"foo"}, cliParams.args)
			require.Equal(t, 0, cliParams.verbose)
			require.Equal(t, false, coreParams.ConfigLoadSecrets)
			require.Equal(t, true, coreParams.ConfigMissingOK)
		})
}

func TestCliParamsFallbackVersion(t *testing.T) {
	tests := map[string]struct {
		cp              cliParams
		configValue     string
		expectedVersion string
	}{
		"falls back to config": {cliParams{}, "2", "2"},
		"overrides config":     {cliParams{pythonMajorVersion: "3"}, "2", "3"},
	}
	for name, test := range tests {
		t.Logf("Running test %s", name)
		test.cp.fallbackVersion(test.configValue)
		assert.Equal(t, test.expectedVersion, test.cp.pythonMajorVersion)
	}
}

// A minimal mock for a command that lets us control its output and make assertions
type CmdMock struct {
	name             string
	arg              []string
	stdout           string
	stderr           string
	stdoutPipeWriter *io.PipeWriter
	stderrPipeWriter *io.PipeWriter
	stdoutPipeReader *io.PipeReader
	stderrPipeReader *io.PipeReader
	env              []string
}

func (c *CmdMock) Output() ([]byte, error) {
	return []byte(c.stdout), nil
}

func (c *CmdMock) Start() error {
	go func() {
		c.stdoutPipeWriter.Write([]byte(c.stdout))
		c.stdoutPipeWriter.Close()
	}()
	go func() {
		c.stderrPipeWriter.Write([]byte(c.stderr))
		c.stderrPipeWriter.Close()
	}()
	return nil
}

func (c *CmdMock) StderrPipe() (io.ReadCloser, error) {
	c.stderrPipeReader, c.stderrPipeWriter = io.Pipe()
	return c.stderrPipeReader, nil
}

func (c *CmdMock) StdoutPipe() (io.ReadCloser, error) {
	c.stdoutPipeReader, c.stdoutPipeWriter = io.Pipe()
	return c.stdoutPipeReader, nil
}

func (c *CmdMock) Wait() error {
	// Cmd.Wait is supposed to close existing pipes
	c.stdoutPipeReader.Close()
	c.stderrPipeReader.Close()
	return nil
}

func (c *CmdMock) SetEnv(newEnv []string) {
	c.env = newEnv
}

func TestPip(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	cmdMock := &CmdMock{
		stdout: "This is the part that went well...\n",
		stderr: "...And this is the error\n",
	}

	newCommand := func(name string, arg ...string) commandRunner {
		cmdMock.name = name
		cmdMock.arg = arg
		return cmdMock
	}

	py := python(defaultPythonPath().py3)
	py.stdout = stdout
	py.stderr = stderr
	py.cmdConstructor = newCommand

	err := pip(py, "freeze", []string{}, 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, defaultPythonPath().py3, cmdMock.name)
	assert.Equal(t, []string{"-m", "pip", "freeze", "--disable-pip-version-check"}, cmdMock.arg)

	assert.Equal(t, cmdMock.stdout, string(stdout.Bytes()))
	assert.Equal(t, cmdMock.stderr, string(stderr.Bytes()))
}

func TestDownloadWheel(t *testing.T) {
	stderr := new(bytes.Buffer)
	stdout := new(bytes.Buffer)
	tempdir := t.TempDir()
	packagePath := filepath.Join(tempdir, "package", "path.whl")
	os.MkdirAll(filepath.Dir(packagePath), 0777)
	f, err := os.Create(packagePath)
	if err != nil {
		t.Errorf("failed to create: %s", err)
		return
	}
	f.Close()

	cmdMock := &CmdMock{
		stdout: fmt.Sprintf("%s\n", packagePath),
		stderr: "...And this is the error\n",
	}

	newCommand := func(name string, arg ...string) commandRunner {
		cmdMock.name = name
		cmdMock.arg = arg
		return cmdMock
	}

	py := python(defaultPythonPath().py3)
	py.stdout = stdout
	py.stderr = stderr
	py.cmdConstructor = newCommand

	wheelPath, err := downloadWheel(py, "datadog-integration", "3.1.4", "core", 0)
	assert.Equal(t, nil, err)
	assert.Equal(t, defaultPythonPath().py3, cmdMock.name)
	assert.Equal(t, []string{"-m", downloaderModule, "datadog-integration", "--version", "3.1.4", "--type", "core"}, cmdMock.arg)

	assert.Equal(t, cmdMock.stdout, string(stdout.Bytes()))
	assert.Equal(t, cmdMock.stderr, string(stderr.Bytes()))
	assert.Equal(t, packagePath, wheelPath)

	// Test that we get an error when the downloader exists but we can't find the wheel
	packagePath = filepath.Join(tempdir, "non-existing-wheel")
	cmdMock.stdout = fmt.Sprintf("%s\n", packagePath)
	_, err = downloadWheel(py, "datadog-integration", "3.1.4", "core", 0)
	assert.Equal(t, fmt.Sprintf("wheel %s does not exist", packagePath), err.Error())
}

func TestInstalledVersion(t *testing.T) {
	cmdMock := &CmdMock{}
	cmdMock.stdout = "3.1.4"

	newCommand := func(name string, arg ...string) commandRunner {
		cmdMock.name = name
		cmdMock.arg = arg
		return cmdMock
	}

	py := python(defaultPythonPath().py3)
	py.cmdConstructor = newCommand

	version, found, err := installedVersion(py, "datadog-integration")
	assert.Equal(t, "3.1.4", version.String())
	assert.Equal(t, true, found)
	assert.Nil(t, err)
}
