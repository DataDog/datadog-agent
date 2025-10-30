// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"syscall"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/health/issues"
)

const (
	// DockerLogsDir is the default Docker logs directory
	DockerLogsDir = "/var/lib/docker"
)

// NewDockerLogPermissionsCheckConfig creates a CheckConfig for Docker log permissions
func NewDockerLogPermissionsCheckConfig() healthplatform.CheckConfig {
	return healthplatform.CheckConfig{
		CheckName: "docker-log-permissions",
		CheckID:   "docker-log-permissions-check",
		Callback:  CheckDockerPermissions,
	}
}

// isPermissionError checks if an error is permission-related using proper error type checking
func isPermissionError(err error) bool {
	return errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM)
}

// CheckDockerPermissions performs Docker file tailing permission health checks
// The context can be used to cancel long-running operations during shutdown
func CheckDockerPermissions(ctx context.Context) ([]healthplatform.Issue, error) {
	var issues []healthplatform.Issue

	// Check if context is already cancelled before starting
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Check Docker file tailing permissions
	if issue := checkDockerFileTailing(); issue != nil {
		issues = append(issues, *issue)
	}

	return issues, nil
}

// checkDockerFileTailing checks if Docker file tailing permissions are working
func checkDockerFileTailing() *healthplatform.Issue {
	// Check if Docker logs directory exists and is accessible
	if _, err := os.Stat(DockerLogsDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Directory doesn't exist, not a permission issue
			return nil
		}
		// If it's a permission error, report it
		if isPermissionError(err) {
			return issues.GetDockerFileTailingIssue(DockerLogsDir)
		}
		return nil
	}

	// Try to access the containers subdirectory
	containersDir := DockerLogsDir + "/containers"
	if _, err := os.Stat(containersDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// No containers directory, not a permission issue
			return nil
		}
		// If it's a permission error, report it
		if isPermissionError(err) {
			return issues.GetDockerFileTailingIssue(DockerLogsDir)
		}
		return nil
	}

	// Try to read the containers directory
	if _, err := os.ReadDir(containersDir); err != nil {
		if isPermissionError(err) {
			return issues.GetDockerFileTailingIssue(DockerLogsDir)
		}
		return nil
	}

	return nil
}
