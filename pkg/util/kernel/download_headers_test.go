// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package kernel

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/nikos/apt"
	"github.com/DataDog/nikos/types"
)

const reposDir = "/tmp/apt-repos-%s"
const reposSource = "%s/sources.list"
const reposSourceDir = "/tmp/apt-repos-%s/sources.list.d"
const headerDownloadDir = "%s/headers"

var _ types.Logger = customLogger{}

var ubuntuRelease map[string]string = map[string]string{
	"ID":                 "ubuntu",
	"ID_LIKE":            "debian",
	"PRETTY_NAME":        "Ubuntu 20.04.3 LTS",
	"VERSION_ID":         "20.04",
	"HOME_URL":           "https://www.ubuntu.com/",
	"SUPPORT_URL":        "https://help.ubuntu.com/",
	"BUG_REPORT_URL":     "https://bugs.launchpad.net/ubuntu/",
	"PRIVACY_POLICY_URL": "https://www.ubuntu.com/legal/terms-and-policies/privacy-policy",
	"VERSION_CODENAME":   "focal",
	"UBUNTU_CODENAME":    "focal",
}

var debianRelease map[string]string = map[string]string{
	"PRETTY_NAME":      "Debian GNU/Linux 11 (bullseye)",
	"NAME":             "Debian GNU/Linux",
	"VERSION_ID":       "11",
	"VERSION":          "11 (bullseye)",
	"VERSION_CODENAME": "bullseye",
	"ID":               "debian",
	"HOME_URL":         "https://www.debian.org/",
	"SUPPORT_URL":      "https://www.debian.org/support",
	"BUG_REPORT_URL":   "https://bugs.debian.org/",
}

var ubuntuRepos = []string{
	"deb http://gb.ports.ubuntu.com/ubuntu-ports/ focal main restricted",
	"deb http://gb.ports.ubuntu.com/ubuntu-ports/ focal-updates main restricted",
	"deb http://gb.ports.ubuntu.com/ubuntu-ports/ focal universe",
	"deb http://gb.ports.ubuntu.com/ubuntu-ports/ focal-updates universe",
	"deb http://gb.ports.ubuntu.com/ubuntu-ports/ focal multiverse",
	"deb http://gb.ports.ubuntu.com/ubuntu-ports/ focal-updates multiverse",
	"deb http://gb.ports.ubuntu.com/ubuntu-ports/ focal-backports main restricted universe multiverse",
	"deb http://gb.ports.ubuntu.com/ubuntu-ports/ focal-security main restricted",
	"deb http://gb.ports.ubuntu.com/ubuntu-ports/ focal-security universe",
	"deb http://gb.ports.ubuntu.com/ubuntu-ports/ focal-security multiverse",
}

var debianRepos = []string{
	"deb http://http.us.debian.org/debian bullseye main",
	"deb-src http://http.us.debian.org/debian bullseye main",
	"deb http://security.debian.org/debian-security bullseye-security main",
	"deb-src http://security.debian.org/debian-security bullseye-security main",
	"deb http://http.us.debian.org/debian bullseye-updates main",
	"deb-src http://http.us.debian.org/debian bullseye-updates main",
}

var targetUbuntu types.Target = types.Target{
	Distro: types.Distro{
		"ubuntu",
		"bullseye/sid",
		"debian",
	},
	OSRelease: ubuntuRelease,
	Uname: types.Utsname{
		"5.4.0-92-generic",
		"aarch64",
	},
}

var targetDebian types.Target = types.Target{
	Distro: types.Distro{
		"debian",
		"11.2",
		"debian",
	},
	OSRelease: debianRelease,
	Uname: types.Utsname{
		"5.10.0-10-arm64",
		"aarch64",
	},
}

type TargetSetup struct {
	target types.Target
	repos  []string
}

var targets map[string]TargetSetup = map[string]TargetSetup{
	"ubuntu": TargetSetup{
		targetUbuntu,
		ubuntuRepos,
	},
	"debian": TargetSetup{
		targetDebian,
		debianRepos,
	},
}

func genRepoSuffix(target types.Target) string {
	return target.Distro.Display + "-" + target.Uname.Kernel + "-" + target.Uname.Machine
}

func mkTargetDirName(target types.Target) string {
	return fmt.Sprintf(reposDir, genRepoSuffix(target))
}

func setup(target types.Target, repos []string, dname string) error {
	// Make directory where apt config is placed
	if err := os.MkdirAll(dname, 0744); err != nil {
		return fmt.Errorf("failed to create dir %s: %w", dname, err)
	}

	// Make source-list.d
	sources := fmt.Sprintf(reposSourceDir, genRepoSuffix(target))
	if err := os.MkdirAll(sources, 0744); err != nil {
		return fmt.Errorf("failed to create dir %s: %w", sources, err)
	}

	// add repo source list
	fname := fmt.Sprintf(reposSource, dname)
	reposF, err := os.Create(fname)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", reposSource, err)
	}

	defer reposF.Close()

	for _, repo := range repos {
		_, err := reposF.WriteString(repo + "\n")
		if err != nil {
			return fmt.Errorf("failed to write repos to file %s: %w", fname, err)
		}
	}

	return nil
}

func getBackend(target *types.Target, reposDir string) (backend types.Backend, err error) {
	logger := customLogger{}
	switch strings.ToLower(target.Distro.Display) {
	case "debian", "ubuntu":
		backend, err = apt.NewBackend(target, reposDir, logger)
	default:
		err = fmt.Errorf("unsupported distribution '%s'", target.Distro.Display)
	}

	return
}

func benchmarkHeaderDownload(ts TargetSetup, b *testing.B) {
	dname := mkTargetDirName(ts.target)
	kname := fmt.Sprintf(headerDownloadDir, dname)

	if err := setup(ts.target, ts.repos, dname); err != nil {
		b.Errorf("Failed to setup target: %s\n", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// create output directory for kernel headers
		if err := os.MkdirAll(kname, 0744); err != nil {
			b.Errorf("failed to create directory %s: %w", kname, err)
		}

		backend, err := getBackend(&ts.target, dname)
		if err != nil {
			b.Errorf("Failed to create backend: %s\n", err)
		}

		if err := backend.GetKernelHeaders(fmt.Sprintf(headerDownloadDir, dname)); err != nil {
			b.Errorf("Failed to download kernel headers: %s\n", err)
		}
		backend.Close()

		// remove kernel headers directory
		if err := os.RemoveAll(kname); err != nil {
			b.Errorf("failed to remove directory %s: %w", kname, err)
		}
	}

	// Remove tmp repos directory
	if err := os.RemoveAll(dname); err != nil {
		b.Errorf("failed to remove directory: %s\n", err)
	}
}

func BenchmarkHeaderDownloadUbuntu(b *testing.B) {
	b.ReportAllocs()
	benchmarkHeaderDownload(targets["ubuntu"], b)
}

func BenchmarkHeaderDownloadDebian(b *testing.B) {
	b.ReportAllocs()
	benchmarkHeaderDownload(targets["debian"], b)
}
