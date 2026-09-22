// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/buildprovider"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/localinfra"
	binaryconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/binary"
	e2eostypes "github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentconfig"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
	"go.yaml.in/yaml/v3"
)

// agentBinPathInContainer is where the pinned agent binary lands inside the
// runtime container: the install bind-mounts it over the image's agent binary.
const agentBinPathInContainer = "/opt/datadog-agent/bin/agent/agent"

// DefaultRuntimeImage provides Ubuntu system libraries and Agent Python
// site-packages. The actual executable, CPython and rtloader come from the staged
// bundle. The adapter verifies matching Python ABI and records the image ID.
const DefaultRuntimeImage = "registry.datadoghq.com/agent:7.83.0"

// Binary installs the Agent built from the working tree, running it in a
// container on the environment's Docker network. The container is the process
// handle: `docker rm -f` stops it, `docker logs` is the debugging surface.
type Binary struct {
	adapter agentbuild.Adapter
	docker  func(...string) (string, error)
	ready   func(envstore.Entry) error
}

// ID implements Installer.
func (b *Binary) ID() string { return "binary" }

// AgentExample implements Installer: the binary section schema's example.
func (b *Binary) AgentExample() (*yaml.Node, error) { return binaryconfig.Schema.Example(nil) }

// Artifact is the legacy requested-summary API. InstalledSummary reports the
// verified output receipt for new binary installations.
func (b *Binary) Artifact(cfg *config.File) (string, string, error) {
	if _, err := decodeBinarySection(cfg); err != nil {
		return "", "", err
	}
	return "", "", nil
}

// Validate implements Installer: the binary section's own rules.
func (b *Binary) Validate(cfg *config.File) []error {
	section, err := decodeBinarySection(cfg)
	if err != nil {
		return []error{err}
	}
	if cfg.Agent.Build != nil {
		if err := buildprovider.Binaries.Validate(cfg.Agent.Build); err != nil {
			return []error{err}
		}
	}
	if err := ValidateReceiver(b, cfg); err != nil {
		return []error{err}
	}
	if cfg.Agent.Receiver != nil {
		if section.RuntimeImage != "" && section.RuntimeImage != DefaultRuntimeImage {
			return []error{fmt.Errorf("explicit routing requires the pinned runtime image")}
		}
		m, err := receivers.ValidateConfig(section.Config)
		if err != nil {
			return []error{err}
		}
		for _, key := range []string{"apm_config.enabled", "process_config.process_collection.enabled", "process_config.container_collection.enabled"} {
			if m[key] == true {
				return []error{fmt.Errorf("%s: Binary runs core only, not separate APM/process subagents", key)}
			}
		}
	}
	return nil
}

func decodeBinarySection(cfg *config.File) (binaryconfig.Config, error) {
	return decodeAgentSection(binaryconfig.Schema, cfg, "binary")
}

// Install implements Installer: build the Agent from the working tree, pin it
// into the environment, and run it in a container on the environment's
// network, replacing any container a previous install left behind.
func (b *Binary) Install(cfg *config.File, entry envstore.Entry) error {
	return b.Update(cfg, entry, false)
}

// writeSnapshotOutputs records the host and agent components in the
// snapshot. remoteHost makes the agent container look like a Host to the test
// framework: Transport=docker tells RemoteHost.Init to use docker exec with
// Address as the container name. agent carries the pinned binary path so the
// AgentClient invokes it directly (the container has no sudo and no
// datadog-agent wrapper). The existing StaticStackProvisioner rehydrates
// environments.Host from these resources with zero provisioner changes.
func (b *Binary) writeSnapshotOutputs(entry envstore.Entry, result agentbuild.Result) error {
	updates, err := localHostOutputs(entry, result.Binary.OSVersion)
	if err != nil {
		return err
	}
	return publishArtifact(entry, result, updates)
}

