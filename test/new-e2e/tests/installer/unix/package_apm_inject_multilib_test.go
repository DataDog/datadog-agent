// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/base64"
	"strings"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/stretchr/testify/require"
)

const (
	releasedMultilibInjectorTag     = "0.71.0-1"
	releasedMultilibInjectorVersion = "0.71.0"
	multilibExecutableSource        = "#include <stdio.h>\nint main(void) { puts(\"ok\"); return 0; }\n"
)

// multilibLauncherPaths mirrors apminject.multilibLauncherPaths: every layout a
// 0.71+ apm-inject OCI must ship before the installer is allowed to write the
// literal "$LIB" entry to /etc/ld.so.preload. Kept in the test as a literal
// (rather than imported) so a change to the production list has to be made
// deliberately in both places.
var multilibLauncherPaths = []string{
	"launcher.preload.i386.so",
	"lib/launcher.preload.so",
	"lib/i386-linux-gnu/launcher.preload.so",
	"lib/x86_64-linux-gnu/launcher.preload.so",
	"lib32/launcher.preload.so",
	"lib64/launcher.preload.so",
}

// packageApmInjectMultilibSuite covers the amd64 multilib launcher layout on one
// host per glibc $LIB convention. It is a suite of its own rather than a test in
// packageApmInjectSuite because the RHEL family is excluded from that suite's
// matrix — most of it needs Docker, which RHEL 9 does not package.
type packageApmInjectMultilibSuite struct {
	packageBaseSuite
}

func testApmInjectMultilib(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &packageApmInjectMultilibSuite{
		packageBaseSuite: newPackageSuite("apm-inject-multilib", os, arch, method),
	}
}

func (s *packageApmInjectMultilibSuite) SetupTest() {
	// Purge() uses Execute (not MustExecute), so failures are silent.
	// A stale packages.db entry causes Install() to skip PostInstall hooks
	// (which create /etc/ld.so.preload).
	s.Env().RemoteHost.Execute("sudo rm -f /opt/datadog-packages/packages.db")
	s.Env().RemoteHost.Execute("sudo rm -f /etc/ld.so.preload")
}

// libExpansion returns the directories this host's dynamic linker substitutes
// for the literal "$LIB" token, for 64-bit and 32-bit processes respectively.
//
// $LIB is baked into ld.so at glibc build time and the two families disagree,
// which is the entire reason this suite runs on more than one distro:
//   - Debian/Ubuntu use multiarch: lib/x86_64-linux-gnu and lib/i386-linux-gnu.
//   - RHEL, CentOS, Fedora, Amazon Linux and SUSE use the classic split:
//     lib64 and lib.
//
// A package that only ships the multiarch pair leaves every process on a RHEL
// host with a preload entry that resolves to nothing.
func (s *packageApmInjectMultilibSuite) libExpansion() (lib64, lib32 string) {
	switch s.os.Flavor {
	case e2eos.Ubuntu, e2eos.Debian:
		return "lib/x86_64-linux-gnu", "lib/i386-linux-gnu"
	default:
		return "lib64", "lib"
	}
}

// installCompiler installs a C compiler able to emit dynamically-linked ELF64
// executables.
func (s *packageApmInjectMultilibSuite) installCompiler() {
	s.T().Helper()
	switch s.os.Flavor {
	case e2eos.Ubuntu, e2eos.Debian:
		s.Env().RemoteHost.MustExecute("sudo apt-get update -qq && sudo apt-get install -y gcc libc6-dev")
	case e2eos.RedHat, e2eos.CentOS, e2eos.Fedora, e2eos.AmazonLinux:
		s.Env().RemoteHost.MustExecute("sudo yum install -y gcc glibc-devel")
	default:
		s.T().Skipf("test does not know how to install gcc on %s", s.os.Flavor)
	}
}

// install32BitSupport adds what `gcc -m32` needs on top of installCompiler.
// None of the amd64 images in the matrix ship a 32-bit libc by default, and the
// package that provides one differs: Debian bundles headers and runtime in
// gcc-multilib, while on RPM distros the stock gcc is already multilib-capable
// and only the i686 glibc bits are missing.
func (s *packageApmInjectMultilibSuite) install32BitSupport() {
	s.T().Helper()
	var cmd string
	switch s.os.Flavor {
	case e2eos.Ubuntu, e2eos.Debian:
		cmd = "sudo apt-get install -y gcc-multilib"
	case e2eos.RedHat, e2eos.CentOS, e2eos.Fedora, e2eos.AmazonLinux:
		cmd = "sudo yum install -y glibc-devel.i686 libgcc.i686"
	default:
		s.T().Skipf("test does not know how to install 32-bit libc on %s", s.os.Flavor)
	}
	out, err := s.Env().RemoteHost.Execute(cmd)
	require.NoErrorf(s.T(), err,
		"could not install a 32-bit libc on %s. Everything above this point passed, so the injector's 64-bit $LIB path is fine and this is a test-host packaging problem.\n%s",
		s.os, out)
}

