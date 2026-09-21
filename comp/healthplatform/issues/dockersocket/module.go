// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dockersocket provides issue modules for Docker socket reachability problems
// (permission-denied and other dial failures). It includes both detection (built-in health
// check) and remediation (issue templates with fix scripts).
package dockersocket

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
	issues.RegisterModuleFactory(NewSocketUnavailableModule)
}

// Docker Socket Permission issue identity.
const (
	// PermissionIssueName is the identifier for the Docker socket permission issue,
	// used as the template registry key and the proto IssueName field.
	PermissionIssueName = "Docker Socket Permission"

	// PermissionIssueType is the snake_case type key for the Docker socket permission
	// issue: PermissionIssueName lowercased with spaces replaced by underscores.
	PermissionIssueType = "docker_socket_permission"

	// PermissionIssueID is the unique instance id prefix used when reporting this issue.
	PermissionIssueID = "docker-socket-permissions"
)

// Docker Socket Unavailable issue identity.
const (
	// SocketUnavailableIssueName is the identifier for a non-permission Docker socket reachability issue.
	SocketUnavailableIssueName = "Docker Socket Unavailable"

	// SocketUnavailableIssueType is the snake_case type key for SocketUnavailableIssueName.
	SocketUnavailableIssueType = "docker_socket_unavailable"

	// SocketUnavailableIssueID is the unique instance id prefix used when reporting this issue.
	SocketUnavailableIssueID = "docker-socket-unavailable"
)

// checker resolves Docker socket reachability and scopes reported issue ids to this host (docker sockets are host-local, not cluster-wide).
type checker struct {
	hostname hostnameinterface.Component
}

func newChecker(hostname hostnameinterface.Component) *checker {
	return &checker{hostname: hostname}
}

// instanceIssueID scopes issueID to this host and the affected socket set, since the backend
// dedups on id alone; caller passes sockets pre-sorted and comma-joined, matching the string
// already put in Context["socketPaths"].
func (c *checker) instanceIssueID(issueID, sortedSocketPaths string) string {
	h := fnv.New64a()
	h.Write([]byte(c.hostname.GetSafe(context.Background()))) // never returns an error for hash.Hash
	h.Write([]byte{0})                                        // delimiter between hostname and sockets
	h.Write([]byte(sortedSocketPaths))
	return fmt.Sprintf("%s:%016x", issueID, h.Sum64())
}

// dockerPermissionsModule implements issues.Module
type dockerPermissionsModule struct {
	template *DockerPermissionIssue
	checker  *checker
}

// NewModule creates a new Docker permissions issue module
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &dockerPermissionsModule{
		template: NewDockerPermissionIssue(),
		checker:  newChecker(deps.Hostname),
	}
}

func (m *dockerPermissionsModule) IssueName() string {
	return PermissionIssueName
}

func (m *dockerPermissionsModule) IssueType() string {
	return PermissionIssueType
}

func (m *dockerPermissionsModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns the periodic health check configuration.
// Interval is 0 to use the default (15 minutes).
func (m *dockerPermissionsModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
			Source: "docker",
			Fn:     m.checker.Check,
			// Check() also reports under SocketUnavailableIssueName, so pre-seed it for restart resolution.
			IssueNames: []string{SocketUnavailableIssueName},
		},
	}
}

// BuiltInStartupHealthCheck returns nil — docker permission checks run periodically.
func (m *dockerPermissionsModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}

// dockerSocketUnavailableModule registers the "Docker Socket Unavailable" template but contributes no check of its own; dockerPermissionsModule's shared Check() reports under both.
type dockerSocketUnavailableModule struct {
	template *DockerSocketUnavailableIssue
}

// NewSocketUnavailableModule creates a new Docker socket unavailable issue module.
func NewSocketUnavailableModule(issues.ModuleDeps) issues.Module {
	return &dockerSocketUnavailableModule{
		template: NewDockerSocketUnavailableIssue(),
	}
}

func (m *dockerSocketUnavailableModule) IssueName() string {
	return SocketUnavailableIssueName
}

func (m *dockerSocketUnavailableModule) IssueType() string {
	return SocketUnavailableIssueType
}

func (m *dockerSocketUnavailableModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil — Check() is already registered by dockerPermissionsModule and emits reports for both issue names.
func (m *dockerSocketUnavailableModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil — see BuiltInPeriodicHealthCheck.
func (m *dockerSocketUnavailableModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
