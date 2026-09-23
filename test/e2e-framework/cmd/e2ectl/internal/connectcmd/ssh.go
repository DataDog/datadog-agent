// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// SSH host bases (ec2-host, docker-host) get a managed entry in
// ~/.ssh/config.d/e2ectl, included from ~/.ssh/config, so `ssh <env>` works
// after a one-time setup. Each environment owns exactly one marked block;
// re-running connect replaces that block in place and never touches others.
const (
	sshIncludeLine = "Include ~/.ssh/config.d/*"
	sshManagedFile = "e2ectl"
)

// connectHost configures shell access to a host environment from its
// "remoteHost" snapshot resource.
func connectHost(entry envstore.Entry, printOnly bool) error {
	var host outputs.HostOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "remoteHost", &host); err != nil {
		return fmt.Errorf("reading the host snapshot: %w", err)
	}
	if host.Transport == "docker" {
		// Address holds the container name; no ssh config applies.
		fmt.Printf("connect with:\n  docker exec -it %s bash\n", host.Address)
		return nil
	}
	keyPath, err := resolveSSHKeyPath(host)
	if err != nil {
		return err
	}
	if keyPath == "" {
		fmt.Println("warning: no private key configured for this host's provider, and a password cannot go into an ssh config entry")
		fmt.Printf("connect once with:\n  ssh -p %d %s@%s\n", host.Port, host.Username, host.Address)
		fmt.Printf("(set a key path with E2E_%s_PRIVATE_KEY_PATH or ~/.test_infra_config.yaml, then re-run connect)\n",
			strings.ToUpper(string(host.CloudProvider)))
		return nil
	}
	return writeSSHConfig(entry.Name, host, keyPath, printOnly)
}

// resolveSSHKeyPath looks the host's private key path up exactly like the
// framework's SSH client: the runner profile's parameter store, keyed by cloud
// provider. A profile initialization failure (GetProfile panics) becomes an
// honest error instead.
func resolveSSHKeyPath(host outputs.HostOutput) (keyPath string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("initializing the runner profile: %v", r)
		}
	}()
	return runner.GetProfile().ParamStore().GetWithDefault(
		parameters.StoreKey(host.CloudProvider+parameters.PrivateKeyPathSuffix), "")
}

// sshHostBlock renders the managed config entry for one environment.
func sshHostBlock(env string, host outputs.HostOutput, keyPath string) string {
	return fmt.Sprintf(`# e2ectl:%s begin
Host %s
    HostName %s
    Port %d
    User %s
    IdentityFile %s
    IdentitiesOnly yes
    StrictHostKeyChecking accept-new
# e2ectl:%s end`, env, env, host.Address, host.Port, host.Username, keyPath, env)
}

// sshMarkers returns the begin/end markers of env's managed block.
func sshMarkers(env string) (begin, end string) {
	return "# e2ectl:" + env + " begin", "# e2ectl:" + env + " end"
}

// upsertSSHBlock replaces env's managed block in content in place, or appends
// the block when absent. Other content is never rewritten.
func upsertSSHBlock(content, env, block string) (string, error) {
	begin, end := sshMarkers(env)
	start := strings.Index(content, begin)
	if start < 0 {
		sep := ""
		if content != "" && !strings.HasSuffix(content, "\n") {
			sep = "\n"
		}
		return content + sep + block + "\n", nil
	}
	rel := strings.Index(content[start:], end)
	if rel < 0 {
		return "", fmt.Errorf("the managed ssh entry for %q has no end marker; fix %s by hand and re-run connect", env, sshManagedFile)
	}
	stop := start + rel + len(end)
	return content[:start] + block + content[stop:], nil
}

// ensureSSHInclude appends the managed-entries Include line unless the config
// already includes the managed directory. It returns the updated content and
// whether the line was added.
func ensureSSHInclude(content string) (string, bool) {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "Include" && strings.Contains(line, "config.d") {
			return content, false
		}
	}
	sep := ""
	if content != "" && !strings.HasSuffix(content, "\n") {
		sep = "\n"
	}
	return content + sep + sshIncludeLine + "\n", true
}

// writeSSHConfig writes (or prints, with printOnly) env's managed ssh entry
// and the Include line that makes ssh load it.
func writeSSHConfig(env string, host outputs.HostOutput, keyPath string, printOnly bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".ssh", "config.d")
	managed := filepath.Join(dir, sshManagedFile)
	mainConfig := filepath.Join(home, ".ssh", "config")

	content, err := readFileOrEmpty(managed)
	if err != nil {
		return err
	}
	updated, err := upsertSSHBlock(content, env, sshHostBlock(env, host, keyPath))
	if err != nil {
		return err
	}
	mainContent, err := readFileOrEmpty(mainConfig)
	if err != nil {
		return err
	}
	mainUpdated, includeAdded := ensureSSHInclude(mainContent)

	if printOnly {
		fmt.Printf("would write to %s:\n%s\n", managed, updated)
		if includeAdded {
			fmt.Printf("would append to %s:\n%s\n", mainConfig, sshIncludeLine)
		} else {
			fmt.Printf("%s already includes the managed entries\n", mainConfig)
		}
		return nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(managed, []byte(updated), 0o600); err != nil {
		return err
	}
	fmt.Printf("wrote the %q ssh entry to %s\n", env, managed)
	if includeAdded {
		if err := os.MkdirAll(filepath.Dir(mainConfig), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(mainConfig, []byte(mainUpdated), 0o600); err != nil {
			return err
		}
		fmt.Printf("added %q to %s\n", sshIncludeLine, mainConfig)
	} else {
		fmt.Printf("%s already includes the managed entries\n", mainConfig)
	}
	fmt.Printf("connect with:\n  ssh %s\n  ssh %s 'sudo datadog-agent status'\n", env, env)
	return nil
}

func readFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}