// elfClass returns "32" or "64" for the ELF at path, read straight from the
// EI_CLASS byte of the header. Reading the byte avoids depending on file(1) or
// readelf(1), which are not installed on every image in the matrix.
func (s *packageApmInjectMultilibSuite) elfClass(path string) string {
	s.T().Helper()
	switch out := strings.TrimSpace(s.Env().RemoteHost.MustExecute("sudo od -An -t u1 -j 4 -N 1 " + path)); out {
	case "1":
		return "32"
	case "2":
		return "64"
	default:
		return "unknown(" + out + ")"
	}
}

// TestMultilibLauncher verifies the Agent integration with the amd64 multilib
// layout released in apm-inject 0.71: /etc/ld.so.preload carries a literal $LIB
// entry, the OCI ships a launcher at every layout $LIB can expand to, the pair
// this host's ld.so actually resolves has the matching ELF class, a 64-bit
// process is injected, and a 32-bit process starts cleanly.
func (s *packageApmInjectMultilibSuite) TestMultilibLauncher() {
	if s.installMethod != InstallMethodInstallScript || s.arch != e2eos.AMD64Arch {
		s.T().Skip("the multilib launcher layout only applies to amd64 hosts installed with the install script")
	}

	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		envForceVersion("datadog-apm-inject", releasedMultilibInjectorTag),
	)
	defer s.Purge()

	host := s.Env().RemoteHost
	content, err := s.host.ReadFile("/etc/ld.so.preload")
	require.NoError(s.T(), err)
	require.Contains(s.T(), string(content), injectTmpfsDir+"/$LIB/launcher.preload.so")
	require.NotContains(s.T(), string(content), injectTmpfsDir+"/launcher.preload.so\n")
	require.Equal(s.T(), releasedMultilibInjectorVersion,
		strings.TrimSpace(host.MustExecute("cat /opt/datadog-packages/datadog-apm-inject/stable/version")))

	for _, launcher := range multilibLauncherPaths {
		host.MustExecute("test -e /opt/datadog-packages/datadog-apm-inject/stable/inject/" + launcher)
	}

	// The two paths ld.so will actually build from the $LIB entry on this
	// distro. Resolving them through the tmpfs symlink (rather than the
	// persistent OCI directory) checks exactly what a process sees, and the ELF
	// class check catches an OCI that ships the right paths with the wrong
	// bitness — which would break every exec on the host, not just injection.
	lib64Dir, lib32Dir := s.libExpansion()
	launcher64 := "/run/datadog-apm-inject/" + lib64Dir + "/launcher.preload.so"
	launcher32 := "/run/datadog-apm-inject/" + lib32Dir + "/launcher.preload.so"
	_, err = host.Execute("test -e " + launcher64)
	require.NoErrorf(s.T(), err, "%s expands $LIB to %s for 64-bit processes, but %s does not exist", s.os.Flavor, lib64Dir, launcher64)
	_, err = host.Execute("test -e " + launcher32)
	require.NoErrorf(s.T(), err, "%s expands $LIB to %s for 32-bit processes, but %s does not exist", s.os.Flavor, lib32Dir, launcher32)
	require.Equal(s.T(), "64", s.elfClass(launcher64), "launcher reached through $LIB=%s must be ELF64", lib64Dir)
	require.Equal(s.T(), "32", s.elfClass(launcher32), "launcher reached through $LIB=%s must be ELF32", lib32Dir)

	// Build real dynamic ELF32 and ELF64 executables from the same source.
	// Installing the toolchain now, with host injection already active, makes the
	// package manager's own subprocesses exercise the new preload path too.
	defer host.Execute("sudo rm -f /tmp/multilib_executable.c /tmp/multilib-executable32 /tmp/multilib-executable64") //nolint:errcheck
	s.installCompiler()
	encodedSource := base64.StdEncoding.EncodeToString([]byte(multilibExecutableSource))
	host.MustExecute("echo " + encodedSource + " | base64 -d | sudo tee /tmp/multilib_executable.c >/dev/null")
	host.MustExecute("gcc -m64 -Wall -Wextra -Werror /tmp/multilib_executable.c -o /tmp/multilib-executable64")
	require.Equal(s.T(), "64", s.elfClass("/tmp/multilib-executable64"))

	// 64-bit first: this is what every process on the host goes through, so a
	// distro whose 32-bit libc cannot be installed still yields a verdict on the
	// $LIB expansion that matters most.
	output64, err := host.Execute("DD_APM_INSTRUMENTATION_DEBUG=true /tmp/multilib-executable64 2>&1")
	require.NoError(s.T(), err)
	require.Contains(s.T(), output64, "ok", "64-bit executable did not run successfully")
	require.Contains(s.T(), output64, "debug flag set, running injection",
		"64-bit executable did not load the APM injector")

	s.install32BitSupport()
	host.MustExecute("gcc -m32 -Wall -Wextra -Werror /tmp/multilib_executable.c -o /tmp/multilib-executable32")
	require.Equal(s.T(), "32", s.elfClass("/tmp/multilib-executable32"))

	// Exact equality, not Contains: anything ld.so prints about a preload entry
	// it could not load (the failure mode when $LIB resolves to a layout the
	// package does not ship) lands on stderr and shows up here.
	output32, err := host.Execute("/tmp/multilib-executable32 2>&1")
	require.NoError(s.T(), err)
	require.Equal(s.T(), "ok", strings.TrimSpace(output32), "32-bit executable emitted a loader error")
}
