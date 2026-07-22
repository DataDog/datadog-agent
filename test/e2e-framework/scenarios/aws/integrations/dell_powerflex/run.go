// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dellpowerflex is the all-in-one Dell PowerFlex lab scenario.
//
// Architecture (single framework-provisioned host; the PowerFlex stack runs as
// nested libvirt VMs inside it):
//
//	ONE m5.metal RHEL9 host, 1500 GiB gp3 root, provisioned via the framework
//	ec2.NewVM (so it is reachable through appgate). By default it boots the golden
//	AMI, which already carries the full stack as nested libvirt VMs: the
//	virtualization stack (qemu-kvm/libvirt/...) + NAT network, a 3-node PFMP
//	4.6.2.1 cluster + a PowerFlex block cluster (MDM + 3 SDS), and the released
//	Datadog Agent + dell_powerflex check pointed at the PFMP Gateway MetalLB VIP
//	https://10.55.0.40:443 — so create boots straight to a healthy cluster
//	emitting the full metric set.
//
// On a cold m5.metal boot the nested cluster takes ~80-90 min to reconstitute
// (rke2 quorum -> MetalLB VIP -> PFMP REST); until then only
// dell_powerflex.api.can_connect is emitted, then the full ~556-metric / 82-family
// set. The golden AMI (defaultGoldenAMI) is us-east-1 only; override via
// ddinfra:dell_powerflex/goldenAMI (see run_args.go).
//
// Setting ddinfra:dell_powerflex/goldenAMI to an empty string takes the
// from-scratch path instead: vanilla RHEL9 + InstallVirtStack + a deferred PFMP
// bring-up staged by bootstrap.sh (the single-node NAT reserves PFMP at
// 10.55.0.20; set ddinfra:dell_powerflex/gatewayUrl accordingly). Until
// bootstrapped the check reports dell_powerflex.api.can_connect=0.
//
// Registered in the scenario registry as "aws/integrations/dell_powerflex".
package dellpowerflex

import (
	"strings"

	_ "embed"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	dellpowerflexcomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/integration/dell_powerflex"
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
	// Override via ddinfra:dell_powerflex/goldenAMI; set it empty for the
	// from-scratch build path (see run_args.go).
	defaultGoldenAMI = "ami-06fd51beaaa00b88d"
)

// checkConfig is the dell_powerflex check configuration template, edited by the
// lab maintainer at dell_powerflex.yaml and embedded here.
//
//go:embed dell_powerflex.yaml
var checkConfig string

// VMRun is the pulumi entry point registered in the integrations registry.
func VMRun(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	return Run(ctx, awsEnv, outputs.NewHost(), ParamsFromEnvironment(awsEnv))
}

// Run provisions the single m5.metal host, prepares the host virtualization
// stack, and installs the released Agent + dell_powerflex check.
func Run(ctx *pulumi.Context, awsEnv aws.Environment, env outputs.HostOutputs, params *Params) error {
	// Default path: boot the turnkey golden AMI on m5.metal — it already carries
	// KVM/libvirt, the autostart nested VMs (PFMP + the PowerFlex block cluster),
	// boot-persistent scini/fio/node services, and the Agent + dell_powerflex
	// check, so we SKIP InstallVirtStack and agent re-provisioning (they would
	// clash with the libvirt net / wipe the installed integration). An empty
	// params.GoldenAMI takes the from-scratch build path instead.
	goldenAMI := params.GoldenAMI

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
	// and credentials are overridable via ddinfra config at deploy time.
	agentConfig := renderCheckConfig(params)
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

// renderCheckConfig applies the optional ddinfra config overrides (GatewayURL /
// Username / Password) to the embedded check template so an operator can retarget
// the PFMP Gateway / credentials without editing the committed YAML.
func renderCheckConfig(params *Params) string {
	cfg := checkConfig
	cfg = replaceIfSet(cfg, `"https://10.55.0.40:443"`, params.GatewayURL)
	cfg = replaceIfSet(cfg, `"platform-manager"`, params.Username)
	cfg = replaceIfSet(cfg, `"CHANGEME"`, params.Password)
	return cfg
}

func replaceIfSet(cfg, placeholder, value string) string {
	if value == "" {
		return cfg
	}
	return strings.Replace(cfg, placeholder, `"`+value+`"`, 1)
}
