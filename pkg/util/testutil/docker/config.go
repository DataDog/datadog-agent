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

	// MinimalDockerImage is the minimal docker image, just used for running a binary
	MinimalDockerImage = "alpine:3.20.3"
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
func (b BaseConfig) Timeout() time.Duration {
	return b.timeout
}

// Retries returns the number of retries to be used when trying to start the container/s
func (b BaseConfig) Retries() int {
	return b.retries
}

// PatternScanner returns the patternScanner object used to match logs for readiness and completion of the target container/s
func (b BaseConfig) PatternScanner() *testutil.PatternScanner {
	return b.patternScanner
}

// Env returns the environment variables to set for the container/s
func (b BaseConfig) Env() []string {
	return b.env
}

// Name returns the name of the docker container or a friendly name for the docker-compose setup
func (b BaseConfig) Name() string {
	return b.name
}

// BaseConfig contains shared configurations for both Docker and Docker Compose.
type BaseConfig struct {
	name           string                   // Container name for docker or an alias for docker-compose
	timeout        time.Duration            // Timeout for the entire operation.
	retries        int                      // Number of retries for starting.
	patternScanner *testutil.PatternScanner // Used to monitor container logs for known patterns.
	env            []string                 // Environment variables to set.
}

// runConfig contains specific configurations for Docker containers, embedding BaseConfig.
type runConfig struct {
	BaseConfig                    // Embed general configuration.
	ImageName   string            // Docker image to use.
	Binary      string            // Binary to run inside the container.
	BinaryArgs  []string          // Arguments for the binary.
	Mounts      map[string]string // Mounts (host path -> container path).
	NetworkMode string            // Network mode to use for the container. If empty, the docker default will apply
	PIDMode     string            // PID mode to use for the container. If empty, the docker default will apply
	Privileged  bool              // Whether to run the container in privileged mode.
	GPUs        string            // GPUs to use for the container
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

		if r.Privileged {
			args = append(args, "--privileged")
		}

		if r.NetworkMode != "" {
			args = append(args, "--network", r.NetworkMode)
		}

		if r.PIDMode != "" {
			args = append(args, "--pid", r.PIDMode)
		}

		if r.GPUs != "" {
			args = append(args, "--gpus", r.GPUs)
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
	BaseConfig        // Embed general configuration.
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

// WithBinaryArgs sets the arguments for the binary to run inside the container.
func WithBinaryArgs(binaryArgs []string) RunConfigOption {
	return func(c *runConfig) {
		c.BinaryArgs = binaryArgs
	}
}

// WithMounts sets the volume mounts for the container (host path -> container path).
func WithMounts(mounts map[string]string) RunConfigOption {
	return func(c *runConfig) {
		c.Mounts = mounts
	}
}

// WithNetworkMode sets the network mode for the container.
func WithNetworkMode(networkMode string) RunConfigOption {
	return func(c *runConfig) {
		c.NetworkMode = networkMode
	}
}

// WithPIDMode sets the PID mode for the container.
func WithPIDMode(pidMode string) RunConfigOption {
	return func(c *runConfig) {
		c.PIDMode = pidMode
	}
}

// WithPrivileged sets the privileged flag for the container.
func WithPrivileged(privileged bool) RunConfigOption {
	return func(c *runConfig) {
		c.Privileged = privileged
	}
}

// NewRunConfig creates a new runConfig instance for a single docker container.
// The baseConfig is the base configuration for the docker container.
// imageName is the name of the docker image to use.
// binary is the binary to run inside the container.
// baseConfig, imageName and binary are required.
// opts are the optional parameters for the runConfig.
func NewRunConfig(baseConfig BaseConfig, imageName string, binary string, opts ...RunConfigOption) LifecycleConfig {
	cfg := &runConfig{
		BaseConfig: baseConfig,
		ImageName:  imageName,
		Binary:     binary,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return *cfg
}

// NewComposeConfig creates a new composeConfig instance for the docker-compose.
// The baseConfig is the base configuration for the docker-compose setup.
// The file is the path to the docker-compose file. Both are required.
// NewComposeConfig does not have optional parameters.
func NewComposeConfig(baseConfig BaseConfig, file string) LifecycleConfig {
	cfg := &composeConfig{
		BaseConfig: baseConfig,
		File:       file,
	}
	return cfg
}

// BaseConfigOption represents options for the base configuration for all docker lifecycle configs
type BaseConfigOption func(*BaseConfig)

// RunConfigOption represents options for the run configuration for a docker container
type RunConfigOption func(*runConfig)

// ComposeConfigOption represents options for the compose configuration for a docker-compose setup
type ComposeConfigOption func(*composeConfig)

// WithTimeout sets the timeout for the container operation.
func WithTimeout(timeout time.Duration) BaseConfigOption {
	return func(c *BaseConfig) {
		c.timeout = timeout
	}
}

// WithRetries sets the number of retries for starting the container.
func WithRetries(retries int) BaseConfigOption {
	return func(c *BaseConfig) {
		c.retries = retries
	}
}

// WithEnv sets the environment variables for the container.
func WithEnv(env []string) BaseConfigOption {
	return func(c *BaseConfig) {
		c.env = env
	}
}

func WithGPUs(gpus string) RunConfigOption {
	return func(c *runConfig) {
		c.GPUs = gpus
	}
}

// NewBaseConfig creates a new base configuration for a docker container or docker-compose setup.
// The name is used to identify the container or docker-compose setup.
// The patternScanner is used to match logs for readiness and completion of the target container/s.
// The opts are used to configure optional parameters for the base configuration.
func NewBaseConfig(name string, patternScanner *testutil.PatternScanner, opts ...BaseConfigOption) BaseConfig {
	cfg := BaseConfig{
		name:           name,
		patternScanner: patternScanner,
		timeout:        DefaultTimeout,
		retries:        DefaultRetries,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}
