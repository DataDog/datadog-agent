// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localpackage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentbuild"
)

// This opt-in test runs real apt/dpkg only in a disposable, network-disabled
// container without host mounts. A separate dpkg database on the host would NOT
// contain maintainer-script effects. No real Agent or SSH/service manager is
// exercised: fixtures have harmless executable payloads and a systemctl stub.
func TestSameVersionPackageReplacementInContainer(t *testing.T) {
	image := os.Getenv("E2ECTL_PACKAGE_SMOKE_IMAGE")
	if image == "" {
		t.Skip("set E2ECTL_PACKAGE_SMOKE_IMAGE to a locally available Ubuntu image; use --test_strategy=standalone")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	name := "e2ectl-package-regression-" + hex.EncodeToString(nonce[:])
	docker := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "docker", args...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	if out, err := docker("run", "--detach", "--rm", "--pull=never", "--network=none", "--name", name, "--entrypoint", "sleep", image, "infinity"); err != nil {
		t.Fatalf("isolated container: %v: %s", err, out)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if out, err := exec.CommandContext(cleanup, "docker", "rm", "-f", name).CombinedOutput(); err != nil {
			t.Errorf("container cleanup: %v: %s", err, out)
		}
	}()
	execute := func(command string) (string, error) { return docker("exec", name, "bash", "-c", command) }
	must := func(command string) string {
		t.Helper()
		out, err := execute(command)
		if err != nil {
			t.Fatalf("container command %s: %v: %s", command, err, out)
		}
		return out
	}
	arch := must("dpkg --print-architecture")
	must("mkdir -p /fixture /run/systemd/system; printf '#!/bin/sh\nexec \"$@\"\n' > /usr/local/bin/sudo; chmod 0755 /usr/local/bin/sudo")
	must(`printf '#!/bin/sh\ncase "$1" in\nshow) printf "LoadState=loaded\\nUnitFileState=disabled\\n";;\ndaemon-reload) exit 0;;\n*) exit 9;;\nesac\n' > /usr/local/bin/systemctl; chmod 0755 /usr/local/bin/systemctl`)
	transport := Transport{Execute: execute, CopyFile: func(src, dst string) error {
		out, err := docker("cp", src, name+":"+dst)
		if err != nil {
			t.Log(out)
		}
		return err
	}}
	makePackage := func(variant string, modifyPostinst bool) agentbuild.Result {
		t.Helper()
		root := t.TempDir()
		write := func(path, content string, mode os.FileMode) {
			t.Helper()
			full := filepath.Join(root, path)
			if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(content), mode); err != nil {
				t.Fatal(err)
			}
		}
		write("DEBIAN/control", "Package: datadog-agent\nVersion: 7.83.0-1\nArchitecture: "+arch+"\nMaintainer: Fixture <fixture@example.com>\nDescription: isolated package regression fixture\n", 0644)
		for _, role := range []string{"agent", "trace-agent"} {
			path, err := agentbuild.PackageExecutablePath(role)
			if err != nil {
				t.Fatal(err)
			}
			write(strings.TrimPrefix(path, "/"), "#!/bin/sh\nprintf '"+variant+"-"+role+"\\n'\n", 0755)
		}
		postinst := "#!/bin/sh\nprintf '" + variant + "\\n' >> /tmp/package-fixture-installs\n"
		if modifyPostinst {
			postinst += "printf changed > /opt/datadog-agent/bin/agent/agent\n"
		}
		write("DEBIAN/postinst", postinst, 0755)
		if out, err := docker("cp", root, name+":/fixture/"+variant); err != nil {
			t.Fatalf("copy fixture: %v: %s", err, out)
		}
		// Bazel test temporary directories can inherit setgid from their parent.
		must("chmod g-s /fixture/" + variant + "/DEBIAN; chmod 0755 /fixture/" + variant + "/DEBIAN")
		must("dpkg-deb --build /fixture/" + variant + " /fixture/" + variant + ".deb")
		archive := filepath.Join(t.TempDir(), variant+".deb")
		if out, err := docker("cp", name+":/fixture/"+variant+".deb", archive); err != nil {
			t.Fatalf("retrieve fixture: %v: %s", err, out)
		}
		r, err := (agentbuild.Adapter{}).ExistingPackage(ctx, archive, "", agentbuild.Target{OS: "linux", Arch: arch})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	old := makePackage("old", false)
	fresh := makePackage("fresh", false)
	if old.Package.Version != fresh.Package.Version || old.Package.File.SHA256 == fresh.Package.File.SHA256 {
		t.Fatal("fixture must have identical version and different contents")
	}
	for _, r := range []agentbuild.Result{old, fresh} {
		release, verification, err := installFileVerified(transport, r)
		if err != nil {
			t.Fatal(err)
		}
		if len(verification.Executables) != 2 || verification.Scope != verificationScope {
			t.Fatal(verification)
		}
		if err = release(); err != nil {
			t.Fatal(err)
		}
	}
	if got := must("/opt/datadog-agent/bin/agent/agent"); got != "fresh-agent" {
		t.Fatalf("same-version replacement skipped: %s", got)
	}
	if got := must("/opt/datadog-agent/embedded/bin/trace-agent"); got != "fresh-trace-agent" {
		t.Fatal(got)
	}
	if got := must("cat /tmp/package-fixture-installs"); got != "old\nfresh" {
		t.Fatalf("maintainer script replacement evidence: %s", got)
	}
	// Ownership is real inode evidence: replacing one mask must not be undone.
	release, err := acquireServiceMasks(transport, "ownership-test")
	if err != nil {
		t.Fatal(err)
	}
	must("rm /run/systemd/system/datadog-agent.service; ln -s /dev/null /run/systemd/system/datadog-agent.service")
	if err = release(); err == nil {
		t.Fatal("foreign replacement mask was removed")
	}
	must("test -L /run/systemd/system/datadog-agent.service")
	must("rm /run/systemd/system/datadog-agent.service")
	changed := makePackage("postinst-changed", true)
	if release, proof, err := installFileVerified(transport, changed); err == nil || release != nil || proof != nil {
		t.Fatalf("postinst payload mismatch accepted: %v", err)
	}
	t.Log("same-version replacement, all-role hashes, package status, changed-mask preservation and postinst mismatch passed in isolated container")
}
