// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/avast/retry-go/v4"
	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/assert"
)

// testSSHUser représente un utilisateur de test pour SSH
type testSSHUser struct {
	Username          string
	HomeDir           string
	KeyPath           string
	PubKeyFingerprint string // SHA256 fingerprint of the public key (base64 hash only, e.g. "J3I5W45pnQ...")
}

// createTestUser creates a temporary system user for SSH tests
func createTestUser() (*testSSHUser, error) {
	username := fmt.Sprintf("ddtest_ssh_%d", time.Now().Unix())

	// 1. Create a system user with a home directory
	cmd := exec.Command("sudo", "useradd", "-r", "-m", "-s", "/bin/bash", "-K", "MAIL_DIR=/dev/null", username)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("useradd failed: %v (out: %s)", err, string(out))
	}

	u, err := user.Lookup(username)
	if err != nil {
		_ = exec.Command("sudo", "userdel", "-r", username).Run()
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	homeDir := u.HomeDir
	uid := u.Uid
	gid := u.Gid
	sshDir := filepath.Join(homeDir, ".ssh")

	// 2. Create ~/.ssh with mode 700
	if err := exec.Command("sudo", "mkdir", "-p", sshDir).Run(); err != nil {
		_ = exec.Command("sudo", "userdel", "-r", username).Run()
		return nil, fmt.Errorf("mkdir .ssh: %w", err)
	}
	if err := exec.Command("sudo", "chmod", "700", sshDir).Run(); err != nil {
		_ = exec.Command("sudo", "userdel", "-r", username).Run()
		return nil, fmt.Errorf("chmod .ssh: %w", err)
	}

	// 3. Generate an ed25519 key pair in a temp directory
	tmpDir, err := os.MkdirTemp("", "ssh_test_keys_*")
	if err != nil {
		_ = exec.Command("sudo", "userdel", "-r", username).Run()
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	keyPath := filepath.Join(tmpDir, "id_test_ed25519")
	pubPath := keyPath + ".pub"

	cmd = exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-f", keyPath, "-q", "-C", "test-key-"+username)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(tmpDir)
		_ = exec.Command("sudo", "userdel", "-r", username).Run()
		return nil, fmt.Errorf("ssh-keygen: %v (out: %s)", err, string(out))
	}

	// 4. Install the public key as authorized_keys (mode 600, correct owner)
	authzPath := filepath.Join(sshDir, "authorized_keys")
	cmd = exec.Command("sudo", "cp", pubPath, authzPath)
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(tmpDir)
		_ = exec.Command("sudo", "userdel", "-r", username).Run()
		return nil, fmt.Errorf("copy authorized_keys: %w", err)
	}
	if err := exec.Command("sudo", "chmod", "600", authzPath).Run(); err != nil {
		_ = os.RemoveAll(tmpDir)
		_ = exec.Command("sudo", "userdel", "-r", username).Run()
		return nil, fmt.Errorf("chmod authorized_keys: %w", err)
	}
	if err := exec.Command("sudo", "chown", "-R", uid+":"+gid, sshDir).Run(); err != nil {
		_ = os.RemoveAll(tmpDir)
		_ = exec.Command("sudo", "userdel", "-r", username).Run()
		return nil, fmt.Errorf("chown .ssh: %w", err)
	}

	// 5. Extract the SHA256 fingerprint of the public key (base64 hash only)
	//    ssh-keygen -lf outputs: "256 SHA256:<base64hash> comment (ED25519)"
	fingerprintOut, err := exec.Command("ssh-keygen", "-lf", pubPath, "-E", "sha256").Output()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		_ = exec.Command("sudo", "userdel", "-r", username).Run()
		return nil, fmt.Errorf("ssh-keygen fingerprint: %w", err)
	}
	fingerprintStr := strings.TrimSpace(string(fingerprintOut))
	var pubKeyFingerprint string
	for _, field := range strings.Fields(fingerprintStr) {
		if strings.HasPrefix(field, "SHA256:") {
			pubKeyFingerprint = strings.TrimPrefix(field, "SHA256:")
			break
		}
	}
	if pubKeyFingerprint == "" {
		_ = os.RemoveAll(tmpDir)
		_ = exec.Command("sudo", "userdel", "-r", username).Run()
		return nil, fmt.Errorf("failed to extract SHA256 fingerprint from: %s", fingerprintStr)
	}
	return &testSSHUser{
		Username:          username,
		HomeDir:           homeDir,
		KeyPath:           keyPath,
		PubKeyFingerprint: pubKeyFingerprint,
	}, nil
}

