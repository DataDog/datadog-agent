// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package docker

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

const (
	// DefaultTimeout is the default timeout for running a server.
	DefaultTimeout = time.Minute

	// DefaultRetries is the default number of retries for starting a container/s.
	DefaultRetries = 3
)

// EmptyEnv is a sugar syntax for empty environment variables
var EmptyEnv []string

type commandType string

const (
	dockerCommand commandType = "docker"
	// we are using old v1 docker-compose command because our CI doesn't support docker cli v2 yet
	composeCommand commandType = "docker-compose"
	runCommand     commandType = "run"
	removeCommand  commandType = "rm"
)

type subCommandType int

const (
	start = iota
	kill
)

// Compile-time interface compliance check
var _ LifecycleConfig = (*runConfig)(nil)
var _ LifecycleConfig = (*composeConfig)(nil)

// LifecycleConfig is an interface for the common configuration of a container lifecycle.
type LifecycleConfig interface {
	Timeout() time.Duration
	Retries() int
	PatternScanner() *testutil.PatternScanner
	Env() []string
	Name() string
	command() string
	commandArgs(t subCommandType) []string
}

// Timeout returns the timeout to be used when running a container/s
func (b baseConfig) Timeout() time.Duration {
	return b.timeout
}

// Retries returns the number of retries to be used when trying to start the container/s
func (b baseConfig) Retries() int {
	return b.retries
}

// PatternScanner returns the patternScanner object used to match logs for readiness and completion of the target container/s
func (b baseConfig) PatternScanner() *testutil.PatternScanner {
	return b.patternScanner
}

// Env returns the environment variables to set for the container/s
func (b baseConfig) Env() []string {
	return b.env
}

// Name returns the name of the docker container or a friendly name for the docker-compose setup
func (b baseConfig) Name() string {
	return b.name
}

// baseConfig contains shared configurations for both Docker and Docker Compose.
type baseConfig struct {
	name           string                   // Container name for docker or an alias for docker-compose
	timeout        time.Duration            // Timeout for the entire operation.
	retries        int                      // Number of retries for starting.
	patternScanner *testutil.PatternScanner // Used to monitor container logs for known patterns.
	env            []string                 // Environment variables to set.
}

// runConfig contains specific configurations for Docker containers, embedding BaseConfig.
type runConfig struct {
	baseConfig                   // Embed general configuration.
	ImageName  string            // Docker image to use.
	Binary     string            // Binary to run inside the container.
	BinaryArgs []string          // Arguments for the binary.
	Mounts     map[string]string // Mounts (host path -> container path).
}

func (r runConfig) command() string {
	return string(dockerCommand)
}

func (r runConfig) commandArgs(t subCommandType) []string {
	var args []string
	switch t {
	case start:
		// we want to remove the container after usage, as it is a temporary container for a particular test
		args = []string{string(runCommand), "--rm"}

		// Add mounts
		for hostPath, containerPath := range r.Mounts {
			args = append(args, "-v", fmt.Sprintf("%s:%s", hostPath, containerPath))
		}

		// Pass environment variables to the container as docker args
		for _, env := range r.Env() {
			args = append(args, "-e", env)
		}

		//append container name and container image name
		args = append(args, "--name", r.Name(), r.ImageName)

		//provide main binary and binary arguments to run inside the docker container
		args = append(args, r.Binary)
		args = append(args, r.BinaryArgs...)
	case kill:
		args = []string{string(removeCommand), "-f", r.Name(), "--volumes"}
	}
	return args
}

// composeConfig contains specific configurations for Docker Compose, embedding BaseConfig.
type composeConfig struct {
	baseConfig        // Embed general configuration.
	File       string // Path to the docker-compose file.
}

func (c composeConfig) command() string {
	return string(composeCommand)
}

func (c composeConfig) commandArgs(t subCommandType) []string {
	switch t {
	case start:
		return []string{"-f", c.File, "up", "--remove-orphans", "-V"}
	case kill:
		return []string{"-f", c.File, "down", "--remove-orphans", "--volumes"}
	default:
		return nil
	}
}

func createBaseConfig(name string, timeout time.Duration, retries int, patternScanner *testutil.PatternScanner, env []string) baseConfig {
	return baseConfig{
		name:           name,
		timeout:        timeout,
		retries:        retries,
		patternScanner: patternScanner,
		env:            env,
	}
}

// NewRunConfig creates a new runConfig instance for a single docker container.
func NewRunConfig(name string, timeout time.Duration, retries int, patternScanner *testutil.PatternScanner, env []string, imageName, binary string, binaryArgs []string, mounts map[string]string) LifecycleConfig {
	return runConfig{
		baseConfig: createBaseConfig(name, timeout, retries, patternScanner, env),
		ImageName:  imageName,
		Binary:     binary,
		BinaryArgs: binaryArgs,
		Mounts:     mounts,
	}
}

// NewComposeConfig creates a new composeConfig instance for the docker-compose.
func NewComposeConfig(name string, timeout time.Duration, retries int, patternScanner *testutil.PatternScanner, env []string, file string) LifecycleConfig {
	return composeConfig{
		baseConfig: createBaseConfig(name, timeout, retries, patternScanner, env),
		File:       file,
	}
}
