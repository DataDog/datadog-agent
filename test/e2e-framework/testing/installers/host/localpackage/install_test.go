// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localpackage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ostypes "github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
)

func fixture(t *testing.T) agentbuild.Result {
	path := filepath.Join(t.TempDir(), "agent.deb")
	os.WriteFile(path, []byte("deb fixture"), 0600)
	f, err := agentbuild.DescribeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return agentbuild.Result{Schema: 1, Target: agentbuild.Target{OS: "linux", Arch: "amd64"}, Provenance: agentbuild.Provenance{Producer: "existing-package"}, Package: &agentbuild.Package{File: f, Format: "deb", Name: "datadog-agent", Version: "7.83.0", Roles: []string{"agent"}}}
}
func TestInstallFileErrorsReturnBeforeActivation(t *testing.T) {
	for _, failure := range []string{"upload", "checksum", "metadata", "archive", "package", "status", "payload", "none"} {
		t.Run(failure, func(t *testing.T) {
			r := fixture(t)
			calls := []string{}
			remote := Transport{CopyFile: func(src, dst string) error {
				if src != r.Package.File.Path || !strings.HasPrefix(dst, "/tmp/e2ectl-package-") || !strings.HasSuffix(dst, "/agent.deb") {
					t.Fatalf("wrong upload: %s -> %s", src, dst)
				}
				calls = append(calls, "upload")
				if failure == "upload" {
					return errors.New("upload failure")
				}
				return nil
			}, Execute: func(cmd string) (string, error) {
				calls = append(calls, cmd)
				switch {
				case strings.HasPrefix(cmd, ". /etc/os-release"):
					return "ubuntu amd64", nil
				case strings.HasPrefix(cmd, "sha256sum"):
					if failure == "checksum" {
						return "wrong", nil
					}
					return r.Package.File.SHA256 + "  agent.deb", nil
				case strings.HasPrefix(cmd, "dpkg-deb"):
					if failure == "metadata" {
						return "datadog-agent arm64", nil
					}
					return "datadog-agent 7.83.0 amd64", nil
				case strings.Contains(cmd, "tar --list"):
					if failure == "archive" {
						return "lrwxrwxrwx root/root 0 now ./opt/datadog-agent/bin/agent/agent -> /outside", nil
					}
					return "-rwxr-xr-x root/root 10 now ./opt/datadog-agent/bin/agent/agent", nil
				case strings.HasPrefix(cmd, "systemctl show"):
					return "LoadState=loaded\nUnitFileState=disabled", nil
				case strings.Contains(cmd, "ln -P"):
					return "1:2", nil
				case strings.HasPrefix(cmd, "dpkg-query"):
					if failure == "status" {
						return "install ok installed\ndatadog-agent\n7.82.0\namd64", nil
					}
					return "install ok installed\ndatadog-agent\n7.83.0\namd64", nil
				case strings.Contains(cmd, "&& sha256sum /opt/"):
					if failure == "payload" {
						return strings.Repeat("b", 64), nil
					}
					return r.Package.File.SHA256, nil
				case strings.Contains(cmd, "apt-get"):
					if !strings.Contains(cmd, "--reinstall") {
						t.Fatal("same-version archive may be skipped", cmd)
					}
					if failure == "package" {
						return "", errors.New("apt failed")
					}
				}
				return "", nil
			}}
			unmask, verification, err := installFileVerified(remote, r)
			all := strings.Join(calls, "\n")
			if strings.Contains(all, "systemctl unmask") {
				t.Fatal("unconditional unmask")
			}

			if failure == "none" {
				if err != nil || unmask == nil {
					t.Fatal(err)
				}
				if verification.Scope != verificationScope || len(verification.Executables) != 1 {
					t.Fatal(verification)
				}
				if err = unmask(); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("expected normal error return")
				}
				if unmask != nil {
					t.Fatal("unmask returned on failure")
				}
				if failure != "package" && failure != "status" && failure != "payload" && strings.Contains(all, "ln -P") {
					t.Fatal("mutated service before verified upload")
				}
			}
		})
	}
}
func TestPackageTargetAndPermissionPreflight(t *testing.T) {
	for _, host := range []outputs.HostOutput{{Transport: "docker", OSFamily: ostypes.LinuxFamily, OSFlavor: ostypes.Ubuntu}, {OSFamily: ostypes.LinuxFamily, OSFlavor: ostypes.Debian}, {OSFamily: ostypes.WindowsFamily}} {
		if _, err := Target(host); err == nil {
			t.Fatal("unsupported host accepted")
		}
	}
	target, err := Target(outputs.HostOutput{OSFamily: ostypes.LinuxFamily, OSFlavor: ostypes.Ubuntu, Architecture: ostypes.AMD64Arch})
	if err != nil || target.Arch != "amd64" {
		t.Fatal(target, err)
	}
	if Validate(Params{Artifact: fixture(t)}, target) == nil {
		t.Fatal("unsigned package permission not enforced")
	}
}