func (u *testSSHUser) cleanup() error {
	// Delete temporary keys
	if u.KeyPath != "" {
		tmpDir := filepath.Dir(u.KeyPath)
		_ = os.RemoveAll(tmpDir)
	}

	// Kill all processes of the user
	_ = exec.Command("sudo", "pkill", "-u", u.Username).Run()

	// Wait until processes are killed
	time.Sleep(500 * time.Millisecond)

	// Force kill if processes are still running
	_ = exec.Command("sudo", "pkill", "-9", "-u", u.Username).Run()

	time.Sleep(200 * time.Millisecond)

	// Delete user and home directory
	cmd := exec.Command("sudo", "userdel", "-r", u.Username)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Si userdel -r échoue, essayer avec -f (force) sans -r
		cmd = exec.Command("sudo", "userdel", "-f", u.Username)
		if out2, err2 := cmd.CombinedOutput(); err2 != nil {
			return fmt.Errorf("userdel failed: %v (out: %s), force attempt: %v (out: %s)", err, string(out), err2, string(out2))
		}

		// Manually delete the home directory if the user was deleted with -f
		if u.HomeDir != "" && u.HomeDir != "/" && u.HomeDir != "/home" {
			_ = exec.Command("sudo", "rm", "-rf", u.HomeDir).Run()
		}
	}

	return nil
}

// SSHUserSessionExpected contient les valeurs attendues pour vérifier une session SSH
type SSHUserSessionExpected struct {
	SessionType       *string  // If nil, only check if the field exists and is "ssh"
	AuthMethod        *string  // If nil, only check if the field exists and is either "password" or "public_key" or "unknown"
	ClientIP          *string  // If nil, only check it's local host
	ClientPort        *float64 // If nil, only check if port > 0
	SessionID         *string  // If nil, only check if ssh_session_id > 0
	ExpectedPublicKey *string  // If not nil and AuthMethod == "public_key", checks that the public key matches this value
}

