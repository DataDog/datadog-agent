// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dellpowerflex is the all-in-one Dell PowerFlex lab scenario.
//
// Architecture (single framework-provisioned host; everything else is nested
// libvirt and deferred to live exploration):
//
//	ONE m5.metal RHEL9 host, 700 GiB gp3 root, provisioned via the framework
//	ec2.NewVM (so it is reachable through appgate). The host runs:
//	  - the libvirt virtualization stack (qemu-kvm/libvirt/...) + a NAT network
//	    with PFMP reserved at 10.55.0.20 (see components/integration/dell-powerflex)
//	  - the released Datadog Agent + the dell_powerflex check, pointed at the
//	    PFMP Gateway on https://10.55.0.20:443
//
// The nested PFMP appliance + MDM/SDS cluster are brought up during live
// exploration via the staged bootstrap.sh; until then the check reports
// dell_powerflex.api.can_connect=0, which is expected.
//
// Registered in the scenario registry as "aws/dell-powerflex".
package dellpowerflex

import (
	"os"
	"strings"

	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	dellpowerflexcomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/integration/dell-powerflex"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// hostName is the VM name; exported as dd-Host-<hostName>. The aws namer
	// prefixes "aws-", so exec/ssh target role "aws-powerflex".
	hostName = "powerflex"

	// instanceType is the bare-metal host required for hardware-accelerated
	// nested virtualization (i3.metal not offered in us-east-1; i4i.metal local
	// NVMe is not captured by AMI snapshots).
	instanceType = "m5.metal"

	// rootVolumeSizeGiB holds all nested-VM qcow2 state on the EBS root so a
	// golden AMI captures the whole cluster. The full topology (3-node PFMP MVM
	// at ~300 GiB/node + MDM + 3 SDS with data disks + the PFMP2 OVA/qcow2)
	// needs ~1.5 TiB; a smaller root fills mid-install and wedges rke2/etcd.
	rootVolumeSizeGiB = 1500

	// defaultGoldenAMI is the turnkey golden image (us-east-1) capturing the full
	// working lab: 3-node PFMP 4.6.2.1 MVM + a real PowerFlex block cluster
	// (MDM + 3 SDS + protection domain + storage pool + volume), the Datadog
	// Agent with the dell_powerflex check, and boot-persistence (all nested VMs
	// autostart; scini/fio/PowerFlex node services enabled). Launched on an
	// m5.metal it boots straight to a healthy cluster under load emitting the
	// full dell_powerflex metric set (~556 metrics / 80 families) — no bootstrap.
	// Override with DELL_POWERFLEX_GOLDEN_AMI; set to "" + the env unset to take
	// the from-scratch build path instead. See README.md + .agint runbook.
	defaultGoldenAMI = "ami-06fd51beaaa00b88d"
)

// checkConfig is the dell_powerflex check configuration template, edited by the
// lab maintainer at config/dell_powerflex.yaml and embedded here.
//
//go:embed config/dell_powerflex.yaml
var checkConfig string

// Run provisions the single m5.metal host, prepares the host virtualization
// stack, and installs the released Agent + dell_powerflex check.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewHost()

	// Default path: boot the turnkey golden AMI (defaultGoldenAMI) on m5.metal.
	// The image already carries KVM/libvirt, the autostart nested VMs (PFMP MVM +
	// the PowerFlex block cluster), boot-persistent scini/fio/node services, and
	// the Agent + dell_powerflex check — so we SKIP InstallVirtStack and agent
	// re-provisioning (re-running them would clash with the libvirt net / wipe the
	// installed integration). We just provision + export the host; the baked
	// cluster auto-recovers and the Agent emits the full metric set.
	//
	// DELL_POWERFLEX_GOLDEN_AMI overrides the image. Set DELL_POWERFLEX_FROM_SCRATCH=1
	// (with no golden AMI) to take the from-scratch build path instead (vanilla
	// RHEL9 + InstallVirtStack + deferred PFMP bootstrap — see the .agint runbook).
	goldenAMI := strings.TrimSpace(os.Getenv("DELL_POWERFLEX_GOLDEN_AMI"))
	if goldenAMI == "" && os.Getenv("DELL_POWERFLEX_FROM_SCRATCH") == "" {
		goldenAMI = defaultGoldenAMI
	}

	vmOpts := []ec2.VMOption{
		ec2.WithInstanceType(instanceType),
		ec2.WithStorageSize(rootVolumeSizeGiB),
	}
	if goldenAMI != "" {
		vmOpts = append(vmOpts, ec2.WithAMI(goldenAMI, compos.RedHat9, compos.AMD64Arch))
	} else {
		vmOpts = append(vmOpts, ec2.WithOS(compos.RedHat9))
	}

	host, err := ec2.NewVM(awsEnv, hostName, vmOpts...)
	if err != nil {
		return err
	}
	// Export as dd-Host-<hostName> so exec/ssh tasks can reach it.
	if err := host.Export(ctx, env.RemoteHostOutput()); err != nil {
		return err
	}

	// This lab is a real E2E lab: no fakeintake, no updater.
	env.DisableFakeIntake()
	env.DisableUpdater()

	if goldenAMI != "" {
		// Golden image already carries the full stack; nothing else to provision.
		env.DisableAgent()
		return nil
	}

	// Fresh-build path: prepare the host virtualization stack (qemu-kvm/libvirt +
	// NAT network + staged deferred bootstrap). Returns the last command so the
	// Agent waits for host readiness before configuring the check.
	virtReady, err := dellpowerflexcomp.InstallVirtStack(&awsEnv, host)
	if err != nil {
		return err
	}

	// Released Agent on the host with the dell_powerflex check. Check endpoint
	// and credentials are env-overridable at deploy time.
	agentConfig := renderCheckConfig()
	hostAgent, err := agent.NewHostAgent(&awsEnv, host,
		agentparams.WithIntegration("dell_powerflex.d", agentConfig),
		agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(virtReady)),
	)
	if err != nil {
		return err
	}
	if err := hostAgent.Export(ctx, env.AgentOutput()); err != nil {
		return err
	}

	return nil
}

// renderCheckConfig applies optional environment overrides to the embedded
// check template so an operator can retarget the PFMP Gateway / credentials
// without editing the committed YAML:
//
//	PFMP_GATEWAY_URL   -> powerflex_gateway_url
//	POWERFLEX_USERNAME -> powerflex_username
//	POWERFLEX_PASSWORD -> powerflex_password
func renderCheckConfig() string {
	cfg := checkConfig
	cfg = replaceIfSet(cfg, `"https://10.55.0.40:443"`, os.Getenv("PFMP_GATEWAY_URL"))
	cfg = replaceIfSet(cfg, `"platform-manager"`, os.Getenv("POWERFLEX_USERNAME"))
	cfg = replaceIfSet(cfg, `"CHANGEME"`, os.Getenv("POWERFLEX_PASSWORD"))
	return cfg
}

func replaceIfSet(cfg, placeholder, value string) string {
	if value == "" {
		return cfg
	}
	return strings.Replace(cfg, placeholder, `"`+value+`"`, 1)
}
