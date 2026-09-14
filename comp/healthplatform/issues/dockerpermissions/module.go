// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dockerpermissions provides a complete issue module for Docker permission problems.
// It includes both detection (built-in health check) and remediation (issue template with fix scripts).
package dockerpermissions

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

const (
	// IssueName is the identifier for the Docker socket permission issue,
	// used as the template registry key and the proto IssueName field.
	IssueName = "Docker Socket Permission"

	// IssueType is the snake_case type key for the Docker socket permission
	// issue: IssueName lowercased with spaces replaced by underscores.
	IssueType = "docker_socket_permission"

	// IssueID is the unique instance id used when reporting this issue
	IssueID = "docker-socket-permissions"

	// SocketUnavailableIssueName is the identifier for the generic Docker
	// socket unavailable issue (any non-permission reachability failure),
	// used as the template registry key and the proto IssueName field.
	SocketUnavailableIssueName = "Docker Socket Unavailable"

	// SocketUnavailableIssueType is the snake_case type key for the Docker
	// socket unavailable issue: SocketUnavailableIssueName lowercased with
	// spaces replaced by underscores.
	SocketUnavailableIssueType = "docker_socket_unavailable"

	// SocketUnavailableIssueID is the unique instance id used when reporting
	// this issue.
	SocketUnavailableIssueID = "docker-socket-unavailable"
)

// checker resolves Docker socket reachability and scopes reported issue ids
// to this host. Docker socket availability is host-local (each node has its
// own daemon), so the discriminator must be the bare hostname rather than
// issues.IssueDiscriminator's DaemonSet-uid preference: under a Kubernetes
// DaemonSet, every node shares the same uid, which would collapse distinct
// per-node failures into a single backend issue. See
// comp/healthplatform/README.md's "per-host (host-local failure)" scoping
// guidance and comp/healthplatform/issues/missedbytes/check.go's
// hostIssueID for the established pattern this mirrors.
type checker struct {
	hostname hostnameinterface.Component
}

func newChecker(hostname hostnameinterface.Component) *checker {
	return &checker{hostname: hostname}
}

// instanceIssueID scopes baseID to this host. The backend dedups on id
// alone, so without this digest one host going clean would resolve the
// issue for every host. See the checker doc comment for why a bare hostname
// is used instead of issues.IssueDiscriminator.
func (c *checker) instanceIssueID(baseID string) string {
	h := fnv.New64a()
	h.Write([]byte(c.hostname.GetSafe(context.Background()))) // never returns an error for hash.Hash
	return fmt.Sprintf("%s:%016x", baseID, h.Sum64())
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
	return IssueName
}

func (m *dockerPermissionsModule) IssueType() string {
	return IssueType
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
			// Check() also reports under SocketUnavailableIssueName (see
			// dockerSocketUnavailableModule below, which owns no check of its
			// own) — pre-seed that name here too so bundle.go's restart
			// seeding resolves persisted docker-socket-unavailable issues,
			// not just docker-socket-permissions ones.
			IssueNames: []string{SocketUnavailableIssueName},
		},
	}
}

// BuiltInStartupHealthCheck returns nil — docker permission checks run periodically.
func (m *dockerPermissionsModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}

// dockerSocketUnavailableModule implements issues.Module. It registers the
// "Docker Socket Unavailable" template so the runner's registry can resolve
// reports emitted by Check(), but contributes no health check of its own:
// Check() (registered by dockerPermissionsModule above) emits reports under
// both this module's IssueName and dockerPermissionsModule's.
//
// Because the shared check is registered under dockerPermissionsModule,
// Registry.RegisterModule only auto-appends IssueName (this module's own
// name is never appended anywhere). dockerPermissionsModule.
// BuiltInPeriodicHealthCheck pre-seeds SocketUnavailableIssueName into the
// check's IssueNames to compensate, so bundle.go's restart-resolution
// seeding covers both issue names instead of leaking persisted
// docker-socket-unavailable issues across restarts.
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

// BuiltInPeriodicHealthCheck returns nil — Check() is already registered by
// dockerPermissionsModule and emits reports for both issue names.
func (m *dockerSocketUnavailableModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil — see BuiltInPeriodicHealthCheck.
func (m *dockerSocketUnavailableModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