// checkSSHUserSessionJSON check if all the fields in the JSON are valid for a SSH Session
func checkSSHUserSessionJSON(testMod *testModule, t testing.TB, data []byte, expected *SSHUserSessionExpected) {
	if expected == nil {
		expected = &SSHUserSessionExpected{}
	}

	jsonPathValidation(testMod, data, func(_ *testModule, jsonData interface{}) {

		// Check all the fields
		var sshSessionID string
		var sshClientIP string
		var sshClientPort float64

		if el, err := jsonpath.JsonPathLookup(jsonData, `$.process.user_session.ssh_session_id`); err != nil || el == nil {
			t.Errorf("user_session.ssh_session_id not found: %v", err)
		} else {
			var ok bool
			sshSessionID, ok = el.(string)
			if !ok || sshSessionID == "" || sshSessionID == "0" {
				t.Errorf("user_session.user_session_id is empty or invalid: %v", el)
			}
			if expected.SessionID != nil && sshSessionID != *expected.SessionID {
				t.Errorf("user_session.ssh_session_id mismatch: got %v, want %v", sshSessionID, *expected.SessionID)
			}
		}

		if el, err := jsonpath.JsonPathLookup(jsonData, `$.process.user_session.id`); err != nil || el == nil {
			t.Errorf("user_session.id not found: %v", err)
		} else if id, ok := el.(string); !ok || id != sshSessionID {
			t.Errorf("user_session.id is different from ssh_session_id: got %v, want %v", el, sshSessionID)
		}

		if el, err := jsonpath.JsonPathLookup(jsonData, `$.process.user_session.session_type`); err != nil || el == nil {
			t.Errorf("user_session.session_type not found: %v", err)
		} else if sessionType, ok := el.(string); !ok {
			t.Errorf("user_session.session_type is not a string: %v", el)
		} else {
			expectedType := "ssh"
			if expected.SessionType != nil {
				expectedType = *expected.SessionType
			}
			if sessionType != expectedType {
				t.Errorf("user_session.session_type mismatch: got %v, want %v", sessionType, expectedType)
			}
		}

		if el, err := jsonpath.JsonPathLookup(jsonData, `$.process.user_session.ssh_client_port`); err != nil || el == nil {
			t.Errorf("user_session.ssh_port not found: %v", err)
		} else {
			var ok bool
			sshClientPort, ok = el.(float64)
			if !ok || sshClientPort <= 0 {
				t.Errorf("user_session.ssh_client_port is invalid: %v", el)
			}
			if expected.ClientPort != nil && sshClientPort != *expected.ClientPort {
				t.Errorf("user_session.ssh_client_port mismatch: got %v, want %v", sshClientPort, *expected.ClientPort)
			}
		}

		if el, err := jsonpath.JsonPathLookup(jsonData, `$.process.user_session.ssh_client_ip`); err != nil || el == nil {
			t.Errorf("user_session.ssh_client_ip not found: %v", err)
		} else {
			var ok bool
			sshClientIP, ok = el.(string)
			if !ok || sshClientIP == "" {
				t.Errorf("user_session.ssh_client_ip is empty: %v", el)
			} else if expected.ClientIP != nil {
				if sshClientIP != *expected.ClientIP {
					t.Errorf("user_session.ssh_client_ip mismatch: got %v, want %v", sshClientIP, *expected.ClientIP)
				}
			} else if sshClientIP != "127.0.0.1" && sshClientIP != "::1" {
				t.Errorf("user_session.ssh_client_ip should be localhost (127.0.0.1 or ::1): %v", sshClientIP)
			}
		}

		// Check Port and IP as identity

		if el, err := jsonpath.JsonPathLookup(jsonData, `$.process.user_session.identity`); err != nil || el == nil {
			t.Errorf("user_session.identity not found: %v", err)
		} else if identity, ok := el.(string); !ok || identity == "" {
			t.Errorf("user_session.identity is empty: %v", el)
		}

		if el, err := jsonpath.JsonPathLookup(jsonData, `$.process.user_session.ssh_auth_method`); err != nil || el == nil {
			t.Errorf("user_session.ssh_auth_method not found: %v", err)
		} else if authMethod, ok := el.(string); !ok || authMethod == "" {
			t.Errorf("user_session.ssh_auth_method is empty: %v", el)
		} else {
			if expected.AuthMethod != nil {
				if authMethod != *expected.AuthMethod {
					t.Errorf("user_session.ssh_auth_method mismatch: got %v, want %v", authMethod, *expected.AuthMethod)
				}
			} else if authMethod != "public_key" && authMethod != "password" && authMethod != "unknown" {
				t.Errorf("user_session.ssh_auth_method has unexpected value: %v", authMethod)
			}
		}

		if expected.ExpectedPublicKey != nil {
			if authMethod, err := jsonpath.JsonPathLookup(jsonData, `$.process.user_session.ssh_auth_method`); err == nil {
				if authMethodStr, ok := authMethod.(string); ok && authMethodStr == "public_key" {
					if el, err := jsonpath.JsonPathLookup(jsonData, `$.process.user_session.ssh_public_key`); err != nil || el == nil {
						t.Errorf("user_session.ssh_public_key not found for publickey auth: %v", err)
					} else if pubKey, ok := el.(string); !ok || pubKey == "" {
						t.Errorf("user_session.ssh_public_key is empty for publickey auth: %v", el)
					} else if pubKey != *expected.ExpectedPublicKey {
						t.Errorf("user_session.ssh_public_key mismatch: got %v, want %v", pubKey, *expected.ExpectedPublicKey)
					}
				}
			}
		}
	})
}

// sshConnectAsTestUser se connecte en SSH à localhost en tant que l'utilisateur de test
func sshConnectAsTestUser(testUser *testSSHUser, remoteCmd string) error {
	// Se connecter en SSH avec la clé de test
	args := []string{
		"-i", testUser.KeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "PasswordAuthentication=no",
		"-o", "PubkeyAuthentication=yes",
		"-o", "BatchMode=yes",
		"-o", "LogLevel=ERROR",
		testUser.Username + "@localhost",
		remoteCmd,
	}

	cmd := exec.Command("ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func rotateAuthLog(logPath string) error {
	st, err := os.Stat(logPath)
	if err != nil {
		return fmt.Errorf("stat before rotate: %w", err)
	}
	mode := st.Mode().Perm()

	uid, gid := 0, 0
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		uid = int(sys.Uid)
		gid = int(sys.Gid)
	}

	if err := os.Rename(logPath, logPath+".1"); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create new log: %w", err)
	}
	_ = f.Close()

	if err := os.Chown(logPath, uid, gid); err != nil {
		return fmt.Errorf("chown new log: %w", err)
	}
	if err := os.Chmod(logPath, mode); err != nil {
		return fmt.Errorf("chmod new log: %w", err)
	}

	if _, err := exec.LookPath("restorecon"); err == nil {
		_ = exec.Command("restorecon", "-v", logPath).Run()
	}

	if err := exec.Command("systemctl", "reload", "rsyslog").Run(); err != nil {
		err = exec.Command("bash", "-c", "pidof rsyslogd >/dev/null 2>&1 && kill -HUP $(pidof rsyslogd)").Run()
		if err != nil {
			return fmt.Errorf("reload rsyslog: %w", err)
		}
	}

	return nil
}

