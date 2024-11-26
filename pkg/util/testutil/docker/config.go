// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package docker

import (
	"regexp"
	"time"
)

const (
	// DefaultTimeout is the default timeout for running a server.
	DefaultTimeout = time.Minute

	// DefaultRetries is the default number of retries for starting a container/s.
	DefaultRetries = 3
)

type commandType string

const (
	composeCommand commandType = "compose"
	runCommand     commandType = "run"
	removeCommand  commandType = "rm"
)

// Compile-time interface compliance check
var _ LifecycleConfig = (*runConfig)(nil)
var _ LifecycleConfig = (*composeConfig)(nil)

// LifecycleConfig is an interface for the common configuration of a container lifecycle.
type LifecycleConfig interface {
	Timeout() time.Duration
	Retries() int
	LogPattern() *regexp.Regexp
	Env() []string
	Name() string
}

// Timeout returns the timeout to be used when running a container/s
func (b baseConfig) Timeout() time.Duration {
	return b.timeout
}

// Retries returns the number of retries to be used when trying to start the container/s
func (b baseConfig) Retries() int {
	return b.retries
}

// LogPattern returns the regex pattern to match logs for readiness
func (b baseConfig) LogPattern() *regexp.Regexp {
	return b.logPattern
}

// Env returns the environment variables to set for the container/s
func (b baseConfig) Env() []string {
	return b.env
}

// Name returns the name of the docker container or a friendly name for the docker-compose setup
func (b baseConfig) Name() string {
	return b.name
}

// BaseConfig contains shared configurations for both Docker and Docker Compose.
type baseConfig struct {
	name       string         // Container name for docker or an alias for docker-compose
	timeout    time.Duration  // Timeout for the entire operation.
	retries    int            // Number of retries for starting.
	logPattern *regexp.Regexp // Regex pattern to match logs for readiness.
	env        []string       // Environment variables to set.
}

// runConfig contains specific configurations for Docker containers, embedding BaseConfig.
type runConfig struct {
	baseConfig                   // Embed general configuration.
	ImageName  string            // Docker image to use.
	Binary     string            // Binary to run inside the container.
	BinaryArgs []string          // Arguments for the binary.
	Mounts     map[string]string // Mounts (host path -> container path).
}

// composeConfig contains specific configurations for Docker Compose, embedding BaseConfig.
type composeConfig struct {
	baseConfig        // Embed general configuration.
	File       string // Path to the docker-compose file.
}

// NewRunConfig creates a new runConfig instance for a single docker container.
func NewRunConfig(name string, timeout time.Duration, retries int, logPattern *regexp.Regexp, env []string, imageName, binary string, binaryArgs []string, mounts map[string]string) LifecycleConfig {
	return runConfig{
		baseConfig: baseConfig{
			timeout:    timeout,
			retries:    retries,
			logPattern: logPattern,
			env:        env,
			name:       name,
		},
		ImageName:  imageName,
		Binary:     binary,
		BinaryArgs: binaryArgs,
		Mounts:     mounts,
	}
}

// NewComposeConfig creates a new composeConfig instance for the docker-compose.
func NewComposeConfig(name string, timeout time.Duration, retries int, logPattern *regexp.Regexp, env []string, file string) LifecycleConfig {
	return composeConfig{
		baseConfig: baseConfig{
			timeout:    timeout,
			retries:    retries,
			logPattern: logPattern,
			env:        env,
			name:       name,
		},
		File: file,
	}
}
