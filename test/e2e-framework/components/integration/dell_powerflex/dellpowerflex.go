// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dellpowerflex provisions the host-side virtualization stack used by
// the Dell PowerFlex all-in-one lab: a bare-metal RHEL9 host (m5.metal) that
// runs nested libvirt guests (the PFMP appliance plus MDM/SDS VMs) on its EBS
// root, with the Datadog Agent + dell_powerflex check running on the host.
//
// This component is responsible only for the deterministic, idempotent host
// preparation that can run unattended at create time:
//   - install qemu-kvm / libvirt / virt-install / libguestfs-tools (dnf)
//   - enable + start libvirtd and verify hardware KVM (/dev/kvm)
//   - define an isolated libvirt NAT network with fixed DHCP reservations so
//     the host Agent can reach PFMP at a stable URL (10.55.0.20:443)
//
// Everything beyond this point (pulling the PFMP OVA + el9 RPMs from S3,
// OVA->qcow2 conversion, the PFMP first-boot wizard, building the nested
// MDM/SDS scli cluster, registering the system into PFMP, and snapshotting a
// golden AMI) is DEFERRED to live exploration. It depends on live-only unknowns
// (appliance console mode, install_PFMP.sh prompts, single-node PFMP support)
// and must not be implemented blind. The deferred steps are captured as TODO
// stubs here and as an operator-driven bootstrap.sh in this directory; the live
// runbook is .agint/labs/dell-powerflex/research/metal-nested.md.
package dellpowerflex

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// PFMPReservedIP is the libvirt-NAT DHCP reservation for the PFMP Gateway.
	// The host Agent's dell_powerflex check targets https://PFMPReservedIP:443.
	PFMPReservedIP = "10.55.0.20"

	// natNetworkName is the libvirt network defined for the nested guests.
	natNetworkName = "pflex"

	// natNetworkXML defines an isolated NAT network on 10.55.0.0/24 with a fixed
	// DHCP reservation for PFMP. MDM/SDS guests get addresses from the same
	// range. MAC 52:54:00:55:00:20 is paired with PFMPReservedIP so the
	// appliance always lands on the address the Agent expects.
	natNetworkXML = `<network>
  <name>` + natNetworkName + `</name>
  <forward mode='nat'/>
  <bridge name='virbr-pflex' stp='on' delay='0'/>
  <ip address='10.55.0.1' netmask='255.255.255.0'>
    <dhcp>
      <range start='10.55.0.100' end='10.55.0.200'/>
      <host mac='52:54:00:55:00:20' name='pfmp' ip='` + PFMPReservedIP + `'/>
    </dhcp>
  </ip>
</network>`
)

// InstallVirtStack prepares the bare-metal host so it can run nested libvirt
// guests. It returns the last command in the ordered chain so callers can make
// the Agent (and any later bootstrap) depend on host readiness.
//
// All commands are idempotent so repeated `pulumi up` runs (and golden-AMI
// relaunches) converge rather than fail.
func InstallVirtStack(e config.Env, host *remoteComp.Host, opts ...pulumi.ResourceOption) (command.Command, error) {
	runner := host.OS.Runner()
	namer := e.CommonNamer().WithPrefix("pflex")

	// 1. Install the virtualization toolchain. dnf is idempotent.
	installPkgs, err := runner.Command(
		namer.ResourceName("install-virt-stack"),
		&command.Args{
			Sudo: true,
			Create: pulumi.String(`bash <<'EOF'
set -euxo pipefail
dnf install -y qemu-kvm libvirt virt-install libguestfs-tools
EOF`),
		},
		opts...,
	)
	if err != nil {
		return nil, err
	}

	// 2. Enable + start libvirtd and verify hardware KVM is present. On a true
	//    bare-metal host /dev/kvm exists and vmx/svm appears in cpuinfo; fail
	//    loudly if not, since nested guests would otherwise silently fall back
	//    to emulation (unusably slow for PFMP).
	enableLibvirt, err := runner.Command(
		namer.ResourceName("enable-libvirtd"),
		&command.Args{
			Sudo: true,
			Create: pulumi.String(`bash <<'EOF'
set -euxo pipefail
systemctl enable --now libvirtd
test -e /dev/kvm
grep -Eq '(vmx|svm)' /proc/cpuinfo
virsh net-list --all
EOF`),
		},
		utils.MergeOptions(opts, utils.PulumiDependsOn(installPkgs))...,
	)
	if err != nil {
		return nil, err
	}

	// 3. Define + autostart the NAT network with the fixed PFMP reservation.
	//    Idempotent: redefine if it already exists, then (re)start it.
	defineNet, err := runner.Command(
		namer.ResourceName("define-nat-network"),
		&command.Args{
			Sudo: true,
			Create: pulumi.Sprintf(`bash <<'EOF'
set -euxo pipefail
cat > /tmp/%s.xml <<'XML'
%s
XML
if virsh net-info %s >/dev/null 2>&1; then
  virsh net-destroy %s || true
  virsh net-undefine %s || true
fi
virsh net-define /tmp/%s.xml
virsh net-autostart %s
virsh net-start %s
virsh net-dumpxml %s
EOF`,
				natNetworkName, natNetworkXML,
				natNetworkName, natNetworkName, natNetworkName,
				natNetworkName, natNetworkName, natNetworkName, natNetworkName),
			Delete: pulumi.Sprintf(`bash <<'EOF'
set -uxo pipefail
virsh net-destroy %s || true
virsh net-undefine %s || true
EOF`, natNetworkName, natNetworkName),
		},
		utils.MergeOptions(opts, utils.PulumiDependsOn(enableLibvirt))...,
	)
	if err != nil {
		return nil, err
	}

	// 4. DEFERRED bootstrap (live exploration only). Stage the operator-driven
	//    bootstrap.sh onto the host so an engineer can drive the PFMP first-boot
	//    wizard + nested cluster build interactively over `virsh console`.
	//    Staging is safe and idempotent; the script itself is NOT executed here.
	//
	//    TODO(live-exploration): once the live unknowns are resolved, the steps
	//    in bootstrap.sh become candidates for ordered runner.Command stages,
	//    each depending on the previous, e.g.:
	//      - stageOVA:        curl <presigned S3> -> /var/lib/libvirt/images/pfmp.ova
	//      - convertOVA:      tar x + qemu-img convert vmdk -> qcow2 (EBS root)
	//      - definePFMP:      virsh define pfmp.xml (NIC mac 52:54:00:55:00:20)
	//      - firstBootWizard: drive install_PFMP.sh via console/cloud-init
	//      - buildCluster:    install el9 RPMs on MDM/SDS, scli --create_mdm_cluster ...
	//      - registerSystem:  import PowerFlex 4.x into PFMP Gateway
	//      - goldenAMI:       quiesce qcow2 + aws ec2 create-image
	//    Do not implement these until the runbook unknowns are confirmed live.
	stageBootstrap, err := stageBootstrapScript(e, host, utils.MergeOptions(opts, utils.PulumiDependsOn(defineNet))...)
	if err != nil {
		return nil, err
	}

	return stageBootstrap, nil
}