// restoreRotatedLog restores the rotated log file to its original location
func restoreRotatedLog(logPath string) error {
	rotatedPath := logPath + ".1"

	// Check if rotated file exists
	if _, err := os.Stat(rotatedPath); os.IsNotExist(err) {
		return nil // Nothing to restore
	}
	// Remove the new empty log
	_ = os.Remove(logPath)

	// Rename .1 back to original
	if err := os.Rename(rotatedPath, logPath); err != nil {
		return fmt.Errorf("restore log: %w", err)
	}

	// Reload rsyslog
	if err := exec.Command("systemctl", "reload", "rsyslog").Run(); err != nil {
		err = exec.Command("bash", "-c", "pidof rsyslogd >/dev/null 2>&1 && kill -HUP $(pidof rsyslogd)").Run()
		if err != nil {
			return fmt.Errorf("reload rsyslog: %w", err)
		}
	}

	return nil
}

func getLogFile() (bool, string, uint64) {
	possibleLogPaths := []string{
		"/var/log/auth.log", // Debian/Ubuntu
		"/var/log/secure",   // RHEL/CentOS/Fedora
		"/var/log/messages", // openSUSE/autres
	}

	var logPath string
	var inodeBeforeRotate uint64

	for _, path := range possibleLogPaths {
		stat, err := os.Stat(path)
		if err == nil {
			logPath = path
			// Get inode
			if sysStat, ok := stat.Sys().(*syscall.Stat_t); ok {
				inodeBeforeRotate = sysStat.Ino
				return true, logPath, inodeBeforeRotate
			}
		}
	}
	return false, "", 0
}

func TestSSHUserSession(t *testing.T) {
	SkipIfNotAvailable(t)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}

	isLogFileExist, _, _ := getLogFile()
	// We skip test when we don't have a log file because we don't use journalctl for now
	if !isLogFileExist {
		t.Skip("Skip test if log file does not exist")
	}

	testUser, err := createTestUser()
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer func() {
		if err := testUser.cleanup(); err != nil {
			t.Logf("warning: failed to cleanup test user: %v", err)
		}
	}()

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_ssh_user_session",
			Expression: `process.user_session.ssh_session_id != 0 && exec.user == "` + testUser.Username + `"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("ssh_then_pwd", func(t *testing.T) {
		err := test.GetEventSent(t, func() error {
			if err := sshConnectAsTestUser(testUser, "pwd"); err != nil {
				fmt.Fprintf(os.Stderr, "ssh failed: %v\n", err)
				return err
			}
			return nil
		}, func(_ *rules.Rule, _ *model.Event) bool {
			return true
		}, time.Second*3, "test_rule_ssh_user_session")

		if err != nil {
			t.Fatal(err)
		}
		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("test_rule_ssh_user_session")
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			// Check all the fields
			expectedAuthType := "public_key"
			expected := &SSHUserSessionExpected{
				AuthMethod:        &expectedAuthType,
				ExpectedPublicKey: &testUser.PubKeyFingerprint,
			}
			checkSSHUserSessionJSON(test, t, msg.Data, expected)

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)

	})
}

func TestSSHUserSessionRotated(t *testing.T) {
	SkipIfNotAvailable(t)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}

	isLogFileExist, logPath, inodeBeforeRotate := getLogFile()
	// We skip test when we don't have a log file because we can't rotate it
	if !isLogFileExist {
		t.Skip("Skip test if log file does not exist")
	}

	testUser, err := createTestUser()
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer func() {
		if err := testUser.cleanup(); err != nil {
			t.Logf("warning: failed to cleanup test user: %v", err)
		}
	}()

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_ssh_user_session",
			Expression: `process.user_session.ssh_session_id != 0 && exec.user == "` + testUser.Username + `"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// Cleanup: restore log after test completion
	t.Cleanup(func() {
		_ = restoreRotatedLog(logPath)
	})

	if err := rotateAuthLog(logPath); err != nil {
		t.Fatalf("rotateAuthLog failed: %v", err)
	}

	stat, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("failed to stat log file after rotation: %v", err)
	}

	var inodeAfterRotate uint64
	if sysStat, ok := stat.Sys().(*syscall.Stat_t); ok {
		inodeAfterRotate = sysStat.Ino
	}

	// Check that the inode has changed
	assert.NotEqual(t, inodeBeforeRotate, inodeAfterRotate, "inode of %s should be different after rotate", logPath)

	t.Run("ssh_then_pwd_after_rotation", func(t *testing.T) {
		err := test.GetEventSent(t, func() error {
			if err := sshConnectAsTestUser(testUser, "pwd"); err != nil {
				fmt.Fprintf(os.Stderr, "ssh failed: %v\n", err)
				return err
			}
			return nil
		}, func(_ *rules.Rule, _ *model.Event) bool {
			return true
		}, time.Second*3, "test_rule_ssh_user_session")

		if err != nil {
			t.Fatal(err)
		}
		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("test_rule_ssh_user_session")
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			// Check all the fields
			expectedAuthType := "public_key"
			expected := &SSHUserSessionExpected{
				AuthMethod:        &expectedAuthType,
				ExpectedPublicKey: &testUser.PubKeyFingerprint,
			}

			checkSSHUserSessionJSON(test, t, msg.Data, expected)

			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)

	})
}

