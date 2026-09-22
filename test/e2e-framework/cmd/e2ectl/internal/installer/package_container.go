// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/buildprovider"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/localinfra"
	pc "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/localpackage"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/container/packagecore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

// validateContainer adds the container-target rules on top of the shared
// section validation: only exact existing packages (a hand-selected local
// DEB or a pipeline download), an explicit receiver, and no integrations (the
// container mechanism runs the fixed Go core checks only).
func (p *Package) validateContainer(c *config.File) []error {
	errs := p.Validate(c)
	section, err := decodeAgentSection(pc.Schema, c, p.ID())
	if err != nil {
		return append(errs, err)
	}
	if c.Agent.Receiver == nil {
		errs = append(errs, fmt.Errorf("agent.package on a container target requires an explicit receiver; legacy routing is unsupported"))
	}
	if c.Agent.Build == nil || (c.Agent.Build.Provider != "existing-package" && c.Agent.Build.Provider != "pipeline") {
		errs = append(errs, fmt.Errorf("agent.package on a container target consumes only exact existing packages (existing-package, pipeline download); never builds Agent source"))
	}
	if len(section.Integrations) > 0 {
		errs = append(errs, fmt.Errorf("agent.package integrations are supported on SSH host targets only; the container target runs the fixed core checks"))
	}
	return errs
}

const packageCoreKey = "_agent_package_core"

func readPackageCore(e envstore.Entry) (packagecore.Runtime, error) {
	var installed packagecore.Runtime
	_, meta, err := provisioner.ReadSnapshotFile(e.SnapshotPath())
	if err != nil {
		return installed, err
	}
	if err = json.Unmarshal(meta[packageCoreKey], &installed); err != nil {
		return installed, fmt.Errorf("container-target package installation receipt missing")
	}
	return installed, nil
}

// updateContainer is the local Docker mechanism: a real dpkg installation in
// an unprivileged Ubuntu container filesystem, then the core Agent in the
// foreground on the environment's network.
func (p *Package) updateContainer(c *config.File, e envstore.Entry, skip bool) error {
	if errs := p.validateContainer(c); len(errs) > 0 {
		return config.NewErrors(errs)
	}
	section, err := decodeAgentSection(pc.Schema, c, p.ID())
	if err != nil {
		return err
	}
	runtime := &Binary{} // Shared local container/owned-volume mechanics only.
	if err = runtime.preflightRuntimeState(e); err != nil {
		return err
	}
	plan, err := p.PrepareRouting(c, e)
	if err != nil {
		return err
	}
	ctx, cancel := artifactContext()
	defer cancel()
	consumer := packagecore.Installer{}
	var result agentbuild.Result
	var installed packagecore.Runtime
	if skip {
		result, err = readArtifact(e)
		if err == nil {
			installed, err = readPackageCore(e)
		}
		if err == nil {
			// The pin comes from the persisted installed receipt when the
			// section does not carry one (derived pipeline selections): the
			// artifact must still match what was actually installed.
			expected := section.SHA256
			if expected == "" {
				expected = installed.PackageSHA256
			}
			if err = packagecore.Validate(result, expected); err == nil {
				err = runtime.verifyRuntimeState(e)
			}
		}
		if err == nil {
			err = consumer.Verify(ctx, result, installed, localinfra.AgentContainer(e.Name))
		}
	} else {
		var prepared buildprovider.PackageResult
		prepared, err = buildprovider.Packages.Prepare(ctx, c.Agent.Build, artifactRequest(e, localTarget()))
		result = prepared.Result
		if err == nil {
			// A derived pipeline selection has no user-typed digest: the
			// provider computed it over the downloaded file; the installation
			// below verifies that exact value again inside the image build.
			expected := section.SHA256
			if expected == "" {
				expected = result.Package.File.SHA256
			}
			installed, err = consumer.InstallImage(ctx, result, expected)
		}
	}
	if err != nil {
		return err
	}
	// Isolated dummy probe precedes credential resolution and old-Agent mutation.
	if err = consumer.Probe(ctx, installed, *plan, section.Config); err != nil {
		return err
	}
	key, err := bindAPIKey(plan)
	if err != nil {
		return err
	}
	rendered, err := packagecore.Config(*plan, key, section.Config, localinfra.AgentContainer(e.Name))
	if err != nil {
		return err
	}
	if err = packagecore.PrepareChecks(filepath.Join(e.Dir, "package-core-conf.d")); err != nil {
		return err
	}
	if err = runtime.prepareRuntimeState(e); err != nil {
		return err
	}
	if err = artifactPhase(e, result, "activating"); err != nil {
		return err
	}
	activate := func() error {
		if err := writePreparedAgentConfig(e, rendered); err != nil {
			return err
		}
		if _, err := runtime.dockerCommand("rm", "-f", localinfra.AgentContainer(e.Name)); err != nil {
			return err
		}
		if _, err := runtime.dockerCommand(packageContainerRunArgs(e, installed)...); err != nil {
			return err
		}
		if err := runtime.waitForReady(e); err != nil {
			return err
		}
		if err := consumer.Verify(ctx, result, installed, localinfra.AgentContainer(e.Name)); err != nil {
			return err
		}
		if err := consumer.Observe(ctx, &installed, *plan, localinfra.AgentContainer(e.Name)); err != nil {
			return err
		}
		updates, err := localHostOutputs(e, installed.OSVersion)
		if err != nil {
			return err
		}
		if err = provisioner.UpdateSnapshotResources(e.SnapshotPath(), nil, map[string]any{packageCoreKey: installed}); err != nil {
			return err
		}
		return publishArtifact(e, result, updates)
	}
	if err = activate(); err != nil {
		_ = artifactPhase(e, result, "failed")
	}
	return err
}

func packageContainerRunArgs(e envstore.Entry, installed packagecore.Runtime) []string {
	volume := localinfra.AgentRuntimeVolume(e.Dir, e.Meta.CreatedAt)
	return []string{"run", "-d", "--pull=never", "--name", localinfra.AgentContainer(e.Name), "--network", localinfra.NetworkName(e.Name), "--hostname", localinfra.AgentContainer(e.Name),
		"-v", filepath.Join(e.Dir, "agent.yaml") + ":/etc/datadog-agent/datadog.yaml:ro",
		"-v", filepath.Join(e.Dir, "package-core-conf.d") + ":/etc/datadog-agent/conf.d:ro",
		"--mount", "type=volume,source=" + volume.Name + ",target=/opt/datadog-agent/run,volume-nocopy",
		"--entrypoint", "sh", installed.ImageID, "-ec", "chmod 0700 /opt/datadog-agent/run && exec " + packagecore.AgentBinPath + " run -c /etc/datadog-agent/datadog.yaml"}
}
