// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localpackage installs one verified DEB on an initialized SSH host.
package localpackage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	ostypes "github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentconfig"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/configure"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

type Params struct {
	Artifact      agentbuild.Result
	AllowUnsigned bool
	Routing       *receivers.Plan
	APIKey        string
	AgentConfig   string
	Integrations  map[string]string
}

func Target(host outputs.HostOutput) (agentbuild.Target, error) {
	if host.Transport == "docker" || host.OSFamily != ostypes.LinuxFamily || host.OSFlavor != ostypes.Ubuntu {
		return agentbuild.Target{}, fmt.Errorf("package installer supports Ubuntu SSH hosts only")
	}
	arch := string(host.Architecture)
	if host.Architecture == ostypes.AMD64Arch {
		arch = "amd64"
	}
	t := agentbuild.Target{OS: "linux", Arch: arch}
	return t, t.Validate()
}
func Validate(p Params, t agentbuild.Target) error {
	if !p.AllowUnsigned {
		return fmt.Errorf("local unsigned DEB requires allow-unsigned: true")
	}
	if err := p.Artifact.Validate(t); err != nil {
		return err
	}
	if p.Artifact.Package == nil {
		return fmt.Errorf("package artifact required")
	}
	if err := configure.ValidateIntegrations(p.Integrations); err != nil {
		return err
	}
	if p.Routing != nil {
		if p.Artifact.Profile == nil {
			return fmt.Errorf("explicit receiver requires verified producer capability receipt; arbitrary packages and unvalidated repacks are unsupported")
		}
		if err := p.Artifact.Profile.Require(receivers.CoreAgent, receivers.TraceAgent, receivers.ProcessAgent); err != nil {
			return err
		}
	}
	return nil
}

// Transport permits offline command-order and upload-failure tests. Commands
// and staging paths are installer-owned, never config/package-derived shell.
type Transport struct {
	Execute  func(string) (string, error)
	CopyFile func(string, string) error
}

const services = "datadog-agent.service datadog-agent-trace.service datadog-agent-process.service datadog-agent-sysprobe.service datadog-agent-security.service"

// InstallFile verifies remote metadata and digest before apt can stop the old
// Agent. Services remain runtime-masked on package/config failure (fail closed).
// It returns the unmask operation only on successful package installation.
func InstallFile(remote Transport, r agentbuild.Result) (func() error, error) {
	release, _, err := installFileVerified(remote, r)
	return release, err
}

func installFileVerified(remote Transport, r agentbuild.Result) (func() error, *Verification, error) {
	if err := r.Validate(r.Target); err != nil {
		return nil, nil, err
	}
	if r.Package == nil {
		return nil, nil, fmt.Errorf("DEB artifact required")
	}
	actual, err := remote.Execute(". /etc/os-release && printf '%s ' \"$ID\" && dpkg --print-architecture")
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(actual) != "ubuntu "+r.Target.Arch {
		return nil, nil, fmt.Errorf("remote host OS/architecture differs from package target")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, nil, err
	}
	dir := "/tmp/e2ectl-package-" + hex.EncodeToString(nonce[:])
	path := dir + "/agent.deb"
	if _, err := remote.Execute("install -d -m 0700 " + dir); err != nil {
		return nil, nil, err
	}
	defer remote.Execute("rm -rf " + dir)
	if err := remote.CopyFile(r.Package.File.Path, path); err != nil {
		return nil, nil, fmt.Errorf("uploading exact package: %w", err)
	}
	sum, err := remote.Execute("sha256sum " + path)
	if err != nil {
		return nil, nil, err
	}
	fields := strings.Fields(sum)
	if len(fields) < 1 || fields[0] != r.Package.File.SHA256 {
		return nil, nil, fmt.Errorf("uploaded package checksum mismatch")
	}
	metadata, err := remote.Execute("dpkg-deb --show --showformat='${Package} ${Version} ${Architecture}' " + path)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(metadata) != "datadog-agent "+r.Package.Version+" "+r.Target.Arch {
		return nil, nil, fmt.Errorf("uploaded package target mismatch")
	}
	verification, err := prepareExecutableVerification(remote, r, path, dir)
	if err != nil {
		return nil, nil, err
	}
	release, err := acquireServiceMasks(remote, hex.EncodeToString(nonce[:]))
	if err != nil {
		return nil, nil, err
	}
	if _, err := remote.Execute(packageInstallCommand(path)); err != nil {
		return nil, nil, fmt.Errorf("installing DEB (owned service masks retained for repair): %w", err)
	}
	if err := verifyInstalledPackage(remote, verification); err != nil {
		return nil, nil, err
	}
	return release, verification, nil
}
func Install(ctx context.Context, env *environments.Host, p Params) error {
	_, err := InstallVerified(ctx, env, p)
	return err
}

// InstallVerified returns bounded installation evidence only after configuration
// and Agent initialization succeed. Install preserves the error-only API.
func InstallVerified(_ context.Context, env *environments.Host, p Params) (*Verification, error) {
	if env == nil || env.RemoteHost == nil {
		return nil, fmt.Errorf("initialized host required")
	}
	target, err := Target(env.RemoteHost.HostOutput)
	if err != nil {
		return nil, err
	}
	if err = Validate(p, target); err != nil {
		return nil, err
	}
	key := p.APIKey
	var config string
	if p.Routing != nil {
		if p.Routing.APIKeyRef == "" {
			key = receivers.DummyAPIKey
		}
		config, err = agentconfig.GenerateWithRouting(*p.Routing, key, p.AgentConfig)
	} else {
		key, err = runner.GetProfile().SecretStore().Get(parameters.APIKey)
		if err == nil {
			config, err = configure.Generate(env, key, p.AgentConfig)
		}
	}
	if err != nil {
		return nil, err
	}
	unmask, verification, err := installFileVerified(Transport{Execute: func(s string) (string, error) { return env.RemoteHost.Execute(s) }, CopyFile: env.RemoteHost.CopyFileE}, p.Artifact)
	if err != nil {
		return nil, err
	}
	// configure.Apply owns the same private YAML/conf.d application and client
	// initialization as script installs. Unmask only after private files are set.
	if err := configure.ApplyBeforeRestart(env, config, p.Integrations, unmask); err != nil {
		return nil, err
	}
	return verification, nil
}