func TestSSHUserSessionBlocking(t *testing.T) {
	SkipIfNotAvailable(t)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}

	isLogFileExist, _, _ := getLogFile()
	// We skip test when we don't have a log file because we don't use journalctl for now
	if !isLogFileExist {
		t.Skip("Skip test if log file does not exist")
	}

	testUser, err := createTestUser()
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer func() {
		if err := testUser.cleanup(); err != nil {
			t.Logf("warning: failed to cleanup test user: %v", err)
		}
	}()

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_blocking_ssh_user_session",
			Expression: `process.user_session.ssh_session_id != 0 && process.comm == "ls" && exec.user == "` + testUser.Username + `"`,
		},
	}
	controlPath := filepath.Join(os.TempDir(),
		fmt.Sprintf("cm-%s-%d", testUser.Username, time.Now().UnixNano()),
	)
	defer os.Remove(controlPath)

	baseOpts := []string{
		"-i", testUser.KeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "PasswordAuthentication=no",
		"-o", "PubkeyAuthentication=yes",
		"-o", "BatchMode=yes",
		"-o", "LogLevel=ERROR",
		"-o", "ControlPath=" + controlPath,
	}

	host := testUser.Username + "@localhost"

	// 2) Start master in background: -M (master), -N (no command), -f (fork background)
	masterArgs := append([]string{}, baseOpts...)
	masterArgs = append(masterArgs,
		"-o", "ControlMaster=yes",
		"-o", "ControlPersist=2m",
		"-N", "-f",
		host,
	)

	cmd := exec.Command("ssh", masterArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to start SSH master: %v", err)
	}

	// 3) Wait socket exists
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(controlPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Control socket not created at %s", controlPath)
		}
		time.Sleep(50 * time.Millisecond)
	}

	test, err := newTestModule(t, nil, ruleDefs, withForceReload())
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("second_ssh_multiplexed", func(t *testing.T) {
		// 5) Must use master
		args := append([]string{}, baseOpts...)
		args = append(args,
			"-o", "ControlMaster=auto",
			host,
			"ls",
		)

		cmd := exec.Command("ssh", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("Second SSH command failed: %v", err)
		}

		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("test_rule_blocking_ssh_user_session")
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			expectedAuthType := "public_key"
			expected := &SSHUserSessionExpected{
				AuthMethod:        &expectedAuthType,
				ExpectedPublicKey: &testUser.PubKeyFingerprint,
			}
			checkSSHUserSessionJSON(test, t, msg.Data, expected)
			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})

	exitArgs := append([]string{}, baseOpts...)
	exitArgs = append(exitArgs, "-O", "exit", host)
	_ = exec.Command("ssh", exitArgs...).Run()

}

