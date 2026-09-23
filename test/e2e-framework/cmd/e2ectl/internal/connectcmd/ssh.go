// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	agentSocket, err := ensureKeyLoaded(keyPath, host, printOnly)
	if err != nil {
		return err
	}
	return writeSSHConfig(entry.Name, host, keyPath, agentSocket, printOnly)
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

// resolveSSHKeyPassphrase reads the key's passphrase from the runner profile
// secret store — the same lookup the framework's in-process SSH client uses
// to decrypt the key. The passphrase is never printed.
func resolveSSHKeyPassphrase(host outputs.HostOutput) (passphrase string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("initializing the runner profile: %v", r)
		}
	}()
	return runner.GetProfile().SecretStore().GetWithDefault(
		parameters.StoreKey(host.CloudProvider+parameters.PrivateKeyPasswordSuffix), "")
}

// keyHasPassphrase reports whether the private key needs a passphrase, by
// probing ssh-keygen with an empty one (exit 0 means it loaded unprotected).
func keyHasPassphrase(keyPath string) bool {
	cmd := exec.Command("ssh-keygen", "-y", "-P", "", "-f", keyPath)
	return cmd.Run() != nil
}

// ensureKeyLoaded makes `ssh <env>` work without typing the key passphrase.
// A protected key cannot carry its passphrase in an ssh config entry, and the
// user's agent must never be modified — it may be forwarded from another
// host. Instead e2ectl runs its OWN dedicated long-lived agent, loads the key
// there once (via an ephemeral askpass helper holding the profile
// passphrase), and the entry points ssh at it with IdentityAgent. The
// returned socket path is empty when no agent is needed (unprotected key).
// Without a stored passphrase the manual one-time command is printed — never
// a silent fallback.
func ensureKeyLoaded(keyPath string, host outputs.HostOutput, printOnly bool) (agentSocket string, err error) {
	if !keyHasPassphrase(keyPath) {
		return "", nil
	}
	passphrase, err := resolveSSHKeyPassphrase(host)
	if err != nil {
		return "", err
	}
	if passphrase == "" {
		fmt.Println("note: the private key is passphrase-protected and the runner profile stores no passphrase for it")
		fmt.Printf("load it once per session with:\n  ssh-add %s\n", keyPath)
		return "", nil
	}
	if printOnly {
		fmt.Printf("would start the dedicated e2ectl ssh-agent (if not running) and load %s into it\n", keyPath)
		return e2ectlAgentSocket(), nil
	}
	socket, err := ensureE2ectlAgent()
	if err != nil {
		fmt.Printf("note: could not start the dedicated ssh-agent (%v); load the key once with:\n  ssh-add %s\n", err, keyPath)
		return "", nil
	}
	if err := sshAddToAgent(socket, keyPath, passphrase); err != nil {
		fmt.Printf("note: could not load the key into the dedicated ssh-agent (%v); load it once with:\n  ssh-add %s\n", err, keyPath)
		return "", nil
	}
	fmt.Printf("loaded %s into the dedicated e2ectl ssh-agent (the entry wires it with IdentityAgent)\n", keyPath)
	return socket, nil
}

// e2ectlAgentSocket is the socket of the dedicated agent e2ectl owns and
// manages. It is independent from SSH_AUTH_SOCK so a forwarded user agent is
// never touched, read or written.
func e2ectlAgentSocket() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "e2ectl-agent.sock")
}

// ensureE2ectlAgent returns the socket of the running dedicated agent,
// starting one (ssh-agent -a <socket>, daemonized) when absent or dead. A
// stale socket file from a previous boot is removed first.
func ensureE2ectlAgent() (string, error) {
	socket := e2ectlAgentSocket()
	if agentAnswers(socket) {
		return socket, nil
	}
	if _, err := os.Stat(socket); err == nil {
		if err := os.Remove(socket); err != nil {
			return "", err
		}
	}
	// The daemonized child inherits stdout, so both streams go to /dev/null —
	// capturing them would block until an EOF that never comes.
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	defer devnull.Close()
	agent := exec.Command("ssh-agent", "-a", socket)
	agent.Stdout = devnull
	agent.Stderr = devnull
	if err := agent.Run(); err != nil {
		return "", fmt.Errorf("ssh-agent -a %s: %w", socket, err)
	}
	if !agentAnswers(socket) {
		return "", errors.New("ssh-agent started but does not answer on its socket")
	}
	return socket, nil
}

