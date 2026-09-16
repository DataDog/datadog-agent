// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/localinfra"
	binaryconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/binary"
	e2eostypes "github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentconfig"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"go.yaml.in/yaml/v3"
)

// agentBinPathInContainer is where the pinned agent binary lands inside the
// runtime container: the install bind-mounts it over the image's agent binary.
const agentBinPathInContainer = "/opt/datadog-agent/bin/agent/agent"

// DefaultRuntimeImage is the pinned official Agent image the locally built
// binary runs in. The binary is mounted over the image's own binary path, so
// the image provides the runtime (glibc, embedded python) — not the Agent.
// The pin location is provisional (see the local agent plan's open decision):
// it should eventually live with the other pinned runtime dependencies.
const DefaultRuntimeImage = "registry.datadoghq.com/agent:7.83.0"

// readinessTimeout bounds the wait for the agent heartbeat in the fakeintake.
// The build happens before this wait starts; this covers agent startup plus
// the first flush only.
const readinessTimeout = 3 * time.Minute

// devLibSubpath is where the rtloader shared libraries live relative to the
// repository root. The dev-container-built binary is dynamically linked
// against them (found via the worktree dev/lib at build time); inside the
// container we mount them and point LD_LIBRARY_PATH at the mount.
const devLibSubpath = "dev/lib"

// Binary installs the Agent built from the working tree, running it in a
// container on the environment's Docker network. The container is the process
// handle: `docker rm -f` stops it, `docker logs` is the debugging surface.
type Binary struct{}

// ID implements Installer.
func (b *Binary) ID() string { return "binary" }

// AgentExample implements Installer: the binary section schema's example.
func (b *Binary) AgentExample() (*yaml.Node, error) { return binaryconfig.Schema.Example(nil) }

// Artifact implements Installer. The binary installer has no version or
// image: the Agent is always built from source. The empty values make `list`
// show the "binary" label via the cmdList tweak.
func (b *Binary) Artifact(cfg *config.File) (string, string, error) {
	if _, err := decodeBinarySection(cfg); err != nil {
		return "", "", err
	}
	return "", "", nil
}

