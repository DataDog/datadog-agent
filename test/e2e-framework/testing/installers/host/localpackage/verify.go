// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localpackage

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
)

// Verification covers package-manager status and every declared executable,
// not conffiles or the complete postinst-mutated runtime filesystem.
type Verification struct {
	Scope        string                   `json:"scope"`
	Package      string                   `json:"package"`
	Version      string                   `json:"version"`
	Architecture string                   `json:"architecture"`
	Executables  []ExecutableVerification `json:"executables"`
}
type ExecutableVerification struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

const verificationScope = "package-status-and-executable-sha256"

var payloadSHA256 = regexp.MustCompile(`^[a-f0-9]{64}$`)

func hashOutput(out string) (string, error) {
	fields := strings.Fields(out)
	if len(fields) < 1 || !payloadSHA256.MatchString(fields[0]) {
		return "", fmt.Errorf("invalid executable checksum output")
	}
	return fields[0], nil
}

// prepareExecutableVerification extracts only selected regular members to
// stdout, redirected to installer-owned private paths. Archive paths are never
// used as filesystem destinations; symlink/hardlink/duplicate members fail.
func prepareExecutableVerification(remote Transport, r agentbuild.Result, archive, dir string) (*Verification, error) {
	v := &Verification{Scope: verificationScope, Package: r.Package.Name, Version: r.Package.Version, Architecture: r.Target.Arch}
	for _, role := range r.Package.Roles {
		path, err := agentbuild.PackageExecutablePath(role)
		if err != nil {
			return nil, err
		}
		member := "." + path
		listing, err := remote.Execute("bash -o pipefail -c 'dpkg-deb --fsys-tarfile " + archive + " | tar --list --verbose --file=- -- " + member + "'")
		if err != nil {
			return nil, fmt.Errorf("inspecting package role %s: %w", role, err)
		}
		lines := strings.Split(strings.TrimSpace(listing), "\n")
		fields := strings.Fields(lines[0])
		if len(lines) != 1 || len(fields) < 2 || !strings.HasPrefix(fields[0], "-") || fields[len(fields)-1] != member {
			return nil, fmt.Errorf("package role %s must be one regular archive member", role)
		}
		stage := dir + "/payload-" + role
		if _, err = remote.Execute("bash -o pipefail -c 'set -o noclobber; dpkg-deb --fsys-tarfile " + archive + " | tar --extract --to-stdout --file=- -- " + member + " > " + stage + "'"); err != nil {
			return nil, fmt.Errorf("extracting package role %s: %w", role, err)
		}
		output, err := remote.Execute("sha256sum " + stage)
		if err != nil {
			return nil, err
		}
		sum, err := hashOutput(output)
		if err != nil {
			return nil, err
		}
		v.Executables = append(v.Executables, ExecutableVerification{Role: role, Path: path, SHA256: sum})
	}
	return v, nil
}
func verifyInstalledPackage(remote Transport, v *Verification) error {
	out, err := remote.Execute("dpkg-query --show --showformat='${Status}\\n${Package}\\n${Version}\\n${Architecture}' datadog-agent")
	if err != nil {
		return fmt.Errorf("verifying installed package status: %w", err)
	}
	want := "install ok installed\n" + v.Package + "\n" + v.Version + "\n" + v.Architecture
	if strings.TrimSpace(out) != want {
		return fmt.Errorf("installed package status/version/architecture differs from selected archive")
	}
	for _, file := range v.Executables {
		// Do not accept a symlink pointing elsewhere as an installed executable.
		out, err := remote.Execute("sudo sh -ec 'test -f " + file.Path + " && test ! -L " + file.Path + " && sha256sum " + file.Path + "'")
		if err != nil {
			return fmt.Errorf("verifying installed %s: %w", file.Role, err)
		}
		sum, err := hashOutput(out)
		if err != nil {
			return err
		}
		if sum != file.SHA256 {
			return fmt.Errorf("installed %s differs from selected archive; postinst-modified executables are unsupported", file.Role)
		}
	}
	return nil
}
func packageInstallCommand(path string) string {
	return "sudo env DEBIAN_FRONTEND=noninteractive apt-get install --reinstall -y -o Dpkg::Options::=--force-confold " + path
}