// agentAnswers reports whether an ssh-agent answers on the socket.
// ssh-add -l exits 0 or 1 with a reachable agent (identities or none) and 2
// when no agent is listening.
func agentAnswers(socket string) bool {
	cmd := exec.Command("ssh-add", "-l")
	cmd.Env = append(os.Environ(), "SSH_AUTH_SOCK="+socket)
	_ = cmd.Run()
	return cmd.ProcessState != nil && (cmd.ProcessState.ExitCode() == 0 || cmd.ProcessState.ExitCode() == 1)
}

// sshAddToAgent loads the key into the dedicated agent on socket. ssh-add
// has no passphrase flag, so it is driven through OpenSSH's SSH_ASKPASS
// mechanism: an ephemeral 0700 script echoes the profile passphrase, is
// removed right after, and the passphrase is never printed or stored
// anywhere else. The script is one-shot: ssh-add retries a wrong passphrase
// indefinitely, so a second invocation fails and ssh-add aborts with an error
// instead of looping forever.
func sshAddToAgent(socket, keyPath, passphrase string) error {
	dir, err := os.MkdirTemp("", "e2ectl-askpass-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	script := filepath.Join(dir, "askpass.sh")
	// A quoted heredoc keeps every passphrase byte literal, including quotes
	// and dollar signs; the delimiter cannot collide with a passphrase line
	// because it carries the marker.
	content := fmt.Sprintf("#!/bin/sh\nf=%s/used\ntest -e \"$f\" && exit 1\ntouch \"$f\"\ncat <<'e2ectl-passphrase-end'\n%s\ne2ectl-passphrase-end\n", dir, passphrase)
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		return err
	}
	cmd := exec.Command("ssh-add", keyPath)
	// Bind to the dedicated agent explicitly: the user's SSH_AUTH_SOCK (a
	// forwarded agent) must never receive the key or be modified.
	cmd.Env = append(os.Environ(),
		"SSH_AUTH_SOCK="+socket,
		"SSH_ASKPASS="+script,
		"SSH_ASKPASS_REQUIRE=force",
		"DISPLAY=:0")
	// ssh-add retries the askpass three times, then falls back to reading the
	// passphrase from its terminal. Without one (or with a wrong stored
	// passphrase) it would block forever on inherited stdin; an empty stdin
	// makes that fallback fail fast with an error instead.
	cmd.Stdin = bytes.NewReader(nil)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh-add: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// sshHostBlock renders the managed config entry for one environment.
// agentSocket is the dedicated e2ectl agent's socket (empty when the key
// needs no agent): IdentityAgent points ssh at it, so the user's own or
// forwarded agent is never consulted for this host.
func sshHostBlock(env string, host outputs.HostOutput, keyPath, agentSocket string) string {
	agentLine := ""
	if agentSocket != "" {
		agentLine = "\n    IdentityAgent " + agentSocket
	}
	return fmt.Sprintf(`# e2ectl:%s begin
Host %s
    HostName %s
    Port %d
    User %s
    IdentityFile %s
    IdentitiesOnly yes%s
    StrictHostKeyChecking accept-new
# e2ectl:%s end`, env, env, host.Address, host.Port, host.Username, keyPath, agentLine, env)
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
func writeSSHConfig(env string, host outputs.HostOutput, keyPath, agentSocket string, printOnly bool) error {
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
	updated, err := upsertSSHBlock(content, env, sshHostBlock(env, host, keyPath, agentSocket))
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