// Validate implements Installer: the binary section's own rules.
func (b *Binary) Validate(cfg *config.File) []error {
	if _, err := decodeBinarySection(cfg); err != nil {
		return []error{err}
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
	section, err := decodeBinarySection(cfg)
	if err != nil {
		return err
	}
	// The running container bind-mounts the pinned binary: it must be
	// replaced before the fresh build can be pinned (Linux refuses to
	// overwrite a file kept busy by the mount).
	if err := localinfra.RemoveContainer(localinfra.AgentContainer(entry.Name)); err != nil {
		return fmt.Errorf("replacing the agent container: %w", err)
	}
	if err := buildAndPinBinary(entry); err != nil {
		return err
	}
	if err := b.runAgentContainer(entry, section); err != nil {
		return err
	}
	if err := b.waitForFlushedMetrics(entry); err != nil {
		return err
	}
	return b.writeSnapshotOutputs(entry)
}

// writeSnapshotOutputs records the host and agent components in the
// snapshot. remoteHost makes the agent container look like a Host to the test
// framework: Transport=docker tells RemoteHost.Init to use docker exec with
// Address as the container name. agent carries the pinned binary path so the
// AgentClient invokes it directly (the container has no sudo and no
// datadog-agent wrapper). The existing StaticStackProvisioner rehydrates
// environments.Host from these resources with zero provisioner changes.
func (b *Binary) writeSnapshotOutputs(entry envstore.Entry) error {
	hostOut := outputs.HostOutput{
		CloudProvider: "local",
		Transport:     "docker",
		Address:       localinfra.AgentContainer(entry.Name),
		OSFamily:      e2eostypes.LinuxFamily,
		OSFlavor:      e2eostypes.Ubuntu,
		OSVersion:     "24.04",
		Architecture:  e2eostypes.ARM64Arch,
	}
	hostJSON, err := json.Marshal(hostOut)
	if err != nil {
		return err
	}
	if err := provisioner.UpdateSnapshotResource(entry.SnapshotPath(), "remoteHost", hostJSON); err != nil {
		return err
	}
	agentJSON, err := json.Marshal(outputs.HostAgentOutput{
		Host:         hostOut,
		AgentBinPath: agentBinPathInContainer,
	})
	if err != nil {
		return err
	}
	return provisioner.UpdateSnapshotResource(entry.SnapshotPath(), "agent", agentJSON)
}

// Update implements Updatable: prepare (rebuild unless skipBuild), replace
// the container, pin the fresh binary, restart. The installer owns the entire
// preparation path — the CLI only passes the skipBuild flag.
func (b *Binary) Update(cfg *config.File, entry envstore.Entry, skipBuild bool) error {
	section, err := decodeBinarySection(cfg)
	if err != nil {
		return err
	}
	if !skipBuild {
		if err := buildAgentBinary(); err != nil {
			return err
		}
	}
	// The running container bind-mounts the pinned binary: replace it before
	// pinning (Linux refuses to overwrite a file kept busy by the mount).
	if err := localinfra.RemoveContainer(localinfra.AgentContainer(entry.Name)); err != nil {
		return fmt.Errorf("replacing the agent container: %w", err)
	}
	if err := pinArtifacts(entry); err != nil {
		return err
	}
	if err := b.runAgentContainer(entry, section); err != nil {
		return err
	}
	if err := b.waitForFlushedMetrics(entry); err != nil {
		return err
	}
	return b.writeSnapshotOutputs(entry)
}

func (b *Binary) runAgentContainer(entry envstore.Entry, section binaryconfig.Config) error {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if err != nil {
		return fmt.Errorf("resolving Agent API key: %w", err)
	}
	if err := writeAgentFiles(entry, section, apiKey); err != nil {
		return err
	}
	// Replace any container from a previous install or a failed readiness
	// (the container is left behind on failure so its logs stay inspectable).
	if err := localinfra.RemoveContainer(localinfra.AgentContainer(entry.Name)); err != nil {
		return fmt.Errorf("replacing the agent container: %w", err)
	}
	image := section.RuntimeImage
	if image == "" {
		image = DefaultRuntimeImage
	}
	cmd := exec.Command("docker", agentRunArgs(entry, image)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("starting the agent container: %w (inspect with `docker logs %s` once it exits)", err, localinfra.AgentContainer(entry.Name))
	}
	return nil
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
	// The agent runs on the environment's Docker network: it reaches the
	// fakeintake by container DNS, not by the operator's 127.0.0.1 URL.
	endpoint := &agentconfig.Endpoint{
		Scheme: "http",
		Host:   localinfra.FakeintakeContainer(entry.Name),
		Port:   80,
	}
	agentYAML, err := agentconfig.Generate(apiKey, endpoint, section.Config)
	if err != nil {
		return err
	}
	// The hostname must be explicit: a bare container often has no resolvable
	// name, and the agent exits when it cannot determine one. Naming it after
	// the environment also makes metrics attributable in the fakeintake.
	agentYAML = "hostname: " + localinfra.AgentContainer(entry.Name) + "\n" + agentYAML
	if err := os.WriteFile(filepath.Join(entry.Dir, "agent.yaml"), []byte(agentYAML), 0o600); err != nil {
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

// buildAndPinBinary builds the Agent with the repo's sanctioned task and pins
// the binary plus its rtloader libraries into the environment directory: a
// later `git clean` cannot break a running environment, and `update
// --skip-build` reuses the pinned artifacts. The repository root is the
// working directory (the same assumption the dev-image build already makes).
func buildAndPinBinary(entry envstore.Entry) error {
	if err := buildAgentBinary(); err != nil {
		return err
	}
	return pinArtifacts(entry)
}

func pinArtifacts(entry envstore.Entry) error {
	if err := copyFile("bin/agent/agent", filepath.Join(entry.Dir, "agent-binary"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(entry.Dir, "dev-lib"), 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(devLibSubpath)
	if err != nil {
		return fmt.Errorf("reading %s (build the agent first): %w", devLibSubpath, err)
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".so") || strings.HasSuffix(e.Name(), ".so.0.1.0") {
			if err := copyFile(filepath.Join(devLibSubpath, e.Name()), filepath.Join(entry.Dir, "dev-lib", e.Name()), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("pinning %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// waitForFlushedMetrics polls the fakeintake until the Agent has flushed any
// metric at all — the heartbeat included. Waiting for a specific metric name
// would misreport readiness exactly when the developer renamed that metric,
// which is this environment's documented use case. On failure the container is
// deliberately left behind so `docker logs` remains inspectable; re-running
// install replaces it.
func (b *Binary) waitForFlushedMetrics(entry envstore.Entry) error {
	var fi outputs.FakeintakeOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "fakeIntake", &fi); err != nil {
		return err
	}
	client := fakeintakeclient.NewClient(fi.URL, fakeintakeclient.WithoutStrictFakeintakeIDCheck())
	deadline := time.Now().Add(readinessTimeout)
	for time.Now().Before(deadline) {
		if names, err := client.GetMetricNames(); err == nil && len(names) > 0 {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("no agent metrics reached the fakeintake after %s; inspect with `docker logs %s`",
		readinessTimeout, localinfra.AgentContainer(entry.Name))
}

var (
	_ Installer = (*Binary)(nil)
	_ Updatable = (*Binary)(nil)
)

// buildAgentBinary runs the repo's sanctioned agent build. It must be invoked
// from the repository root (the same working-directory assumption the dev
// image build already makes).
func buildAgentBinary() error {
	fmt.Println("building agent binary (dda inv agent.build)...")
	cmd := exec.Command("dda", "inv", "agent.build", "--build-exclude=systemd")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building the agent binary (run from the repository root): %w", err)
	}
	return nil
}
