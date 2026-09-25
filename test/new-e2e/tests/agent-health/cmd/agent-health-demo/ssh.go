// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ensureKeyInAgent loads an encrypted SSH key into ssh-agent non-interactively
// via SSH_ASKPASS. BatchMode=yes (used by runSSHCommands) suppresses passphrase
// prompts, so an encrypted key that is not already in the agent would otherwise
// fail silently. No-op when keyPath or password is empty.
func ensureKeyInAgent(keyPath, password string) error {
	if keyPath == "" || password == "" {
		return nil
	}

	// Skip if the key is already listed by the agent.
	if out, err := exec.Command("ssh-add", "-l").CombinedOutput(); err == nil && strings.Contains(string(out), keyPath) {
		return nil
	}

	// Minimal askpass script that prints the passphrase to stdout.
	f, err := os.CreateTemp("", "askpass-*.sh")
	if err != nil {
		return err
	}
	askpass := f.Name()
	defer os.Remove(askpass)
	if _, err := fmt.Fprintf(f, "#!/bin/sh\nprintf '%%s' %s\n", shellQuote(password)); err != nil {
		f.Close()
		return err
	}
	f.Close()
	if err := os.Chmod(askpass, 0o700); err != nil {
		return err
	}

	cmd := exec.Command("ssh-add", keyPath)
	cmd.Env = append(os.Environ(),
		"SSH_ASKPASS="+askpass,
		"SSH_ASKPASS_REQUIRE=force",
		"DISPLAY=dummy",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ssh-add failed: %v\n%s", err, out)
	}
	return nil
}

// runSSHCommands runs the given commands in order on host over SSH, streaming
// output. It loads an encrypted key into ssh-agent first when a password is set.
func runSSHCommands(hostIP, sshUser string, commands []string, keyPath, password string) error {
	if err := ensureKeyInAgent(keyPath, password); err != nil {
		return err
	}
	for _, c := range commands {
		args := []string{"-o", "StrictHostKeyChecking=no", "-o", "BatchMode=yes"}
		if keyPath != "" {
			args = append(args, "-i", keyPath)
		}
		args = append(args, fmt.Sprintf("%s@%s", sshUser, hostIP), c)

		cmd := exec.Command("ssh", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("command failed on %s:\n  %s\n%v", hostIP, c, err)
		}
	}
	return nil
}

// shellQuote wraps s in single quotes, escaping embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
