// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/usersession"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func ensureLocalhostSSHAuth() error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	home := u.HomeDir
	sshDir := filepath.Join(home, ".ssh")
	keyPath := filepath.Join(sshDir, "ci_localhost_ed25519")
	pubPath := keyPath + ".pub"
	authz := filepath.Join(sshDir, "authorized_keys")

	// 1) ~/.ssh with good rights
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", sshDir, err)
	}
	if err := os.Chmod(sshDir, 0o700); err != nil {
		return fmt.Errorf("chmod %s: %w", sshDir, err)
	}

	// 2) Generate a key if missing (readable format for OpenSSH)
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-f", keyPath, "-q")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ssh-keygen: %v (out: %s)", err, string(out))
		}
		fmt.Printf("Generated key : %s\n", keyPath)
		_ = os.Chmod(keyPath, 0o600)
		_ = os.Chmod(pubPath, 0o644)
	}

	// 3) Add pubkey if missing
	pub, err := os.ReadFile(pubPath)
	if err != nil {
		return fmt.Errorf("read pub: %w", err)
	}
	if _, err := os.Stat(authz); os.IsNotExist(err) {
		if err := os.WriteFile(authz, pub, 0o600); err != nil {
			return fmt.Errorf("write authorized_keys: %w", err)
		}
	} else {
		existing, err := os.ReadFile(authz)
		if err != nil {
			return fmt.Errorf("read authorized_keys: %w", err)
		}
		if !bytes.Contains(existing, pub) {
			f, err := os.OpenFile(authz, os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				return fmt.Errorf("open authorized_keys for append: %w", err)
			}
			defer f.Close()
			if _, err := f.Write(append(pub, '\n')); err != nil {
				return fmt.Errorf("append authorized_keys: %w", err)
			}
		}
	}
	// Strict rights needs for ssh
	if err := os.Chmod(authz, 0o600); err != nil {
		return fmt.Errorf("chmod authorized_keys: %w", err)
	}

	return nil
}

func sshLocalhostWithGeneratedKey(remoteCmd string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	keyPath := filepath.Join(u.HomeDir, ".ssh", "ci_localhost_ed25519")

	// Force key authentification
	args := []string{
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "PasswordAuthentication=no",
		"-o", "PubkeyAuthentication=yes",
		"-o", "BatchMode=yes",
		"-o", "LogLevel=ERROR",
		u.Username + "@localhost",
		remoteCmd,
	}

	cmd := exec.Command("ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func TestPam(t *testing.T) {
	SkipIfNotAvailable(t)

	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("failed to get current user: %v", err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_pam",
			Expression: `exec.user_session.ssh_username == "` + currentUser.Username + `"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("pam_session_open_command_close", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			if err := ensureLocalhostSSHAuth(); err != nil {
				fmt.Fprintf(os.Stderr, "setup ssh failed: %v\n", err)
				return err
			}
			if err := sshLocalhostWithGeneratedKey("echo 'test ssh connection'"); err != nil {
				fmt.Fprintf(os.Stderr, "ssh failed: %v\n", err)
				return err
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_pam")
			assert.NotEqual(t, 0, event.ProcessContext.UserSession.ID)
			assert.Equal(t, usersession.UserSessionTypes["ssh"], event.ProcessContext.UserSession.SessionType)
			assert.Equal(t, currentUser.Username, event.ProcessContext.UserSession.SSHUsername)
			assert.Contains(t, []string{"127.0.0.1", "::1"}, event.ProcessContext.UserSession.SSHClientIP)
			assert.Equal(t, event.ProcessContext.UserSession.WhereIsLog, 1)
		})
	})
}

func tryReadN(r *bufio.Reader, n int) string {
	_, _ = r.Peek(1) // force fill
	var b strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, n)
		m, _ := r.Read(buf)
		b.Write(buf[:m])
	}()
	select {
	case <-ctx.Done():
	case <-done:
	}
	return b.String()
}
