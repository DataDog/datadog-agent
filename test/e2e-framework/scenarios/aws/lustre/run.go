// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package lustre is the Agent E2E scenario for the Datadog `lustre` integration.
//
// It provisions a single x86_64 EL9 EC2 host, bootstraps an all-in-one Lustre
// 2.15 filesystem on it (MGS/MGT + MDS/MDT + OSS/OST + client over loopback
// LNet, see components/datadog/apps/lustre), starts a continuous I/O + metadata
// workload, and installs the Datadog Agent with a three-instance `lustre.d`
// check config (one instance per node_type: client, mds, oss).
//
// Deployment shape (from the generation profile):
//
//   - Lustre relies on out-of-tree kernel modules (lustre, lnet, ldiskfs, ...)
//     and a kernel-matched, Lustre-patched e2fsprogs (mkfs.lustre). Docker
//     Compose cannot represent it faithfully, so this scenario uses the
//     upstream `llmount.sh`-style single-VM all-in-one mode instead. There is
//     intentionally NO docker-compose.yaml.
//   - x86_64 only: the Whamcloud el9 *server* RPMs are x86_64; arm64 servers are
//     unsupported. This is why the scenario cannot reuse the installer lab's
//     ARM64 t4g instances and instead pins os.AMD64Arch + m5.xlarge.
//   - m5.xlarge gives >= 16 GB RAM; the check's 5 s subprocess timeout drops
//     metrics on under-sized hosts.
//
// Registry wiring (DOCUMENTED, not applied by this phase): add
//
//	"aws/lustre": lustre.Run
//
// to the ScenarioRegistry in test/e2e-framework/registry/scenarios.go (importing
// this package as `"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/lustre"`).
// The invoke create/destroy tasks select this scenario by the key "aws/lustre".
package lustre

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	lustrecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/lustre"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	_ "embed"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// vmName is the logical name of the single all-in-one host. It is the
	// resource-name prefix for the VM, the Lustre component, and the Agent, and
	// it appears in the exported Pulumi stack outputs.
	vmName = "lustre"

	// instanceType is the EC2 instance type for the all-in-one host.
	// m5.xlarge => 4 vCPU / 16 GiB RAM (x86_64). >= 16 GiB is recommended so the
	// check's 5 s subprocess timeout does not drop metrics, and so the DKMS
	// kernel-module build is not starved.
	instanceType = "m5.xlarge"
)

// lustreCheckConfig is the three-instance `lustre.d/conf.yaml` delivered to the
// Agent via agentparams.WithIntegration("lustre.d", ...). It is embedded from
// the scenario-local conf.yaml, which is a maintained copy of the component
// package's config/conf.yaml (//go:embed cannot read across package
// directories). The two MUST stay in lock-step:
//
//   - the three pinned node_type instances (client | mds | oss) match the
//     all-in-one host, where autodetection would otherwise return only `mds`;
//   - `filesystems: [lustre]` matches lustrecomp.FilesystemName, the fsname the
//     bootstrap creates;
//   - `enable_extra_params: true` on every instance unlocks the
//     osc/mdc/mdt/obdfilter/ost/ldlm/export stat families.
//
//go:embed conf.yaml
var lustreCheckConfig string

