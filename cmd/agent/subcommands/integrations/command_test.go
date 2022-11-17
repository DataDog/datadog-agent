// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python
// +build python

package integrations

import (
	"bytes"
	"io"
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

	err := pip("my/python", 0, "freeze", []string{}, stdout, stderr, newCommand)
	assert.Equal(t, nil, err)
	assert.Equal(t, "my/python", cmdMock.name)
	assert.Equal(t, []string{"-mpip", "freeze", "--disable-pip-version-check"}, cmdMock.arg)

	assert.Equal(t, cmdMock.stdout, string(stdout.Bytes()))
	assert.Equal(t, cmdMock.stderr, string(stderr.Bytes()))
}
