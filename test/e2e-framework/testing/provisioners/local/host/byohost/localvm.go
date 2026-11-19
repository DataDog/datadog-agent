// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package byohost

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	fakeintakeversion "github.com/DataDog/datadog-agent/test/fakeintake/version"
)

// localVMFakeIntakeContainer names the fakeintake Docker container started on a LOCAL_VM host, so
// a leftover from a previous, aborted run can be removed before starting a fresh one. Suites that
// share a host never run concurrently, so they share the name.
const localVMFakeIntakeContainer = "e2e-fakeintake"

// brewEnv puts Homebrew's tools (docker, colima) on PATH for a non-login ssh command. Homebrew
// lives in /opt/homebrew on Apple Silicon and /usr/local on Intel.
const brewEnv = `eval "$( (/opt/homebrew/bin/brew || /usr/local/bin/brew) shellenv 2>/dev/null)"; `

// LocalVMOption configures LocalVMProvisioner.
type LocalVMOption func(*localVMParams)

type localVMParams struct {
	remoteConfig   bool
	descriptorName string
}

// WithRemoteConfig starts the fakeintake with Remote Config enabled, signed with the same fixed
// key every other provisioner's fakeintake uses (fakeintake.DefaultRCSigningKeySeed). The Agent's
// remote_configuration roots are computed from that seed at install time, before the container
// starts, so the TUF root has to match deterministically: without the pinned key the container
// generates a random one, and every Remote Config refresh fails with "tuf: valid signatures did
// not meet threshold".
func WithRemoteConfig() LocalVMOption {
	return func(p *localVMParams) { p.remoteConfig = true }
}

// WithDescriptorName names the host descriptor file written in the test's temporary directory.
func WithDescriptorName(name string) LocalVMOption {
	return func(p *localVMParams) { p.descriptorName = name }
}

// LocalVMProvisioner returns a provisioner for the pre-existing macOS host named by the LOCAL_VM
// environment variable (a local machine or another box reachable over SSH), with a fakeintake
// container started on it, or false when LOCAL_VM is unset. No Pulumi and no cloud provider are
// involved. LOCAL_ARCH (default arm64) and LOCAL_USER (default $USER) describe the host; Docker
// must already be installed and running there, e.g. through colima.
func LocalVMProvisioner(t *testing.T, opts ...LocalVMOption) (provisioners.Provisioner, bool) {
	t.Helper()

	localVM := os.Getenv("LOCAL_VM")
	if localVM == "" {
		return nil, false
	}
	params := localVMParams{descriptorName: "macos-byo-host.json"}
	for _, opt := range opts {
		opt(&params)
	}

	arch := os.Getenv("LOCAL_ARCH")
	if arch == "" {
		arch = "arm64"
	}
	user := os.Getenv("LOCAL_USER")
	if user == "" {
		user = os.Getenv("USER")
	}

	// MacosOS is the only macOS Flavor value; the actual OS version is carried by the descriptor's
	// Version string, and "sonoma" is the only one with a descriptor entry today
	// (components/os/macos_descriptors.go).
	descriptor := oscomp.NewDescriptorWithArch(oscomp.MacosOS, "sonoma", oscomp.ArchitectureFromString(arch))
	fakeIntakePort := startLocalVMFakeIntake(t, user, localVM, params.remoteConfig)

	descriptorPath := filepath.Join(t.TempDir(), params.descriptorName)
	require.NoError(t, WriteHostDescriptorWithFakeIntake(descriptorPath, localVM, 22, user, descriptor, localVM, fakeIntakePort))
	return provisioners.NewStaticStackProvisioner[environments.Host]("", descriptorPath), true
}

// localVMSSHArgs returns the ssh flags that reach a LOCAL_VM host non-interactively. The identity
// file is the one the framework's own SSH client reads for a local host (local_private_key_path,
// from the config file), else E2E_LOCAL_PRIVATE_KEY_PATH or E2E_AWS_PRIVATE_KEY_PATH. The
// framework does not read either variable for a local host: it falls back to ssh-agent, so the
// key also has to be loaded there. IdentitiesOnly stops ssh from first offering every key the
// agent holds and being disconnected once sshd's MaxAuthTries is reached.
func localVMSSHArgs(t *testing.T) []string {
	t.Helper()

	keyPath, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.StoreKey(components.CloudProviderLocal+parameters.PrivateKeyPathSuffix), "")
	require.NoError(t, err, "reading the local host's private key path")
	for _, env := range []string{"E2E_LOCAL_PRIVATE_KEY_PATH", "E2E_AWS_PRIVATE_KEY_PATH"} {
		if keyPath != "" {
			break
		}
		keyPath = os.Getenv(env)
	}
	if keyPath == "" {
		return nil
	}
	return []string{"-i", keyPath, "-o", "IdentitiesOnly=yes"}
}

// startLocalVMFakeIntake starts a fakeintake container on the LOCAL_VM host at addr over SSH and
// returns the host port it published. It registers a t.Cleanup that removes the container.
func startLocalVMFakeIntake(t *testing.T, user, addr string, remoteConfig bool) uint32 {
	t.Helper()

	sshTarget := user + "@" + addr
	sshArgs := localVMSSHArgs(t)
	sshCommand := func(remoteCmd string) *exec.Cmd {
		return exec.Command("ssh", append(append([]string{}, sshArgs...), sshTarget, brewEnv+remoteCmd)...)
	}

	// Best-effort: a container from a previous, aborted run may still be around.
	_ = sshCommand("docker rm -f " + localVMFakeIntakeContainer).Run()

	runArgs := "docker run -d --name " + localVMFakeIntakeContainer + " -p 0:80 public.ecr.aws/datadog/fakeintake:" + fakeintakeversion.Tag
	if remoteConfig {
		runArgs += " --remoteconfig --rc-key-data=" + fakeintake.DefaultRCSigningKeySeed
	}
	out, err := sshCommand(runArgs).CombinedOutput()
	require.NoError(t, err, "starting fakeintake container on %s: %s", addr, out)
	t.Cleanup(func() {
		_ = sshCommand("docker rm -f " + localVMFakeIntakeContainer).Run()
	})

	portOut, err := sshCommand("docker port " + localVMFakeIntakeContainer + " 80/tcp").Output()
	require.NoError(t, err, "resolving fakeintake port on %s", addr)
	portStr := strings.TrimSpace(string(portOut))
	idx := strings.LastIndex(portStr, ":")
	require.NotEqual(t, -1, idx, "unexpected `docker port` output: %q", portStr)
	port, err := strconv.ParseUint(portStr[idx+1:], 10, 32)
	require.NoError(t, err, "parsing fakeintake port from %q", portStr)
	return uint32(port)
}
