// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package lustre provisions a two-host AWS environment for the Lustre 2.15.6
// integration lab: a combined MGS+MDS+OSS server and a client that mounts the
// filesystem and runs the Datadog Agent with the bundled lustre check.
//
// Topology:
//   - lustre-server (m5.xlarge, EL8.10): Whamcloud kernel-pinned + versionlocked,
//     combined MGT+MDT + single OST on ldiskfs, LNet tcp0. vm.Export() emits stack
//     output dd-Host-aws-lustre-server (task role "aws-lustre-server", user alias
//     "server").
//   - agent-host    (m5.large): Lustre client + continuous fio I/O + the Datadog
//     Agent running the bundled lustre check (node_type=client). Defaults to Lustre
//     2.15.6 on AlmaLinux 8.10; opt into a 2.16.1 client on AlmaLinux 9.4 via
//     ddinfra:lustre/clientLustreVersion + ddinfra:lustre/clientOS (see run_args.go).
//     vm.Export() emits dd-Host-aws-agent-host (task role "aws-agent-host", the
//     default; user aliases "client"/"agent").
//
// The reusable Lustre bootstrap (server + client host roles, embedded scripts) is
// the shared component components/integration/lustre; this scenario wires it up.
//
// Coverage note: the bundled datadog-lustre v1.5.0 check collects only net.*/
// device.* on Lustre <=2.15.x (it uses `lctl get_param -ny`, unsupported there);
// full family coverage lands with the Agent 7.83 check fix.
package lustre

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	awsec2 "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2"
	ec2scenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed lustre.yaml
var lustreCheckConfig string

// almaLinuxOwner is the official AlmaLinux OS AWS account that publishes AMIs
// (both the EL8 and EL9 lines).
const almaLinuxOwner = "764336703387"

// clientOSSpec maps a clientOS param value to everything the client host needs:
// the AMI search name pattern (pinned to the point release so the running kernel
// matches the Whamcloud kmod), the framework OS descriptor version, and the
// Whamcloud client repo el-path.
type clientOSSpec struct {
	amiNameFilter string
	descVersion   string
	elPath        string
}

// resolveClientOS returns the AMI/descriptor/el-path for the requested client OS.
func resolveClientOS(name string) (clientOSSpec, error) {
	switch name {
	case clientOSAlmaLinux810:
		// EL8.10: stock GA kernel is KABI-compatible with the 2.15.x client kmod.
		return clientOSSpec{amiNameFilter: "AlmaLinux OS 8*x86_64*", descVersion: "8-10", elPath: "el8.10"}, nil
	case clientOSAlmaLinux94:
		// EL9.4: pin the AMI to the 9.4 line (5.14.0-427 kernel) so the standard
		// KABI kmod-lustre-client (built for el9.4) loads with no reboot.
		return clientOSSpec{amiNameFilter: "AlmaLinux OS 9.4*x86_64*", descVersion: "9-4", elPath: "el9.4"}, nil
	default:
		return clientOSSpec{}, fmt.Errorf("unsupported clientOS %q (want %q or %q)", name, clientOSAlmaLinux810, clientOSAlmaLinux94)
	}
}

// VMRun is the pulumi entry point registered in the integrations registry.
func VMRun(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	return Run(ctx, awsEnv, outputs.NewHost(), ParamsFromEnvironment(awsEnv))
}

// Run provisions the Lustre server + client and the Datadog Agent on the client.
func Run(ctx *pulumi.Context, awsEnv aws.Environment, env outputs.HostOutputs, params *Params) error {
	// Resolve a version-pinned EL8.10 x86_64 AMI for the SERVER. The framework
	// AlmaLinux resolver hardcodes EL9, so resolve EL8 explicitly via the official
	// AlmaLinux owner. SearchAMI returns the newest match at deploy time (no
	// fabricated AMI id). The server is always 2.15.6 ldiskfs on EL8.10.
	el8AMI, err := awsec2.SearchAMI(awsEnv, almaLinuxOwner, "AlmaLinux OS 8*x86_64*", string(compos.AMD64Arch))
	if err != nil {
		return fmt.Errorf("resolving EL8.10 AMI: %w", err)
	}
	el8Desc := compos.NewDescriptorWithArch(compos.AlmaLinux, "8-10", compos.AMD64Arch)

	// Resolve the CLIENT OS (default EL8.10; opt-in EL9.4 for the 2.16 client).
	clientSpec, err := resolveClientOS(params.ClientOS)
	if err != nil {
		return err
	}
	clientAMI, err := awsec2.SearchAMI(awsEnv, almaLinuxOwner, clientSpec.amiNameFilter, string(compos.AMD64Arch))
	if err != nil {
		return fmt.Errorf("resolving client AMI (%s): %w", params.ClientOS, err)
	}
	clientDesc := compos.NewDescriptorWithArch(compos.AlmaLinux, clientSpec.descVersion, compos.AMD64Arch)

	// ----- Server host (combined MGS+MDS+OSS) -----
	// The server provisions itself entirely from cloud-init user-data (patched
	// kernel install + boot-time format/mount + reboot into the patched kernel),
	// so it has no Pulumi remote command that the reboot could sever. The client
	// waits for it (lctl ping + retry mount) from its own host.
	serverHost, err := ec2scenario.NewVM(awsEnv, serverVMName,
		ec2scenario.WithAMI(el8AMI, el8Desc, compos.AMD64Arch),
		ec2scenario.WithInstanceType(params.ServerInstanceType),
		ec2scenario.WithUserData(serverUserData()),
	)
	if err != nil {
		return err
	}
	if err := serverHost.Export(ctx, nil); err != nil {
		return err
	}

	// ----- Client host (Lustre client + Datadog Agent) -----
	clientHost, err := ec2scenario.NewVM(awsEnv, clientVMName,
		ec2scenario.WithAMI(clientAMI, clientDesc, compos.AMD64Arch),
		ec2scenario.WithInstanceType(params.ClientInstanceType),
	)
	if err != nil {
		return err
	}
	// Export under dd-Host-aws-agent-host for the task surface (REMOTE_HOSTNAME=aws-agent-host).
	if err := clientHost.Export(ctx, env.RemoteHostOutput()); err != nil {
		return err
	}

	// The client waits for the server itself (lctl ping + retry mount inside
	// client_setup.sh); it depends on the server VM only for its IP/NID.
	// The client Lustre version + repo el-path are plumbed to client_setup.sh.
	clientSetup, err := newClientSetup(&awsEnv, clientHost, serverHost.Address,
		params.ClientLustreVersion, clientSpec.elPath)
	if err != nil {
		return err
	}

	// Continuous I/O so the check's performance families are non-zero.
	load, err := newLoad(&awsEnv, clientHost,
		utils.PulumiDependsOn(clientSetup))
	if err != nil {
		return err
	}

	// FakeIntake is opt-in and off by default (real E2E lab).
	env.DisableFakeIntake()
	env.DisableUpdater()

	// ----- Datadog Agent on the client, gated on the mount + load being up -----
	if params.agentOptions != nil {
		agentOptions := append(params.agentOptions,
			agentparams.WithIntegration("lustre.d", lustreCheckConfig),
			agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}),
			agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(clientSetup, load)),
		)
		agentComp, err := agent.NewHostAgent(&awsEnv, clientHost, agentOptions...)
		if err != nil {
			return err
		}
		if err := agentComp.Export(ctx, env.AgentOutput()); err != nil {
			return err
		}
		env.SetAgentClientOptions()
	} else {
		env.DisableAgent()
	}

	return nil
}
