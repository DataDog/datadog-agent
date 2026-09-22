// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/buildprovider"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	pc "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/localpackage"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/container/packagecore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/localpackage"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"go.yaml.in/yaml/v3"
)

// Package installs a verified DEB. Both installation targets install the same
// package; the environment snapshot's attached host output selects the target:
// a Docker-transport host (or no attached host, which is how the local base
// looks before its first install) installs into an unprivileged Ubuntu
// container filesystem and runs only the core Agent in the foreground, while
// an SSH host installs through apt with full systemd services. The
// core-health vs full-systemd limitation is a property of the target, not of
// the installer: local Docker targets run core foreground only; remote VM
// targets run full systemd services.
type Package struct{}

func (*Package) ID() string                                    { return "package" }
func (*Package) AgentExample() (*yaml.Node, error)             { return pc.Schema.Example(nil) }
func (*Package) Artifact(*config.File) (string, string, error) { return "", "", nil }

// packageTarget reads the attached host output — the public environment
// contract — to select the installation mechanism. It never dispatches on the
// driver. The local base attaches no host output before the first install, so
// a missing resource is the container target, not an error.
func packageTarget(entry envstore.Entry) bool {
	var host outputs.HostOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "remoteHost", &host); err != nil {
		return true
	}
	return host.Transport == "docker"
}

// RoutingScope names the container target's bounded core-health/configuration
// scope; SSH-host targets run full systemd services and carry no scope.
func (*Package) RoutingScope(entry envstore.Entry) string {
	if packageTarget(entry) {
		return packagecore.Scope
	}
	return ""
}

// PrepareRouting resolves the receiver plan for the selected target. The
// container target requires an explicit receiver (its observations are bounded
// and legacy routing is unsupported); the SSH host target keeps the legacy
// plan fallback.
func (p *Package) PrepareRouting(c *config.File, e envstore.Entry) (*receivers.Plan, error) {
	if !packageTarget(e) {
		return PrepareRouting(c, e, false)
	}
	if c.Agent.Receiver == nil {
		return nil, fmt.Errorf("agent.package on a container target requires an explicit receiver; legacy routing is unsupported")
	}
	plan, err := PrepareRouting(c, e, true)
	if err == nil {
		plan.Warnings = append(plan.Warnings, "Container-target core-health/configuration installation; no package producer attestation, all-signal routing safety, systemd/subagent validation, or ingestion proof.")
	}
	return plan, err
}

func (p *Package) Validate(c *config.File) []error {
	section, err := decodeAgentSection(pc.Schema, c, p.ID())
	if err != nil {
		return []error{err}
	}
	if !section.AllowUnsigned {
		return []error{fmt.Errorf("agent.package.allow-unsigned must explicitly permit the selected package")}
	}
	if err := buildprovider.Packages.Validate(c.Agent.Build); err != nil {
		return []error{err}
	}
	if err := ValidateReceiver(p, c); err != nil {
		return []error{err}
	}
	return nil
}

func (p *Package) Install(c *config.File, e envstore.Entry) error { return p.Update(c, e, false) }

func (p *Package) Update(c *config.File, e envstore.Entry, skip bool) error {
	if packageTarget(e) {
		return p.updateContainer(c, e, skip)
	}
	return p.updateHost(c, e, skip)
}

// updateHost is the SSH-host mechanism: upload, verified apt reinstall, service
// masks and full configuration/integrations through the localpackage installer.
func (p *Package) updateHost(c *config.File, e envstore.Entry, skip bool) error {
	if errs := p.Validate(c); len(errs) > 0 {
		return config.NewErrors(errs)
	}
	section, err := decodeAgentSection(pc.Schema, c, p.ID())
	if err != nil {
		return err
	}
	var host outputs.HostOutput
	if err = provisioner.ReadSnapshotResource(e.SnapshotPath(), "remoteHost", &host); err != nil {
		return err
	}
	target, err := localpackage.Target(host)
	if err != nil {
		return err
	}
	routing, err := p.PrepareRouting(c, e)
	if err != nil {
		return err
	}
	ctx, cancel := artifactContext()
	defer cancel()
	var result agentbuild.Result
	if skip {
		result, err = readArtifact(e)
		if err == nil {
			err = result.Validate(target)
		}
	} else {
		if routing != nil {
			if err := buildprovider.Packages.ValidateRouting(c.Agent.Build); err != nil {
				return err
			}
		}
		var prepared buildprovider.PackageResult
		prepared, err = buildprovider.Packages.Prepare(ctx, c.Agent.Build, artifactRequest(e, target))
		result = prepared.Result
	}
	if err != nil {
		return err
	}
	// The section digest pins the exact package when one is user-selected; a
	// derived pipeline selection has no user-typed digest — the provider
	// computed it over the downloaded file, and the upload verification below
	// checks that same value again on the remote host.
	if result.Package == nil {
		return fmt.Errorf("selected source did not produce a package")
	}
	if section.SHA256 != "" && result.Package.File.SHA256 != section.SHA256 {
		return fmt.Errorf("agent.package.sha256 does not match the selected package")
	}
	params := localpackage.Params{Artifact: result, AllowUnsigned: section.AllowUnsigned, Routing: routing, AgentConfig: section.Config, Integrations: section.Integrations}
	if err = localpackage.Validate(params, target); err != nil {
		return err
	}
	params.APIKey, err = bindAPIKey(routing)
	if err != nil {
		return err
	}
	env, err := attachHostForInstall(e)
	if err != nil {
		return err
	}
	if err = artifactPhase(e, result, "activating"); err != nil {
		return err
	}
	verification, err := localpackage.InstallVerified(ctx, env, params)
	if err != nil {
		_ = artifactPhase(e, result, "failed")
		return err
	}
	data, err := json.Marshal(env.Agent.HostAgentOutput)
	if err != nil {
		return err
	}
	return publishArtifact(e, result, provisioner.RawResources{"agent": data}, verification)
}

// ApplyRouting is the container target's no-build receiver apply. SSH host
// targets fail closed: their no-build apply path is not implemented and no
// install fallback is performed.
func (p *Package) ApplyRouting(c *config.File, e envstore.Entry) error {
	if c.Agent.Receiver == nil {
		return fmt.Errorf("receiver apply requires an explicit selection")
	}
	if err := validateRoutingOnlyChange(c, e); err != nil {
		return err
	}
	if !packageTarget(e) {
		return fmt.Errorf("no-build receiver apply is not supported for agent.package on SSH host targets; no install fallback is performed")
	}
	return p.Update(c, e, true)
}

var (
	_ Installer      = (*Package)(nil)
	_ Updatable      = (*Package)(nil)
	_ RoutingApplier = (*Package)(nil)
)
