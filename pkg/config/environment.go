// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
)

// GetEnvDefault retrieves a value from the environment named by the key or return def if not set.
func GetEnvDefault(key, def string) string {
	value, found := os.LookupEnv(key)
	if !found {
		return def
	}
	return value
}

// IsContainerized returns whether the Agent is running on a Docker container
// DOCKER_DD_AGENT is set in our official Dockerfile
func IsContainerized() bool {
	return os.Getenv("DOCKER_DD_AGENT") != ""
}

// IsDockerRuntime returns true if we are to find the /.dockerenv file
// which is typically only set by Docker
func IsDockerRuntime() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// IsKubernetes returns whether the Agent is running on a kubernetes cluster
func IsKubernetes() bool {
	// Injected by Kubernetes itself
	if os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		return true
	}
	// support of Datadog environment variable for Kubernetes
	if os.Getenv("KUBERNETES") != "" {
		return true
	}
	return false
}

func IsECS() bool {
	if os.Getenv("AWS_EXECUTION_ENV") == "AWS_ECS_EC2" {
		return true
	}

	if IsECSFargate() {
		return false
	}

	return os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
		os.Getenv("ECS_CONTAINER_METADATA_URI") != "" ||
		os.Getenv("ECS_CONTAINER_METADATA_URI_V4") != ""
}

// IsECSFargate returns whether the Agent is running in ECS Fargate
func IsECSFargate() bool {
	return os.Getenv("ECS_FARGATE") != "" || os.Getenv("AWS_EXECUTION_ENV") == "AWS_ECS_FARGATE"
}

// IsHostProcAvailable returns whether host proc is available or not
func IsHostProcAvailable() bool {
	if IsContainerized() {
		return pathExists("/host/proc")
	}
	return true
}

// IsHostSysAvailable returns whether host proc is available or not
func IsHostSysAvailable() bool {
	if IsContainerized() {
		return pathExists("/host/sys")
	}
	return true
}

// IsServerless returns whether the Agent is running in a Lambda function
func IsServerless() bool {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
}