func localHostOutputs(entry envstore.Entry, osVersion string) (provisioner.RawResources, error) {
	hostOut := outputs.HostOutput{
		CloudProvider: "local",
		Transport:     "docker",
		Address:       localinfra.AgentContainer(entry.Name),
		OSFamily:      e2eostypes.LinuxFamily,
		OSFlavor:      e2eostypes.Ubuntu,
		OSVersion:     osVersion,
		Architecture:  binaryArchitecture(),
	}
	hostJSON, err := json.Marshal(hostOut)
	if err != nil {
		return nil, err
	}
	agentJSON, err := json.Marshal(outputs.HostAgentOutput{
		Host:         hostOut,
		AgentBinPath: agentBinPathInContainer,
	})
	if err != nil {
		return nil, err
	}
	return provisioner.RawResources{"remoteHost": hostJSON, "agent": agentJSON}, nil
}

// Update prepares/acquires and verifies a fresh immutable generation before
// activation. skipBuild reuses installed pins, never current worktree outputs.
func (b *Binary) Update(cfg *config.File, entry envstore.Entry, skipBuild bool) error {
	section, err := decodeBinarySection(cfg)
	if err != nil {
		return err
	}
	if err := b.preflightRuntimeState(entry); err != nil {
		return err
	}
	agentYAML, err := b.prepareConfig(cfg, entry, section)
	if err != nil {
		return err
	}
	ctx, cancel := artifactContext()
	defer cancel()
	var result agentbuild.Result
	if skipBuild {
		result, err = readArtifact(entry)
		if err == nil && cfg.Agent.Receiver != nil {
			err = result.RequireBinaryRouting()
		}
		if err == nil {
			err = result.Validate(localTarget())
		}
		if err == nil && result.Binary == nil {
			err = fmt.Errorf("installed artifact is not a binary bundle")
		}
		if err == nil {
			err = b.verifyRuntimeState(entry)
		}
		if err == nil {
			_, err = b.adapter.InspectImage(ctx, result.Binary.RuntimeImageID, result.Target)
		}
	} else {
		selection := cfg.Agent.Build
		if selection == nil {
			selection, err = legacyBinarySelection()
			if err != nil {
				return err
			}
		}
		if cfg.Agent.Receiver != nil {
			if err := buildprovider.Binaries.ValidateRouting(selection); err != nil {
				return err
			}
		}
		image := section.RuntimeImage
		if image == "" {
			image = DefaultRuntimeImage
		}
		runtimeID, preflightErr := b.adapter.PreflightRuntimeImage(ctx, image, localTarget())
		if preflightErr != nil {
			return preflightErr
		}
		request := artifactRequest(entry, localTarget())
		request.Adapter = b.adapter
		prepared, prepErr := buildprovider.Binaries.Prepare(ctx, selection, request)
		result, err = prepared.Result, prepErr
		if err == nil && cfg.Agent.Receiver != nil {
			err = result.RequireBinaryRouting()
		}
		if err == nil {
			result, err = b.adapter.VerifyRuntime(ctx, result, runtimeID)
		}
	}
	if err != nil {
		return err
	}
	if result.Binary.RuntimeImageID == "" || result.Binary.PythonPath == "" {
		return fmt.Errorf("runtime verification receipt missing")
	}
	if err := b.prepareRuntimeState(entry); err != nil {
		return err
	}
	// All builds, staging, identity checks and Python/Go runtime probes finish
	// before replacing a working Agent or writing its live configuration.
	if err := artifactPhase(entry, result, "activating"); err != nil {
		return err
	}
	if err := b.runPreparedBinary(entry, section, agentYAML, result); err != nil {
		_ = artifactPhase(entry, result, "failed")
		return err
	}
	if err := b.waitForReady(entry); err != nil {
		_ = artifactPhase(entry, result, "failed")
		return err
	}
	return b.writeSnapshotOutputs(entry, result)
}

func (b *Binary) runPreparedBinary(entry envstore.Entry, section binaryconfig.Config, agentYAML string, result agentbuild.Result) error {
	return b.activatePreparedBinary(entry, section, agentYAML, preparedBinaryRunArgs(entry, *result.Binary))
}