func TestSSHUserSessionSnapshot(t *testing.T) {
	SkipIfNotAvailable(t)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}

	isLogFileExist, _, _ := getLogFile()
	// We skip test when we don't have a log file because we don't use journalctl for now
	if !isLogFileExist {
		t.Skip("Skip test if log file does not exist")
	}

	testUser, err := createTestUser()
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer func() {
		if err := testUser.cleanup(); err != nil {
			t.Logf("warning: failed to cleanup test user: %v", err)
		}
	}()

	cmdChan := make(chan string, 10)
	stopChan := make(chan struct{})
	doneChan := make(chan struct{})

	args := []string{
		"-i", testUser.KeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "PasswordAuthentication=no",
		"-o", "PubkeyAuthentication=yes",
		"-o", "LogLevel=ERROR",
		testUser.Username + "@localhost",
		"bash -s",
	}

	cmd := exec.Command("ssh", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start SSH session: %v", err)
	}
	// Go routine to send commands to the SSH session
	go func() {
		defer close(doneChan)
		for {
			select {
			case <-stopChan:
				stdin.Close()
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
				return
			case command := <-cmdChan:
				_, err := fmt.Fprintf(stdin, "%s\n", command)
				if err != nil {
					t.Logf("Failed to send command '%s': %v", command, err)
				}
			}
		}
	}()

	defer func() {
		close(stopChan)
		<-doneChan
	}()

	// Wait for ssh session to start
	time.Sleep(2 * time.Second)

	// Helper to send commands
	sendCommand := func(command string) {
		cmdChan <- command
		time.Sleep(100 * time.Millisecond)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_ssh_user_session_snapshot",
			Expression: `exec.file.name != "" && process.comm == "ls"`,
		},
	}

	commands := []string{
		"ls",
		"whoami",
		"pwd",
		"pwd",
	}

	for _, command := range commands {
		sendCommand(command)
	}

	test, err := newTestModule(t, nil, ruleDefs, withForceReload())
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("ssh_session_started_before_agent", func(t *testing.T) {
		err = test.GetEventSent(t, func() error {
			sendCommand("ls")
			commands = append(commands, "ls")

			return nil
		}, func(_ *rules.Rule, event *model.Event) bool {
			eventSSHSessionID := event.ProcessContext.UserSession.SSHSessionID

			if eventSSHSessionID == 0 {
				t.Logf("SSH Session ID is 0 for %s", event.ProcessContext.Comm)
				return false
			}
			// Now we check if we got all the executed commands
			i := len(commands) - 1
			ancestor := event.ProcessContext.Ancestor
			for ancestor != nil && !strings.HasPrefix(ancestor.Comm, "sshd") {
				if ancestor.UserSession.SSHSessionID != eventSSHSessionID && ancestor.ProcessContext.Comm != commands[i] {
					t.Logf("SSH Session incorrect: expected %s with id %d, got %s with id %d",
						ancestor.Comm, ancestor.UserSession.SSHSessionID, event.ProcessContext.Comm, eventSSHSessionID)
					return false
				}
				ancestor = ancestor.Ancestor
			}
			// Now we check if we end with sshd
			if ancestor == nil {
				t.Log("ancestor is nil")
				return false
			}
			if !strings.HasPrefix(ancestor.Comm, "sshd") || ancestor.UserSession.SSHSessionID != 0 {
				t.Logf("sshd not found with id 0, got %s with id %d (current session had id %d)", ancestor.Comm, ancestor.UserSession.SSHSessionID, eventSSHSessionID)
				return false
			}
			return true
		}, time.Second*10, "test_rule_ssh_user_session_snapshot")

		if err != nil {
			t.Error(err)
		}
		err = retry.Do(func() error {
			msg := test.msgSender.getMsg("test_rule_ssh_user_session_snapshot")
			if msg == nil {
				return errors.New("not found")
			}
			validateMessageSchema(t, string(msg.Data))

			expectedAuthType := "public_key"
			expected := &SSHUserSessionExpected{
				AuthMethod:        &expectedAuthType,
				ExpectedPublicKey: &testUser.PubKeyFingerprint,
			}
			checkSSHUserSessionJSON(test, t, msg.Data, expected)
			return nil
		}, retry.Delay(200*time.Millisecond), retry.Attempts(30), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})
}
