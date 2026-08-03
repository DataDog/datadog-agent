// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import "slices"

// family groups distros that share a package manager and service names, so a probe
// builder can resolve the right rule and remediation for each.
type family int

const (
	rhel family = iota
	debian
)

// probeCheck exercises one OpenSCAP OVAL probe by driving a rule from a failing to a
// passing state. build resolves the concrete rule and remediation for a distro, since
// package managers and service names differ across families.
type probeCheck struct {
	name     string // subtest label and probe identifier
	hostOnly bool   // skip in the containerized agent (the probe needs live host state)
	build    func(d distro) probe
}

// probeChecks is the single source of truth for the per-probe tests: each entry is
// written once and run on every distro and environment where it applies (probesFor).
// The names double as the OpenSCAP probe coverage: file, textfilecontent54 (sshd,
// accounts, kernel_module), rpminfo/dpkginfo (package), sysctl, systemdunitproperty
// (service) and partition.
var probeChecks = []probeCheck{
	{name: "file", build: fileCheck},
	{name: "sshd", build: sshdCheck},
	{name: "package", build: packageCheck},
	{name: "accounts", build: accountsCheck},
	{name: "sysctl", hostOnly: true, build: sysctlCheck},
	{name: "service", hostOnly: true, build: serviceCheck},
	{name: "partition", hostOnly: true, build: partitionCheck},
	{name: "kernel_module", hostOnly: true, build: kernelModuleCheck},
}

// probesFor returns the probes to exercise on d in the given environment. A nil
// onlyProbes runs every applicable check; a non-nil one restricts to those names, for
// distros whose content evaluates only a subset (see distroAmazonLinux2023).
func probesFor(d distro, onHost bool) []probe {
	var ps []probe
	for _, c := range probeChecks {
		if c.hostOnly && !onHost {
			continue
		}
		if d.onlyProbes != nil && !slices.Contains(d.onlyProbes, c.name) {
			continue
		}
		ps = append(ps, c.build(d))
	}
	return ps
}

// fileCheck exercises the file probe via the permissions of /etc/cron.daily.
func fileCheck(distro) probe {
	return probe{"file", rulePrefix + "file_permissions_cron_daily",
		"sudo chmod o+r /etc/cron.daily", "sudo chmod og-rwx /etc/cron.daily"}
}

// sshdCheck exercises the textfilecontent54 probe via sshd_config content.
func sshdCheck(distro) probe {
	return probe{"sshd", rulePrefix + "sshd_disable_root_login",
		`sudo sed -i -E 's/^#?(PermitRootLogin).*/\1 yes/' /etc/ssh/sshd_config`,
		`sudo sed -i -E 's/^#?(PermitRootLogin).*/\1 no/' /etc/ssh/sshd_config`}
}

// packageCheck exercises the rpminfo/dpkginfo probe via the telnet package.
func packageCheck(d distro) probe {
	rule := rulePrefix + "package_telnet_removed"
	if d.family == debian {
		return probe{"package", rule,
			"sudo apt-get update -qq && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y telnet",
			"sudo DEBIAN_FRONTEND=noninteractive apt-get remove -y telnet"}
	}
	return probe{"package", rule, "sudo dnf install -y telnet", "sudo dnf remove -y telnet"}
}

// accountsCheck exercises the textfilecontent54 probe via the login.defs password-warn
// default. It edits only the default, so existing accounts (and the SSH user) are untouched.
func accountsCheck(distro) probe {
	const f = "/etc/login.defs"
	return probe{"accounts", rulePrefix + "accounts_password_warn_age_login_defs",
		"sudo sed -i -E 's/^#?PASS_WARN_AGE.*/PASS_WARN_AGE 0/' " + f,
		"sudo sed -i -E 's/^#?PASS_WARN_AGE.*/PASS_WARN_AGE 7/' " + f}
}

// sysctlCheck exercises the sysctl probe. cis-rhel8 and cis-rhel10 evaluate
// kernel.dmesg_restrict, the other benchmarks net.ipv4.ip_forward (note the opposite
// compliant values). Host-only: a Docker host forces ip_forward=1, which the
// containerized variant could never satisfy.
func sysctlCheck(d distro) probe {
	if d.name == "rhel8" || d.name == "rhel10" {
		const f = "/etc/sysctl.d/90-dmesg.conf"
		return probe{"sysctl", rulePrefix + "sysctl_kernel_dmesg_restrict",
			"sudo sysctl -w kernel.dmesg_restrict=0 && sudo rm -f " + f,
			"sudo sysctl -w kernel.dmesg_restrict=1 && echo 'kernel.dmesg_restrict = 1' | sudo tee " + f}
	}
	const f = "/etc/sysctl.d/60-netipv4_sysctl.conf"
	return probe{"sysctl", rulePrefix + "sysctl_net_ipv4_ip_forward",
		"sudo sysctl -w net.ipv4.ip_forward=1 && sudo rm -f " + f,
		"sudo sysctl -w net.ipv4.ip_forward=0 && echo 'net.ipv4.ip_forward = 0' | sudo tee " + f}
}

// serviceCheck exercises the systemdunitproperty probe via the cron service. Host-only:
// a containerized agent cannot read the host's live systemd unit state.
func serviceCheck(d distro) probe {
	svc, rule := "crond", rulePrefix+"service_crond_enabled"
	if d.family == debian {
		svc, rule = "cron", rulePrefix+"service_cron_enabled"
	}
	return probe{"service", rule,
		"sudo systemctl disable --now " + svc, "sudo systemctl enable --now " + svc}
}

// partitionCheck exercises the partition probe via a separate /tmp. Host-only: it reads
// the live mount table. The fix mounts a tmpfs /tmp, as the benchmark expects.
func partitionCheck(distro) probe {
	return probe{"partition", rulePrefix + "partition_for_tmp",
		"sudo umount /tmp 2>/dev/null || true",
		"echo 'tmpfs /tmp tmpfs defaults,nosuid,nodev,noexec 0 0' | sudo tee -a /etc/fstab && sudo mount /tmp"}
}

// kernelModuleCheck exercises the textfilecontent54 probe via the cramfs modprobe
// blacklist. Host-only: the rule also depends on the module not being loaded.
func kernelModuleCheck(distro) probe {
	const f = "/etc/modprobe.d/cramfs.conf"
	return probe{"kernel_module", rulePrefix + "kernel_module_cramfs_disabled",
		"sudo rm -f " + f + " && sudo modprobe cramfs 2>/dev/null || true",
		"sudo modprobe -r cramfs 2>/dev/null; printf 'install cramfs /bin/false\\nblacklist cramfs\\n' | sudo tee " + f}
}
