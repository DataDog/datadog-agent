// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentbuild

import (
	"fmt"
	"strings"
)

// PackageExecutablePath accepts only known Agent executable roles. These fixed
// paths are also the confinement boundary for installer archive extraction.
func PackageExecutablePath(role string) (string, error) {
	if role == "agent" {
		return "/opt/datadog-agent/bin/agent/agent", nil
	}
	switch role {
	case "trace-agent", "process-agent", "security-agent", "system-probe", "installer", "trace-loader", "privateactionrunner":
		return "/opt/datadog-agent/embedded/bin/" + role, nil
	}
	return "", fmt.Errorf("unsupported package executable role %q", role)
}

func validatePackageRoles(roles []string) error {
	seen := map[string]bool{}
	for _, role := range roles {
		if _, err := PackageExecutablePath(role); err != nil {
			return err
		}
		if seen[role] {
			return fmt.Errorf("duplicate package role %q", role)
		}
		seen[role] = true
	}
	return nil
}

// The DEB checksum covers the packaged embedded runtime; Depends records its
// external package-manager requirements. Roles are observed archive paths, not
// claims about which binaries were rebuilt or a receiver capability profile.
func packageRoles(contents string) []string {
	paths := map[string]bool{}
	for _, line := range strings.Split(contents, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.HasPrefix(fields[0], "-") {
			paths[fields[len(fields)-1]] = true
		}
	}
	roles := []string{}
	if paths["./opt/datadog-agent/bin/agent/agent"] {
		roles = append(roles, "agent")
	}
	for _, role := range []string{"trace-agent", "process-agent", "security-agent", "system-probe", "installer", "trace-loader", "privateactionrunner"} {
		if paths["./opt/datadog-agent/embedded/bin/"+role] {
			roles = append(roles, role)
		}
	}
	return roles
}
