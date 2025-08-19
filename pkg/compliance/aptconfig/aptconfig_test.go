// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aptconfig

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAptConfigParser(t *testing.T) {
	f, err := os.Open("testdata/apt.conf")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	conf := parseAPTConfiguration(string(data))
	expected := map[string]interface{}{
		"APT::Move-Autobit-Sections": []string{
			"oldlibs",
			"contrib/oldlibs",
			"non-free/oldlibs",
			"restricted/oldlibs",
			"universe/oldlibs",
			"multiverse/oldlibs",
		},
		"APT::Never-MarkAuto-Sections": []string{
			"metapackages",
			"contrib/metapackages",
			"non-free/metapackages",
			"restricted/metapackages",
			"universe/metapackages",
			"multiverse/metapackages",
		},
		"APT::NeverAutoRemove": []string{
			"^firmware-linux.*",
			"^linux-firmware$",
			"^linux-image-[a-z0-9]*$",
			"^linux-image-[a-z0-9]*-[a-z0-9]*$",
		},
		"APT::Periodic::Enable":               "0",
		"APT::Periodic::Unattended-Upgrade":   "1",
		"APT::Periodic::Update-Package-Lists": "1",
		"APT::Update::Post-Invoke":            "rm -f /var/cache/apt/archives/*.deb /var/cache/apt/archives/partial/*.deb /var/cache/apt/*.bin || true",
		"APT::Update::Post-Invoke-Success":    "/usr/bin/test -e /usr/share/dbus-1/system-services/org.freedesktop.PackageKit.service \u0026\u0026 /usr/bin/test -S /var/run/dbus/system_bus_socket \u0026\u0026 /usr/bin/gdbus call --system --dest org.freedesktop.PackageKit --object-path /org/freedesktop/PackageKit --timeout 4 --method org.freedesktop.PackageKit.StateHasChanged cache-update \u003e /dev/null; /bin/echo \u003e /dev/null",
		"APT::VersionedKernelPackages": []string{
			"linux-.*",
			"kfreebsd-.*",
			"gnumach-.*",
			".*-modules",
			".*-kernel",
		},
		"Acquire::Changelogs::AlwaysOnline":         "true",
		"Acquire::CompressionTypes::Order::":        "gz",
		"Acquire::GzipIndexes":                      "true",
		"Acquire::Languages":                        "none",
		"Acquire::http::User-Agent-Non-Interactive": "true",
		"Apt::AutoRemove::SuggestsImportant":        "false",
		"DPkg::Post-Invoke": []string{
			"/usr/bin/test -e /usr/share/dbus-1/system-services/org.freedesktop.PackageKit.service \u0026\u0026 /usr/bin/test -S /var/run/dbus/system_bus_socket \u0026\u0026 /usr/bin/gdbus call --system --dest org.freedesktop.PackageKit --object-path /org/freedesktop/PackageKit --timeout 4 --method org.freedesktop.PackageKit.StateHasChanged cache-update \u003e /dev/null; /bin/echo \u003e /dev/null",
			"rm -f /var/cache/apt/archives/*.deb /var/cache/apt/archives/partial/*.deb /var/cache/apt/*.bin || true",
		},
		"DPkg::Pre-Install-Pkgs":  "/usr/sbin/dpkg-preconfigure --apt || true",
		"Dir::Cache::pkgcache":    "",
		"Dir::Cache::srcpkgcache": "",
		"Unattended-Upgrade::Allowed-Origins": []string{
			"${distro_id}:${distro_codename}",
			"${distro_id}:${distro_codename}-security",
			"${distro_id}ESMApps:${distro_codename}-apps-security",
			"${distro_id}ESM:${distro_codename}-infra-security",
		},
		"Unattended-Upgrade::DevRelease": "auto",
	}
	assert.Equal(t, expected, conf)
}

func TestSystemdConfigParser(t *testing.T) {
	f, err := os.Open("testdata/apt-daily.timer")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	conf := parseSystemdConf(string(data))
	expected := map[string]string{
		"Install/WantedBy":         "timers.target",
		"Unit/Description":         "Message of the Day",
		"Timer/OnCalendar":         "00,12:00:00",
		"Timer/RandomizedDelaySec": "12h",
		"Timer/Persistent":         "true",
		"Timer/OnStartupSec":       "1min",
	}
	assert.Equal(t, expected, conf)

}
