// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package packagecore installs a DEB in a disposable Docker filesystem: the
// container-target mechanism of the unified package installer. It does not
// attest receiver capability, service installation, arbitrary integrations,
// or any other emitting process.
package packagecore

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentconfig"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"go.yaml.in/yaml/v3"
)

const Scope = "core-health/configuration"
const AgentBinPath = "/opt/datadog-agent/bin/agent/agent"
const BaseImage = "docker.io/library/ubuntu:24.04"

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Runtime binds installation observations to the exact archive and Docker image.
// It is deliberately NOT a receivers.ProducerProfile and cannot authorize the
// SSH package installer or any all-signal receiver contract.
type Runtime struct {
	Scope             string         `json:"scope"`
	PackageSHA256     string         `json:"packageSHA256"`
	ExecutableSHA256  string         `json:"executableSHA256"`
	BaseImageID       string         `json:"baseImageID"`
	ImageID           string         `json:"imageID"`
	OSVersion         string         `json:"osVersion"`
	EffectiveSettings map[string]any `json:"effectiveSettings,omitempty"`
}

// Installer owns only container installation/probing, not CLI state or secrets.
type Installer struct{ Run agentbuild.RunFunc }

func (i Installer) command(ctx context.Context, args ...string) (string, error) {
	var out []byte
	var err error
	if i.Run != nil {
		out, err = i.Run(ctx, agentbuild.Invocation{Program: "docker", Args: args})
	} else {
		out, err = exec.CommandContext(ctx, "docker", args...).Output()
	}
	// Never return Docker stderr: probes can involve live private configuration.
	if err != nil {
		return "", fmt.Errorf("package-core docker %s failed: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

func Validate(r agentbuild.Result, expectedSHA256 string) error {
	if err := r.Validate(r.Target); err != nil {
		return err
	}
	if err := r.Target.Native(); err != nil {
		return err
	}
	if r.Package == nil || r.Package.Format != "deb" || r.Package.Name != "datadog-agent" {
		return fmt.Errorf("package-core requires a native datadog-agent DEB")
	}
	if !digestPattern.MatchString(expectedSHA256) || expectedSHA256 != r.Package.File.SHA256 {
		return fmt.Errorf("package-core requires the explicitly selected package SHA256")
	}
	return nil
}

// ValidateConfig applies the standard receiver rules to the user's extra
// config: valid YAML mapping, no receiver-owned destination/credential or
// trust settings, no unsupported backend sections. The container target runs
// the core Agent only, so re-enabling any setting it force-disables is
// rejected here instead of being silently overridden in the rendered config.
func ValidateConfig(raw string) error {
	m, err := receivers.ValidateConfig(raw)
	if err != nil {
		return err
	}
	for k, v := range disabledSettings() {
		if enabled, ok := m[k].(bool); ok && v != enabled {
			return fmt.Errorf("package-core config cannot re-enable %s: the container target runs the core Agent only", k)
		}
	}
	return nil
}

// These are disabled even when the package has different defaults. Only the
// core foreground process and installer-owned Go checks participate.
func disabledSettings() map[string]any {
	return map[string]any{
		"skip_ssl_validation":                         false,
		"apm_config.enabled":                          false,
		"process_config.process_collection.enabled":   false,
		"process_config.container_collection.enabled": false,
		"process_config.run_in_core_agent.enabled":    false,
		"logs_enabled":                                false,
		"container_image.enabled":                     false,
		"container_lifecycle.enabled":                 false,
		"sbom.enabled":                                false,
		"orchestrator_explorer.enabled":               false,
		"agent_telemetry.enabled":                     false,
		"inventories_enabled":                         false,
		"inventories_configuration_enabled":           false,
		"otelcollector.enabled":                       false,
		"network_path.connections_monitoring.enabled": false,
	}
}

func Config(p receivers.Plan, key, extra, hostname string) (string, error) {
	if err := ValidateConfig(extra); err != nil {
		return "", err
	}
	rendered, err := agentconfig.GenerateWithRouting(p, key, extra)
	if err != nil {
		return "", err
	}
	m := map[string]any{}
	if err = yaml.Unmarshal([]byte(rendered), &m); err != nil {
		return "", err
	}
	if _, ok := m["hostname"]; !ok {
		m["hostname"] = hostname
	}
	for k, v := range disabledSettings() {
		m[k] = v
	}
	m["confd_path"] = "/etc/datadog-agent/conf.d"
	m["additional_checksd"] = "/etc/datadog-agent/checks.d"
	data, err := yaml.Marshal(m)
	return string(data), err
}

// InstallImage performs package installation, never a source build. The build
// context contains only the chosen DEB and an installer-owned Dockerfile. No
// keys, checkout, host /opt, socket, or privileged container are involved.
func (i Installer) InstallImage(ctx context.Context, r agentbuild.Result, expectedSHA256 string) (Runtime, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var result Runtime
	if err := Validate(r, expectedSHA256); err != nil {
		return result, err
	}
	adapter := agentbuild.Adapter{Run: i.Run}
	base, err := adapter.InspectImage(ctx, BaseImage, r.Target)
	if err != nil {
		return result, err
	}
	dir, err := os.MkdirTemp("", "e2ectl-package-install-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(dir)
	// BuildKit interprets FROM sha256:<image-ID> as a repository tag. Give
	// the inspected image a unique, temporary local tag instead; never use a
	// mutable shared base tag during installation.
	baseTag := "localhost/e2ectl-package-base:" + filepath.Base(dir)
	if _, err = i.command(ctx, "tag", base.ID, baseTag); err != nil {
		return result, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		_, _ = i.command(cleanup, "image", "rm", baseTag)
	}()
	src, err := os.Open(r.Package.File.Path)
	if err != nil {
		return result, err
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(dir, "agent.deb"))
	if err != nil {
		return result, err
	}
	_, err = io.Copy(dst, src)
	closeErr := dst.Close()
	if err != nil {
		return result, err
	}
	if closeErr != nil {
		return result, closeErr
	}
	dockerfile := fmt.Sprintf(`FROM %s
RUN printf '#!/bin/sh\nexit 101\n' > /usr/sbin/policy-rc.d && chmod 0755 /usr/sbin/policy-rc.d \
 && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY agent.deb /tmp/agent.deb
RUN --network=none echo '%s  /tmp/agent.deb' | sha256sum -c - \
 && mkdir /tmp/archive && dpkg-deb -x /tmp/agent.deb /tmp/archive \
 && sha256sum /tmp/archive/opt/datadog-agent/bin/agent/agent | cut -d ' ' -f 1 > /tmp/core.sha256 \
 && dpkg -i /tmp/agent.deb \
 && test "$(dpkg-query -W -f='${Status}' datadog-agent)" = 'install ok installed' \
 && test "$(sha256sum /opt/datadog-agent/bin/agent/agent | cut -d ' ' -f 1)" = "$(cat /tmp/core.sha256)" \
 && mkdir -p /etc/datadog-agent/checks.d /opt/datadog-agent/run \
 && rm -rf /tmp/archive /tmp/agent.deb /tmp/core.sha256
`, baseTag, expectedSHA256)
	if err = os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0600); err != nil {
		return result, err
	}
	iid := filepath.Join(dir, "image-id")
	// Installation has no credentials; retain package-manager diagnostics. Live
	// configuration probes below intentionally never stream their output.
	run := i.Run
	if run == nil {
		run = agentbuild.Run
	}
	if _, err = run(ctx, agentbuild.Invocation{Program: "docker", Args: []string{"build", "--network=default", "--pull=false", "--iidfile", iid, dir}, StreamOutput: true}); err != nil {
		return result, fmt.Errorf("installing DEB in Docker filesystem: %w", err)
	}
	observedBase, err := adapter.InspectImage(ctx, baseTag, r.Target)
	if err != nil || observedBase.ID != base.ID {
		return result, fmt.Errorf("package installation base image identity changed")
	}
	data, err := os.ReadFile(iid)
	if err != nil {
		return result, err
	}
	image, err := adapter.InspectImage(ctx, strings.TrimSpace(string(data)), r.Target)
	if err != nil {
		return result, err
	}
	result = Runtime{Scope: Scope, PackageSHA256: expectedSHA256, BaseImageID: base.ID, ImageID: image.ID, OSVersion: "24.04"}
	name := filepath.Base(dir) + "-identity"
	defer i.removeContainer(name)
	out, err := i.command(ctx, "run", "--rm", "--name", name, "--pull=never", "--network=none", "--entrypoint", "sh", image.ID, "-ec", identityScript)
	if err != nil {
		return result, err
	}
	result.ExecutableSHA256, err = checkIdentity(out, r, result)
	return result, err
}

const identityScript = `test -s /etc/ssl/certs/ca-certificates.crt
. /etc/os-release
printf '%s %s\n' "$ID" "$VERSION_ID"
dpkg-query -W -f='${Status}\n${Package}\n${Version}\n${Architecture}\n' datadog-agent
sha256sum /opt/datadog-agent/bin/agent/agent`

func checkIdentity(out string, r agentbuild.Result, installed Runtime) (string, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 6 || lines[0] != "ubuntu 24.04" || lines[1] != "install ok installed" || lines[2] != r.Package.Name || lines[3] != r.Package.Version || lines[4] != r.Target.Arch {
		return "", fmt.Errorf("installed package metadata/target mismatch")
	}
	fields := strings.Fields(lines[5])
	if len(fields) != 2 || !digestPattern.MatchString(fields[0]) || fields[1] != AgentBinPath {
		return "", fmt.Errorf("installed core executable checksum missing")
	}
	if installed.ExecutableSHA256 != "" && installed.ExecutableSHA256 != fields[0] {
		return "", fmt.Errorf("installed core executable checksum mismatch")
	}
	return fields[0], nil
}

func (i Installer) Verify(ctx context.Context, r agentbuild.Result, installed Runtime, container string) error {
	if err := Validate(r, installed.PackageSHA256); err != nil {
		return err
	}
	if installed.Scope != Scope || !digestPattern.MatchString(installed.ExecutableSHA256) || !strings.HasPrefix(installed.ImageID, "sha256:") {
		return fmt.Errorf("limited package-core installation receipt missing")
	}
	image, err := i.command(ctx, "inspect", "--format", "{{.Image}}", container)
	if err != nil {
		return err
	}
	if image != installed.ImageID {
		return fmt.Errorf("running container differs from package-core image receipt")
	}
	out, err := i.command(ctx, "exec", container, "sh", "-ec", identityScript)
	if err != nil {
		return err
	}
	_, err = checkIdentity(out, r, installed)
	return err
}

// Probe runs the selected configuration with a DUMMY key and no network before
// any real credential is resolved. No state/IPC files are written into host binds.
func (i Installer) Probe(ctx context.Context, installed Runtime, p receivers.Plan, extra string) error {
	config, err := Config(p, receivers.DummyAPIKey, extra, "e2ectl-package-core-probe")
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "e2ectl-package-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err = os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(config), 0644); err != nil {
		return err
	}
	if err = PrepareChecks(filepath.Join(dir, "conf.d")); err != nil {
		return err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	name := filepath.Base(dir)
	defer i.removeContainer(name)
	script := `cp /probe/datadog.yaml /etc/datadog-agent/datadog.yaml
agent=/opt/datadog-agent/bin/agent/agent
"$agent" run >/tmp/agent.log 2>&1 & pid=$!
trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true' EXIT
for i in $(seq 1 60); do
 if "$agent" health >/dev/null 2>&1; then
  "$agent" check cpu >/tmp/cpu.log 2>&1
  grep -q 'system.cpu.' /tmp/cpu.log
  "$agent" config
  exit 0
 fi
 kill -0 "$pid" 2>/dev/null || exit 1
 sleep 1
done
exit 1`
	out, err := i.command(probeCtx, "run", "--rm", "--name", name, "--pull=never", "--network=none", "-v", dir+":/probe:ro", "-v", filepath.Join(dir, "conf.d")+":/etc/datadog-agent/conf.d:ro", "--entrypoint", "sh", installed.ImageID, "-ec", script)
	if err != nil {
		return err
	}
	_, err = EffectiveSettings(out, p)
	return err
}

func (i Installer) removeContainer(name string) {
	cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	_, _ = i.command(cleanup, "rm", "-f", name)
}

// EffectiveSettings returns only checked non-secret settings, never raw config.
// This is configuration observation, NOT a proof of all possible network behavior.
func EffectiveSettings(raw string, p receivers.Plan) (map[string]any, error) {
	m, err := receivers.ParseConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("cannot parse effective package-core configuration")
	}
	expected := p.Settings()
	for k, v := range disabledSettings() {
		expected[k] = v
	}
	for k, v := range expected {
		if !reflect.DeepEqual(m[k], v) {
			return nil, fmt.Errorf("package-core effective setting mismatch: %s", k)
		}
	}
	return expected, nil
}

func (i Installer) Observe(ctx context.Context, installed *Runtime, p receivers.Plan, container string) error {
	out, err := i.command(ctx, "exec", container, AgentBinPath, "config")
	if err != nil {
		return err
	}
	installed.EffectiveSettings, err = EffectiveSettings(out, p)
	return err
}
