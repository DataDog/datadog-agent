// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package policyreceipt

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testReceipt() *Receipt {
	return &Receipt{SchemaVersion: 1, OrgID: 42, PolicyID: "policy", PolicyVersion: 7, SecretRevision: "first01", DeploymentID: "deployment"}
}
func encodedReceipt(t *testing.T, r *Receipt) string {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawStdEncoding.EncodeToString(data)
}
func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0640); err != nil {
		t.Fatal(err)
	}
}

func TestEncryptedIdentityBinding(t *testing.T) {
	r := testReceipt()
	if got, err := FromSecrets(map[string]string{SecretKey: encodedReceipt(t, r)}, "deployment"); err != nil || *got != *r {
		t.Fatal("valid identity was rejected")
	}
	if got, err := FromSecrets(nil, "deployment"); err != nil || got != nil {
		t.Fatal("classic jobs must remain unchanged")
	}
	for _, payload := range []string{encodedReceipt(t, r), "invalid", strings.Repeat("x", 4097)} {
		if _, err := FromSecrets(map[string]string{SecretKey: payload}, "other-deployment"); err == nil {
			t.Fatal("unbound identity accepted")
		}
	}
	r.VerifiedAt = 1
	if _, err := FromSecrets(map[string]string{SecretKey: encodedReceipt(t, r)}, "deployment"); err == nil {
		t.Fatal("backend supplied verification accepted")
	}
}

func TestReceiptRechecksActualFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "datadog.yaml")
	r := testReceipt()
	writeConfig(t, path, "password: private-password\n")
	if err := Record(root, r, []string{"/datadog.yaml", "/datadog.yaml"}); err != nil {
		t.Fatal(err)
	}
	before := time.Now().Add(-time.Second).UnixMilli()
	for range 3 {
		got := Verify(root, path)
		if len(got) != 1 || got[0].VerifiedAt < before || got[0].SecretRevision != r.SecretRevision {
			t.Fatal("unchanged applied file failed verification")
		}
		public, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(public), "private-password") || strings.Contains(string(public), `"mac"`) || strings.Contains(string(public), `"key"`) {
			t.Fatal("private evidence leaked")
		}
	}
	writeConfig(t, path, "password: changed-password\n")
	if len(Verify(root, path)) != 0 {
		t.Fatal("secret-only drift was not detected")
	}
	r.PolicyVersion++
	r.SecretRevision = "second2"
	if err := Record(root, r, []string{"/datadog.yaml"}); err != nil {
		t.Fatal(err)
	}
	got := Verify(root, path)
	if len(got) != 1 || got[0].PolicyVersion != 8 || got[0].SecretRevision != "second2" {
		t.Fatal("rotation did not replace prior receipt")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(root, StoreName))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0640 {
			t.Fatal("receipt store has unsafe permissions")
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if len(Verify(root, path)) != 0 {
		t.Fatal("absence was attested")
	}
	writeConfig(t, path, "password: another-password\n")
	if len(Verify(root, path)) != 0 {
		t.Fatal("changed recreated file was attested")
	}
	if err := Invalidate(root); err != nil {
		t.Fatal(err)
	}
	if len(Verify(root, path)) != 0 {
		t.Fatal("invalidated evidence retained")
	}
}

func TestReceiptUnavailableEvidence(t *testing.T) {
	for _, tc := range []string{"missing store", "malformed store", "modified authenticator", "missing file", "outside root", "oversized file"} {
		t.Run(tc, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "datadog.yaml")
			writeConfig(t, path, "password: private-password\n")
			if tc != "missing store" {
				if err := Record(root, testReceipt(), []string{"/datadog.yaml"}); err != nil {
					t.Fatal(err)
				}
			}
			switch tc {
			case "malformed store":
				writeConfig(t, filepath.Join(root, StoreName), "{")
			case "modified authenticator":
				data, err := os.ReadFile(filepath.Join(root, StoreName))
				if err != nil {
					t.Fatal(err)
				}
				var state store
				if err := json.Unmarshal(data, &state); err != nil {
					t.Fatal(err)
				}
				state.Entries[0].MAC[0] ^= 0xff
				data, err = json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}
				writeConfig(t, filepath.Join(root, StoreName), string(data))
			case "missing file":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "outside root":
				path = filepath.Join(t.TempDir(), "datadog.yaml")
				writeConfig(t, path, "password: private-password\n")
			case "oversized file":
				writeConfig(t, path, strings.Repeat("x", maxBytes+1))
			}
			if got := Verify(root, path); got == nil || len(got) != 0 {
				t.Fatal("unverifiable evidence must be explicitly empty")
			}
		})
	}
}

func TestReceiptRejectsEscapingPaths(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"../datadog.yaml", "/../datadog.yaml", StoreName} {
		if err := Record(root, testReceipt(), []string{path}); err == nil {
			t.Fatalf("unsafe path accepted: %s", path)
		}
	}
	if err := Record(root, nil, nil); err != nil {
		t.Fatal("non-policy configuration changed")
	}
}

func TestSuccessfulApplicationReplacesCorruptEvidence(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "datadog.yaml")
	writeConfig(t, filename, "password: private-password\n")
	writeConfig(t, filepath.Join(root, StoreName), "{")
	if err := Record(root, testReceipt(), []string{"/datadog.yaml"}); err != nil {
		t.Fatal(err)
	}
	if len(Verify(root, filename)) != 1 {
		t.Fatal("successful application did not repair unavailable evidence")
	}
}
