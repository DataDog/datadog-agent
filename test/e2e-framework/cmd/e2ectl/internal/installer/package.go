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
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/localpackage"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"go.yaml.in/yaml/v3"
)

type HostPackage struct{}

func (*HostPackage) ID() string                                    { return "package" }
func (*HostPackage) AgentExample() (*yaml.Node, error)             { return pc.Schema.Example(nil) }
func (*HostPackage) Artifact(*config.File) (string, string, error) { return "", "", nil }
func (h *HostPackage) PrepareRouting(c *config.File, e envstore.Entry) (*receivers.Plan, error) {
	return PrepareRouting(c, e, false)
}
func (h *HostPackage) Validate(c *config.File) []error {
	section, err := decodeAgentSection(pc.Schema, c, h.ID())
	if err != nil {
		return []error{err}
	}
	if !section.AllowUnsigned {
		return []error{fmt.Errorf("agent.package.allow-unsigned must explicitly permit the selected package")}
	}
	if err := buildprovider.Packages.Validate(c.Agent.Build); err != nil {
		return []error{err}
	}
	if err := ValidateReceiver(h, c); err != nil {
		return []error{err}
	}
	return nil
}
func (h *HostPackage) Install(c *config.File, e envstore.Entry) error { return h.Update(c, e, false) }
func (h *HostPackage) Update(c *config.File, e envstore.Entry, skip bool) error {
	if errs := h.Validate(c); len(errs) > 0 {
		return config.NewErrors(errs)
	}
	section, err := decodeAgentSection(pc.Schema, c, h.ID())
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
	routing, err := h.PrepareRouting(c, e)
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

var _ Updatable = (*HostPackage)(nil)