// Run is the Pulumi entry point for the "aws/lustre" scenario.
//
// It is a custom Run(ctx) program (not a typed-environment shape) because the
// Lustre host bootstrap is bespoke: it cannot be expressed through the stock
// awshost provisioner options. The flow is:
//
//  1. Create the AWS environment and a single x86_64 EL9 EC2 VM.
//  2. Bootstrap Lustre on the VM via lustrecomp.NewLustreHost. The returned
//     component exposes Ready, a resource that completes only once the
//     filesystem is mounted and healthy and the workload is running.
//  3. Install the Datadog Agent, gated on lustreHost.Ready via
//     utils.PulumiDependsOn, so the first check runs against a host whose
//     counters are already moving.
//
// Stack outputs: the VM (RemoteHost: SSH connection, private IP, OS) and the
// Agent are exported via their Export(ctx, nil) methods. Those outputs are what
// the invoke create task surfaces (SSH command / `datadog-agent` commands) and
// what the destroy task tears down.
func Run(ctx *pulumi.Context) error {
	env, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// --- Infrastructure: single all-in-one x86_64 EL9 host ------------------
	//
	// os.RedHat9 is the primary descriptor: NewDescriptor(RedHat, "9") and it
	// is wired with a default AMI in the framework. RedHat and RockyLinux are
	// both dnf-family EL9, so the component's bootstrap script works on either.
	//
	// WithOSArch(..., os.AMD64Arch) is explicit even though RedHat9 already
	// defaults to x86_64, to make the arch requirement unmistakable: the
	// Whamcloud el9 server RPMs are x86_64-only.
	//
	// NOTE (root volume): the bootstrap places the MGT/MDT/OST loop backing
	// files plus the Whamcloud RPMs and DKMS build artifacts on the root
	// volume, which needs ~30-40 GB. ec2.NewVM does not expose a root-volume
	// size option; the size comes from the framework default
	// (env.DefaultInstanceStorageSize()). If that default is smaller than
	// ~40 GB, raise it for this stack via the `ddinfra:aws/defaultInstanceStorageSize`
	// Pulumi config (or an equivalent infra config key) when running the
	// create task. This is a deploy-time knob, not a code change.
	vm, err := ec2.NewVM(
		env,
		vmName,
		ec2.WithOSArch(os.RedHat9, os.AMD64Arch),
		ec2.WithInstanceType(instanceType),
	)
	if err != nil {
		return err
	}
	// Export the RemoteHost outputs (SSH connection, private IP, OS) so the
	// invoke create task can print the SSH command and run native validation
	// (lctl dl, lnetctl net show, lfs df, lsmod | grep lustre, ...).
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	// --- Lustre bootstrap ----------------------------------------------------
	//
	// NewLustreHost stages and runs the embedded install.sh (repos +
	// Lustre-patched e2fsprogs + lustre-dkms + kernel pin + reboot) and
	// configure.sh (modprobe -> LNet 0@lo -> mkfs.lustre loop targets ->
	// mount MGS/MDT/OST/client -> jobstats -> changelog_register -> dd-agent
	// NOPASSWD sudoers -> load systemd unit -> warm-up I/O). It also installs
	// the continuous workload (load.sh) so the check sees non-zero counters.
	//
	// On `pulumi destroy` the component's Delete command runs configure.sh
	// teardown (stop load, umount, detach loops) before the EC2 instance is
	// removed; loop devices live on the ephemeral instance, so no external
	// state is left behind.
	lustreHost, err := lustrecomp.NewLustreHost(&env, vmName, vm)
	if err != nil {
		return err
	}

	// --- Datadog Agent -------------------------------------------------------
	//
	// agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(lustreHost.Ready))
	// gates the Agent install on the filesystem being healthy AND the workload
	// running. This is the load-bearing ordering contract from the component:
	// the Agent must not run its first `lustre` check before the mount exists
	// and counters are moving, or the check would auto-detect the wrong
	// node_type / emit empty families.
	//
	// The check shells out to /usr/sbin/lctl, /usr/sbin/lnetctl, /usr/bin/lfs
	// (most with sudo); the component's sudoers drop-in grants dd-agent
	// passwordless sudo for exactly those three binaries.
	agentComp, err := agent.NewHostAgent(
		&env,
		vm,
		// Three-instance lustre.d check config (client | mds | oss).
		agentparams.WithIntegration("lustre.d", lustreCheckConfig),
		// WithLogs() sets logs_enabled: true in datadog.yaml, required for the
		// client instance's Lustre changelog log stream (conf.yaml `logs:`
		// stanza + enable_changelogs: true). No secrets here; DD_API_KEY is
		// injected by the Agent install via env.AgentAPIKey().
		agentparams.WithLogs(),
		// Order the Agent strictly after the Lustre bootstrap + workload.
		agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(lustreHost.Ready)),
		// Tag the host so payloads in fakeintake/Datadog are easy to find.
		agentparams.WithTags([]string{"lustre_lab:all-in-one", "scenario:aws-lustre"}),
	)
	if err != nil {
		return err
	}
	// Export the Agent outputs (host reference, FIPS flag) for the create task.
	if err := agentComp.Export(ctx, nil); err != nil {
		return err
	}

	// --- Fakeintake (DEFERRED) ----------------------------------------------
	//
	// This scenario does not provision a fakeintake; the minimal validation
	// path is `datadog-agent check lustre` plus the native `lctl`/`lnetctl`/
	// `lfs` commands on the host (the installer lab it is modeled on also omits
	// fakeintake). Payloads flow to the real Datadog org configured by the E2E
	// framework (dddev by default).
	//
	// TODO(future bridge): to assert lustre.* metrics in a downstream E2E suite,
	// provision a fakeintake and point the Agent at it. The wiring is:
	//
	//	fi, err := fakeintakescenario.NewECSFargateInstance(env, vmName)
	//	if err != nil { return err }
	//	if err := fi.Export(ctx, nil); err != nil { return err }
	//	// then add agentparams.WithFakeintake(fi) to the NewHostAgent options
	//	// above, and export the fakeintake URL so the create task / README can
	//	// surface it.
	//
	// (import: fakeintakescenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake")

	return nil
}