func (b *Binary) activatePreparedBinary(entry envstore.Entry, section binaryconfig.Config, agentYAML string, args []string) error {
	if err := writePreparedAgentFiles(entry, section, agentYAML); err != nil {
		return err
	}
	if _, err := b.dockerCommand("rm", "-f", localinfra.AgentContainer(entry.Name)); err != nil {
		return err
	}
	_, err := b.dockerCommand(args...)
	return err
}
func preparedBinaryBaseArgs(entry envstore.Entry, bundle agentbuild.BinaryBundle) []string {
	args := []string{"run", "-d", "--pull=never", "--name", localinfra.AgentContainer(entry.Name), "--network", localinfra.NetworkName(entry.Name), "--hostname", localinfra.AgentContainer(entry.Name)}
	args = append(args, agentbuild.BinaryMountArgs(bundle)...)
	return append(args, "-v", filepath.Join(entry.Dir, "agent.yaml")+":/etc/datadog-agent/datadog.yaml:ro", "-v", filepath.Join(entry.Dir, "conf.d")+":/etc/datadog-agent/conf.d")
}
func preparedBinaryRunArgs(entry envstore.Entry, bundle agentbuild.BinaryBundle) []string {
	volume := localinfra.AgentRuntimeVolume(entry.Dir, entry.Meta.CreatedAt)
	// No image-state copy-up. Restrict only the owned volume root before exec.
	return append(preparedBinaryBaseArgs(entry, bundle), "--mount", "type=volume,source="+volume.Name+",target=/opt/datadog-agent/run,volume-nocopy", "--entrypoint", "sh", bundle.RuntimeImageID, "-ec", "chmod 0700 /opt/datadog-agent/run && exec "+agentBinPathInContainer+" run -c /etc/datadog-agent/datadog.yaml")
}

// Only receiver apply of an intermediate bind-state installation uses this.
// Normal install/update must not silently discard or migrate that state.
func preparedBinaryBindStateArgs(entry envstore.Entry, bundle agentbuild.BinaryBundle) []string {
	return append(preparedBinaryBaseArgs(entry, bundle), "-v", filepath.Join(entry.Dir, "agent-run")+":/opt/datadog-agent/run", "--entrypoint", agentBinPathInContainer, bundle.RuntimeImageID, "run", "-c", "/etc/datadog-agent/datadog.yaml")
}

func (b *Binary) prepareConfig(cfg *config.File, entry envstore.Entry, section binaryconfig.Config) (string, error) {
	if errs := b.Validate(cfg); len(errs) > 0 {
		return "", config.NewErrors(errs)
	}
	routing, err := b.PrepareRouting(cfg, entry)
	if err != nil {
		return "", err
	}
	apiKey, err := bindAPIKey(routing)
	if err != nil {
		return "", err
	}
	return renderBinaryConfig(entry, section, apiKey, routing)
}

func agentRunArgs(entry envstore.Entry, image string) []string {
	dir := entry.Dir
	bind := func(hostPath, containerPath string) string { return hostPath + ":" + containerPath + ":ro" }
	return []string{
		"run", "-d",
		"--name", localinfra.AgentContainer(entry.Name),
		"--network", localinfra.NetworkName(entry.Name),
		"--hostname", localinfra.AgentContainer(entry.Name),
		"-v", bind(filepath.Join(dir, "agent-binary"), "/opt/datadog-agent/bin/agent/agent"),
		"-v", bind(filepath.Join(dir, "dev-lib"), "/opt/datadog-agent/dev-lib"),
		"-v", bind(filepath.Join(dir, "agent.yaml"), "/etc/datadog-agent/datadog.yaml"),
		"-v", filepath.Join(dir, "conf.d") + ":/etc/datadog-agent/conf.d",
		"-v", filepath.Join(dir, "agent-run") + ":/opt/datadog-agent/run",
		"-e", "LD_LIBRARY_PATH=/opt/datadog-agent/dev-lib:/opt/datadog-agent/embedded/lib",
		"--entrypoint", "/opt/datadog-agent/bin/agent/agent",
		image,
		"run", "-c", "/etc/datadog-agent/datadog.yaml",
	}
}

