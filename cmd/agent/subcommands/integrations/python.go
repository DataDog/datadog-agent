// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python
// +build python

package integrations

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/fatih/color"
)

type pythonRunner struct {
	path           string
	stdout         io.Writer
	stderr         io.Writer
	cmdConstructor commandConstructor
	env            []string
}

func python(path string) *pythonRunner {
	return &pythonRunner{
		path:           path,
		stdout:         os.Stdout,
		stderr:         os.Stderr,
		cmdConstructor: execCommand,
	}
}

func (p *pythonRunner) withEnv(newEnv []string) *pythonRunner {
	newRunner := *p
	newRunner.env = newEnv
	return &newRunner
}

// Runs a python module with the interpreter, piping to stdout and stderr
func (p *pythonRunner) runModule(module string, args []string) error {
	return p.runModuleWithCallback(module, args, func(_s string) {})
}

// Runs a python module with the interpreter, calling a callback on every stdout line
func (p *pythonRunner) runModuleWithCallback(module string, args []string, callback func(string)) error {
	args = append([]string{"-m", module}, args...)
	pipCmd := p.cmdConstructor(p.path, args...)
	pipCmd.SetEnv(p.env)

	// Create a waitgroup for waiting for piping goroutines
	var wg sync.WaitGroup

	// forward the standard output to stdout
	pipStdout, err := pipCmd.StdoutPipe()
	if err != nil {
		return err
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		in := bufio.NewScanner(pipStdout)
		for in.Scan() {
			line := in.Text()
			fmt.Fprintf(p.stdout, "%s\n", line)
			callback(line)
		}
	}()

	// forward the standard error to stderr
	pipStderr, err := pipCmd.StderrPipe()
	if err != nil {
		return err
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		in := bufio.NewScanner(pipStderr)
		for in.Scan() {
			fmt.Fprintf(p.stderr, "%s\n", color.RedString(in.Text()))
		}
	}()

	if err = pipCmd.Start(); err != nil {
		wg.Wait()
		return fmt.Errorf("error running command: %v", err)
	}

	// Wait for both piping goroutines to complete to ensure pipes are exhausted
	wg.Wait()

	err = pipCmd.Wait()
	if err != nil {
		return fmt.Errorf("error running command: %v", err)
	}

	return nil
}

// Runs a python command (python literal program) and returns its output
func (p *pythonRunner) runCommand(cmd string) (string, error) {
	pythonCmd := p.cmdConstructor(p.path, "-c", cmd)
	output, err := pythonCmd.Output()

	if err != nil {
		errMsg := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			errMsg = string(exitErr.Stderr)
		} else {
			errMsg = err.Error()
		}

		return "", fmt.Errorf("error executing python: %s", errMsg)
	}

	return string(output), nil
}

// Holds paths for both versions of Python
type pythonPath struct {
	py2 string
	py3 string
}

func defaultPythonPath() pythonPath {
	return pythonPath{
		py2: filepath.Join(rootDir, getRelPyPath("2")),
		py3: filepath.Join(rootDir, getRelPyPath("3")),
	}
}

func sysPythonPath() pythonPath {
	return pythonPath{
		py2: pythonBin,
		py3: pythonBin,
	}
}