// defaultCoreChecks are the Go core checks that ship with conf.yaml.default in
// the official agent image. The installer writes the enabled ones into the
// environment's conf.d so system metrics flow out of the box — the mounted
// (initially empty) conf.d would otherwise replace the image's defaults with
// nothing, and only the Go heartbeat would run.
var defaultCoreChecks = map[string]string{
	"cpu":         "init_config:\ninstances:\n  - {}\n",
	"memory":      "init_config:\ninstances:\n  - {}\n",
	"disk":        "init_config:\ninstances:\n  - {}\n",
	"network":     "init_config:\ninstances:\n  - {}\n",
	"uptime":      "init_config:\ninstances:\n  - {}\n",
	"load":        "init_config:\ninstances:\n  - {}\n",
	"io":          "init_config:\ninstances:\n  - {}\n",
	"file_handle": "init_config:\ninstances:\n  - {}\n",
}

// writeAgentFiles generates agent.yaml (api key + fakeintake wiring + the
// section's config) and the conf.d folders the section declares — seeded with
// the default core-check configs so system metrics work out of the box. All
// paths are environment-local: the mounted container never touches system paths.
func writeAgentFiles(entry envstore.Entry, section binaryconfig.Config, apiKey string) error {
	agentYAML, err := renderBinaryConfig(entry, section, apiKey, nil)
	if err != nil {
		return err
	}
	return writePreparedAgentFiles(entry, section, agentYAML)
}

func renderBinaryConfig(entry envstore.Entry, section binaryconfig.Config, apiKey string, routing *receivers.Plan) (string, error) {
	var agentYAML string
	var err error
	if routing != nil {
		agentYAML, err = agentconfig.GenerateWithRouting(*routing, apiKey, section.Config)
	} else {
		endpoint := &agentconfig.Endpoint{Scheme: "http", Host: localinfra.FakeintakeContainer(entry.Name), Port: 80}
		agentYAML, err = agentconfig.Generate(apiKey, endpoint, section.Config)
	}
	if err != nil {
		return "", err
	}
	var config map[string]interface{}
	if err := yaml.Unmarshal([]byte(agentYAML), &config); err != nil {
		return "", err
	}
	if _, ok := config["hostname"]; !ok {
		config["hostname"] = localinfra.AgentContainer(entry.Name)
	}
	data, err := yaml.Marshal(config)
	return string(data), err
}

func writePreparedAgentConfig(entry envstore.Entry, agentYAML string) error {
	// Atomic replacement prevents the running bind mount from observing partial
	// config; activation recreates the container against the new inode.
	f, err := os.CreateTemp(entry.Dir, ".agent-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(agentYAML); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(entry.Dir, "agent.yaml"))
}

func writePreparedAgentFiles(entry envstore.Entry, section binaryconfig.Config, agentYAML string) error {
	if err := writePreparedAgentConfig(entry, agentYAML); err != nil {
		return err
	}
	confDir := filepath.Join(entry.Dir, "conf.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		return err
	}
	// Seed the default core checks, then the user's integrations on top (the
	// integrations may override a default check's config by using the same
	// folder name — the later write wins).
	checks := make(map[string]string, len(defaultCoreChecks)+len(section.Integrations))
	for checkName, content := range defaultCoreChecks {
		checks[checkName+".d"] = content
	}
	for folder, content := range section.Integrations {
		checks[folder] = content
	}
	for folder, content := range checks {
		if err := os.MkdirAll(filepath.Join(confDir, folder), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(confDir, folder, "conf.yaml"), []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// waitForReady uses the installed Agent client, never retained fixture data.
// Capture delivery remains a separate, explicitly unverified observation.
func (b *Binary) waitForReady(entry envstore.Entry) error {
	host := outputs.HostOutput{Transport: "docker", Address: localinfra.AgentContainer(entry.Name), OSFamily: e2eostypes.LinuxFamily}
	agent, err := client.NewHostAgentClientWithParams(standalone.NewContext(entry.Dir), host,
		agentclientparams.WithAgentBinPath(agentBinPathInContainer))
	if err != nil {
		return err
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		_, err = agent.Health()
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(time.Second)
	}
}

var (
	_ Installer = (*Binary)(nil)
	_ Updatable = (*Binary)(nil)
)

func binaryArchitecture() e2eostypes.Architecture {
	if runtime.GOARCH == "arm64" {
		return e2eostypes.ARM64Arch
	}
	return e2eostypes.AMD64Arch
}
